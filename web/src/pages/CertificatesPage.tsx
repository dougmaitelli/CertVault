import { useState } from "react";
import "./CertificatesPage.css";
import { api } from "../api/client";
import type { Certificate } from "../api/types";
import { CertificateDetails } from "../components/CertificateDetails";
import { CertificateDownloadLink } from "../components/CertificateDownloadLink";
import { Stat } from "../components/Stat";
import { StatusBadge, StatusBadgeGroup } from "../components/StatusBadge";
import { formatDate, formatRemainingValidity } from "../utils/date";

type CertificatesPageProps = {
  certificates: Certificate[];
  reload: () => Promise<void>;
};

export function CertificatesPage({
  certificates,
  reload,
}: CertificatesPageProps) {
  const [selectedName, setSelectedName] = useState<string>();
  const selected = certificates.find(
    (certificate) => certificate.name === selectedName,
  );
  const [layout, setLayout] = useState<"list" | "grid">("list");
  const [renewalError, setRenewalError] = useState("");
  const [requestedRenewals, setRequestedRenewals] = useState<
    Partial<Record<string, number>>
  >({});

  const renew = async (certificate: Certificate) => {
    const name = certificate.name;
    setRenewalError("");
    const latestJobID = certificate.latest_job?.id ?? 0;
    setRequestedRenewals((current) => ({
      ...current,
      [name]: latestJobID + 1,
    }));
    try {
      const job = await api<{ job_id: number }>(
        `certificates/${encodeURIComponent(name)}/renew`,
        {
          method: "POST",
        },
      );
      setRequestedRenewals((current) => ({ ...current, [name]: job.job_id }));
      await reload();
    } catch (error) {
      setRequestedRenewals((current) => {
        const pending = { ...current };
        delete pending[name];
        return pending;
      });
      setRenewalError(`Unable to renew ${name}: ${String(error)}`);
    }
  };

  const taskRunning = (certificate: Certificate) => {
    const previousJobID = requestedRenewals[certificate.name];
    const latestJob = certificate.latest_job;
    const requestedRenewalCompleted =
      previousJobID !== undefined &&
      latestJob?.finished_at !== undefined &&
      latestJob.id >= previousJobID;
    const requestedRenewalPending =
      previousJobID !== undefined && !requestedRenewalCompleted;
    return (
      requestedRenewalPending ||
      latestJob?.status === "running" ||
      latestJob?.status === "queued"
    );
  };

  return (
    <>
      {renewalError && (
        <div className="error" role="alert">
          {renewalError}
        </div>
      )}
      <div className="stats">
        <Stat value={certificates.length} label="Managed certificates" />
        <Stat
          value={certificates.filter((item) => item.status === "valid").length}
          label="Healthy"
        />
        <Stat
          value={certificates.filter((item) => item.status === "error").length}
          label="Needs attention"
        />
      </div>
      <div className="certificate-toolbar">
        <div
          className="layout-picker"
          role="group"
          aria-label="Certificate layout"
        >
          <button
            className={layout === "list" ? "active" : ""}
            aria-pressed={layout === "list"}
            onClick={() => setLayout("list")}
          >
            <svg aria-hidden="true" viewBox="0 0 16 16">
              <path d="M2 3h2v2H2zm4 0h8v2H6zM2 7h2v2H2zm4 0h8v2H6zm-4 4h2v2H2zm4 0h8v2H6z" />
            </svg>
            List
          </button>
          <button
            className={layout === "grid" ? "active" : ""}
            aria-pressed={layout === "grid"}
            onClick={() => setLayout("grid")}
          >
            <svg aria-hidden="true" viewBox="0 0 16 16">
              <path d="M2 2h5v5H2zm7 0h5v5H9zM2 9h5v5H2zm7 0h5v5H9z" />
            </svg>
            Grid
          </button>
        </div>
      </div>
      <div className={`grid certificate-list ${layout}`}>
        {certificates.map((certificate) => (
          <article
            key={certificate.name}
            onClick={() => setSelectedName(certificate.name)}
          >
            <div className="row">
              <h3>{certificate.name}</h3>
              <StatusBadgeGroup>
                {taskRunning(certificate) && (
                  <StatusBadge
                    status="running"
                    label={
                      certificate.latest_job?.status === "queued"
                        ? "Queued"
                        : "Running"
                    }
                    active
                  />
                )}
                <StatusBadge status={certificate.status} />
              </StatusBadgeGroup>
            </div>
            <code className="certificate-domains">
              {certificate.domains.join(", ")}
            </code>
            <dl>
              <div>
                <dt>Expires</dt>
                <dd>
                  {formatDate(certificate.current_version?.not_after)}{" "}
                  {formatRemainingValidity(
                    certificate.current_version?.not_after,
                  )}
                </dd>
              </div>
              <div>
                <dt>Key</dt>
                <dd>{certificate.key_type.toUpperCase()}</dd>
              </div>
            </dl>
            {certificate.last_error && (
              <p className="error">{certificate.last_error}</p>
            )}
            <div className="action-buttons">
              <CertificateDownloadLink
                certificateName={certificate.name}
                artifact="fullchain.crt"
                disabled={!certificate.current_version}
              >
                Full chain
              </CertificateDownloadLink>
              <CertificateDownloadLink
                certificateName={certificate.name}
                artifact="private.key"
                disabled={!certificate.current_version}
              >
                Private key
              </CertificateDownloadLink>
              <button
                className="action-button success"
                disabled={taskRunning(certificate)}
                onClick={(event) => {
                  event.stopPropagation();
                  void renew(certificate);
                }}
              >
                {taskRunning(certificate)
                  ? certificate.latest_job?.status === "queued"
                    ? "Queued"
                    : "Running"
                  : "Renew"}
              </button>
            </div>
          </article>
        ))}
      </div>
      {selected && (
        <CertificateDetails
          key={selected.name}
          certificate={selected}
          onClose={() => setSelectedName(undefined)}
        />
      )}
    </>
  );
}
