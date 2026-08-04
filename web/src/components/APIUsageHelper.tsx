import { useEffect, useMemo, useRef, useState } from "react";
import type { Certificate } from "../api/types";
import "./APIUsageHelper.css";

const copyFeedbackDuration = 2000;

const artifacts = [
  { value: "fullchain.pem", label: "Full chain" },
  { value: "certificate.pem", label: "Certificate" },
  { value: "chain.pem", label: "CA chain" },
  { value: "private-key.pem", label: "Private key" },
] as const;

const schedules = [
  { value: "17 3 * * *", label: "Daily at 03:17" },
  { value: "17 * * * *", label: "Hourly at :17" },
  { value: "17 3 * * 0", label: "Sunday at 03:17" },
] as const;

type APIUsageHelperProps = {
  certificates: Certificate[];
  token: string;
};

export function APIUsageHelper({ certificates, token }: APIUsageHelperProps) {
  const [certificate, setCertificate] = useState(certificates[0]?.name ?? "");
  const [artifact, setArtifact] = useState<string>(artifacts[0].value);
  const [output, setOutput] = useState(() =>
    defaultOutput(certificates[0]?.name ?? "", artifacts[0].value),
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
      `curl -fsSL ${shellQuote(installer)} | ${executor} -s --`,
      `--server ${shellQuote(window.location.origin)}`,
      `--certificate ${shellQuote(certificate)}`,
      `--artifact ${shellQuote(artifact)}`,
      `--output ${shellQuote(output)}`,
      `--schedule ${shellQuote(schedule)}`,
    ].join(" ");
  }, [artifact, certificate, output, schedule, token]);

  function selectCertificate(nextCertificate: string) {
    const previousDefault = defaultOutput(certificate, artifact);
    setCertificate(nextCertificate);
    setOutput((current) =>
      current === previousDefault
        ? defaultOutput(nextCertificate, artifact)
        : current,
    );
  }

  function selectArtifact(nextArtifact: string) {
    const previousDefault = defaultOutput(certificate, artifact);
    setArtifact(nextArtifact);
    setOutput((current) =>
      current === previousDefault
        ? defaultOutput(certificate, nextArtifact)
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
          <p>Run this once on the client to install a recurring sync job.</p>
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
            File
            <select
              value={artifact}
              onChange={(event) => selectArtifact(event.target.value)}
            >
              {artifacts.map((item) => (
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
        Destination on client
        <input
          value={output}
          onChange={(event) => setOutput(event.target.value)}
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

function defaultOutput(certificate: string, artifact: string): string {
  return `/etc/ssl/certvault/${certificate}-${artifact}`;
}
