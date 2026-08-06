import { useEffect, useMemo, useRef, useState } from "react";
import type { Certificate } from "../api/types";
import "./APIUsageHelper.css";

const copyFeedbackDuration = 2000;

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

type APIUsageHelperProps = {
  certificates: Certificate[];
  scopes: string[];
  token: string;
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
  const [schedule, setSchedule] = useState<string>(schedules[0].value);
  const [copied, setCopied] = useState(false);
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
    const executor = `sudo env CERTVAULT_API_KEY=${shellQuote(token)} sh`;
    return [
      `curl -fsSL ${shellQuote(installer)}`,
      `| ${executor} -s --`,
      `--server ${shellQuote(window.location.origin)}`,
      `--certificate ${shellQuote(certificate)}`,
      `--files ${shellQuote(files)}`,
      `--destination ${shellQuote(destination)}`,
      `--schedule ${shellQuote(schedule)}`,
    ].join(" \\\n  ");
  }, [certificate, destination, files, schedule, token]);

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
          <h2>Automatic download</h2>
          <p>
            Run this on the client to install or update a recurring sync job.
          </p>
        </div>
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
      </div>
      <label className="api-usage-output">
        Destination folder on client
        <input
          value={destination}
          onChange={(event) => setDestination(event.target.value)}
          required
        />
      </label>
      <div className="api-command">
        <code>{command}</code>
        <button
          className={`action-button ${copied ? "copied" : ""}`}
          onClick={() => void copyCommand()}
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
