import { useEffect, useMemo, useRef, useState } from "react";
import type { Certificate } from "../api/types";
import "./APIUsageHelper.css";

const copyFeedbackDuration = 2000;
const defaultOutputNames: Record<string, string> = {
  "certificate.crt": "certificate.crt",
  "chain.crt": "chain.crt",
  "fullchain.crt": "fullchain.crt",
  "private.key": "private.key",
};

const fileOptions = [
  {
    value: "fullchain.crt,private.key",
    label: "Full chain + private key",
  },
  { value: "fullchain.crt", label: "Full chain" },
  { value: "certificate.crt", label: "Certificate" },
  { value: "chain.crt", label: "CA chain" },
  { value: "private.key", label: "Private key" },
] as const;

const schedules = [
  { value: "17 3 * * *", label: "Daily at 03:17" },
  { value: "17 * * * *", label: "Hourly at :17" },
  { value: "17 3 * * 0", label: "Sunday at 03:17" },
] as const;

const installationPresets = [
  {
    id: "proxmox-ve",
    label: "Proxmox VE",
    description:
      "Run on each Proxmox VE node as root. These settings use the custom certificate files and restart pveproxy after files change. You can edit the settings below.",
    files: "fullchain.crt,private.key",
    destination: "/etc/pve/local",
    outputNames: {
      ...defaultOutputNames,
      "fullchain.crt": "pveproxy-ssl.pem",
      "private.key": "pveproxy-ssl.key",
    },
    schedule: schedules[0].value,
    reloadCommand: "systemctl restart pveproxy",
    useSudo: false,
  },
  {
    id: "proxmox-backup-server",
    label: "Proxmox Backup Server",
    description:
      "Run on the backup server as root. The command sets root:backup ownership and mode 0640 before reloading proxmox-backup-proxy. Keep the paths in the command aligned with any destination or filename edits. Clients that pin the certificate fingerprint need their fingerprint updated when the certificate changes.",
    files: "fullchain.crt,private.key",
    destination: "/etc/proxmox-backup",
    outputNames: {
      ...defaultOutputNames,
      "fullchain.crt": "proxy.pem",
      "private.key": "proxy.key",
    },
    schedule: schedules[0].value,
    reloadCommand:
      "chown root:backup /etc/proxmox-backup/proxy.pem /etc/proxmox-backup/proxy.key && chmod 640 /etc/proxmox-backup/proxy.pem /etc/proxmox-backup/proxy.key && systemctl reload proxmox-backup-proxy",
    useSudo: false,
  },
] as const;

type APIUsageHelperProps = {
  certificates: Certificate[];
  scopes: string[];
  token?: string;
};

export function APIUsageHelper({
  certificates,
  scopes,
  token,
}: APIUsageHelperProps) {
  const availableFileOptions = fileOptions.filter((option) =>
    option.value
      .split(",")
      .every((file) =>
        file === "private.key"
          ? scopes.includes("private_keys:read")
          : scopes.includes("certificates:read"),
      ),
  );
  const [certificate, setCertificate] = useState(certificates[0]?.name ?? "");
  const [files, setFiles] = useState<string>(
    availableFileOptions[0]?.value ?? "",
  );
  const [destination, setDestination] = useState(() =>
    defaultDestination(certificates[0]?.name ?? ""),
  );
  const [outputNames, setOutputNames] = useState(defaultOutputNames);
  const [autoUpdate, setAutoUpdate] = useState(
    window.location.protocol === "https:",
  );
  const [reloadCommand, setReloadCommand] = useState("");
  const [schedule, setSchedule] = useState<string>(schedules[0].value);
  const [copied, setCopied] = useState(false);
  const [updateOnly, setUpdateOnly] = useState(false);
  const [preset, setPreset] = useState("");
  const selectedPreset = installationPresets.find((item) => item.id === preset);
  const [useSudo, setUseSudo] = useState(true);
  const canInstall = certificates.length > 0 && availableFileOptions.length > 0;
  const copyFeedbackTimeout = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (copyFeedbackTimeout.current !== null) {
        window.clearTimeout(copyFeedbackTimeout.current);
      }
    },
    [],
  );

  const command = useMemo(() => {
    const installer = `${window.location.origin}/client/install.sh`;
    if (updateOnly) {
      return [
        `curl -fsSL ${shellQuote(installer)}`,
        `| ${useSudo ? "sudo " : ""}sh -s --`,
        "--update-client-only",
        `--server ${shellQuote(window.location.origin)}`,
      ].join(" \\\n  ");
    }
    const executor = `${useSudo ? "sudo " : ""}${token ? `env CERTVAULT_API_KEY=${shellQuote(token)} ` : ""}sh`;
    const fileArguments = files
      .split(",")
      .filter(Boolean)
      .map((file) => {
        const output = outputNames[file] || file;
        const specification = output === file ? file : `${file}=${output}`;
        return `--file ${shellQuote(specification)}`;
      });
    return [
      `curl ${autoUpdate ? "--proto '=https' --proto-redir '=https' " : ""}-fsSL ${shellQuote(installer)}`,
      `| ${executor} -s --`,
      `--server ${shellQuote(window.location.origin)}`,
      `--certificate ${shellQuote(certificate)}`,
      ...(!token ? ["--reuse-key"] : []),
      ...(autoUpdate ? ["--auto-update"] : []),
      ...fileArguments,
      `--destination ${shellQuote(destination)}`,
      `--schedule ${shellQuote(schedule)}`,
      ...(reloadCommand
        ? [`--reload-command ${shellQuote(reloadCommand)}`]
        : []),
    ].join(" \\\n  ");
  }, [
    autoUpdate,
    certificate,
    destination,
    files,
    outputNames,
    reloadCommand,
    schedule,
    token,
    updateOnly,
    useSudo,
  ]);

  function selectPreset(id: string) {
    const selected = installationPresets.find((item) => item.id === id);
    setPreset(id);
    setFiles(selected?.files ?? availableFileOptions.at(0)?.value ?? "");
    setDestination(selected?.destination ?? defaultDestination(certificate));
    setOutputNames(selected?.outputNames ?? defaultOutputNames);
    setSchedule(selected?.schedule ?? schedules[0].value);
    setReloadCommand(selected?.reloadCommand ?? "");
    setUseSudo(selected?.useSudo ?? true);
    setCopied(false);
  }

  function selectCertificate(nextCertificate: string) {
    const previousDefault = defaultDestination(certificate);
    setCertificate(nextCertificate);
    setDestination((current) =>
      current === previousDefault
        ? defaultDestination(nextCertificate)
        : current,
    );
  }

  async function copyCommand() {
    await navigator.clipboard.writeText(command);
    setCopied(true);
    if (copyFeedbackTimeout.current !== null) {
      window.clearTimeout(copyFeedbackTimeout.current);
    }
    copyFeedbackTimeout.current = window.setTimeout(
      () => setCopied(false),
      copyFeedbackDuration,
    );
  }

  return (
    <section className="api-usage-helper">
      <div className="api-usage-heading">
        <div>
          <h2>{updateOnly ? "Update client only" : "Automatic download"}</h2>
          <p>
            {updateOnly
              ? "Run this on the original client as the same user to update its scripts. All jobs keep their saved keys, schedules, destinations, file mappings, and reload commands."
              : "Run this on the client to install or update a recurring sync job."}
            {!updateOnly &&
              !token &&
              " Reuses the API key saved on that client. Run as the same user and select the same certificate and file set as the original installation."}
          </p>
        </div>
        {!updateOnly && (
          <div className="api-usage-options">
            <label>
              Certificate
              <select
                value={certificate}
                onChange={(event) => selectCertificate(event.target.value)}
              >
                {certificates.map((item) => (
                  <option key={item.name} value={item.name}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Files
              <select
                value={files}
                onChange={(event) => setFiles(event.target.value)}
              >
                {availableFileOptions.map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Schedule
              <select
                value={schedule}
                onChange={(event) => setSchedule(event.target.value)}
              >
                {schedules.map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </label>
          </div>
        )}
      </div>
      <label className="api-usage-auto-update">
        <input
          type="checkbox"
          checked={updateOnly}
          onChange={(event) => {
            setUpdateOnly(event.target.checked);
            setCopied(false);
          }}
        />
        Update only
      </label>
      {!updateOnly && !canInstall && (
        <p>
          Select update only to update an existing client. Installing a job
          requires an accessible certificate and download permissions.
        </p>
      )}
      {!updateOnly && (
        <>
          <label className="api-usage-output">
            Installation preset
            <select
              value={preset}
              onChange={(event) => selectPreset(event.target.value)}
            >
              <option value="">Custom</option>
              {installationPresets.map((item) => (
                <option
                  key={item.id}
                  value={item.id}
                  disabled={
                    !availableFileOptions.some(
                      (option) => option.value === item.files,
                    )
                  }
                >
                  {item.label}
                </option>
              ))}
            </select>
          </label>
          {selectedPreset && <p>{selectedPreset.description}</p>}
          <label className="api-usage-output">
            Destination folder on client
            <input
              value={destination}
              onChange={(event) => setDestination(event.target.value)}
              required
            />
          </label>
          <div className="api-usage-deployment">
            {files
              .split(",")
              .filter(Boolean)
              .map((file) => (
                <label className="api-usage-output" key={file}>
                  Output name for {file}
                  <input
                    value={outputNames[file] ?? file}
                    onChange={(event) =>
                      setOutputNames((current) => ({
                        ...current,
                        [file]: event.target.value,
                      }))
                    }
                    pattern="[A-Za-z0-9._-]+"
                    required
                  />
                </label>
              ))}
            <label className="api-usage-output">
              Command after files change (optional)
              <input
                value={reloadCommand}
                onChange={(event) => setReloadCommand(event.target.value)}
                placeholder="systemctl restart pveproxy"
              />
            </label>
          </div>
          <label className="api-usage-auto-update">
            <input
              type="checkbox"
              checked={autoUpdate}
              disabled={window.location.protocol !== "https:"}
              onChange={(event) => setAutoUpdate(event.target.checked)}
            />
            Automatically update the client (requires HTTPS)
          </label>
        </>
      )}
      <label className="api-usage-auto-update">
        <input
          type="checkbox"
          checked={useSudo}
          onChange={(event) => setUseSudo(event.target.checked)}
        />
        Use sudo (disable when running as root)
      </label>
      <div className="api-command">
        <code>{command}</code>
        <button
          className={`action-button ${copied ? "copied" : ""}`}
          onClick={() => void copyCommand()}
          disabled={!updateOnly && !canInstall}
          aria-live="polite"
        >
          {copied ? "Copied!" : "Copy command"}
        </button>
      </div>
    </section>
  );
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", `'"'"'`)}'`;
}

function defaultDestination(certificate: string): string {
  return `/etc/ssl/certvault/${certificate}`;
}
