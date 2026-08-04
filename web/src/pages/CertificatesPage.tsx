import { useState } from "react";
import "./CertificatesPage.css";
import { api } from "../api/client";
import type { Certificate, Job } from "../api/types";
import { CertificateDetails } from "../components/CertificateDetails";
import { CertificateDownloadLink } from "../components/CertificateDownloadLink";
import { Stat } from "../components/Stat";
import { StatusBadge, StatusBadgeGroup } from "../components/StatusBadge";
import { formatDate, formatRemainingValidity } from "../utils/date";

type CertificatesPageProps = {
  certificates: Certificate[];
  jobs: Job[];
  reload: () => Promise<void>;
};

export function CertificatesPage({
  certificates,
  jobs,
  reload,
}: CertificatesPageProps) {
  const [selected, setSelected] = useState<Certificate>();
  const [layout, setLayout] = useState<"list" | "grid">("list");
  const [requestedRenewals, setRequestedRenewals] = useState<
    Partial<Record<string, number>>
  >({});

  const renew = async (name: string) => {
    const latestJobID = jobs.reduce(
      (latest, job) => Math.max(latest, job.id),
      0,
    );
    setRequestedRenewals((current) => ({
      ...current,
      [name]: latestJobID,
    }));
    try {
      await api(`certificates/${name}/renew`, { method: "POST" });
      await reload();
    } catch (error) {
      setRequestedRenewals((current) => {
        const pending = { ...current };
        delete pending[name];
        return pending;
      });
      throw error;
    }
  };

  const taskRunning = (name: string) => {
    const previousJobID = requestedRenewals[name];
    const requestedRenewalPending =
      previousJobID !== undefined &&
      !jobs.some(
        (job) =>
          job.certificate_name === name &&
          job.finished_at !== undefined &&
          job.id > previousJobID,
      );
    return (
      requestedRenewalPending ||
      jobs.some(
        (job) => job.certificate_name === name && job.status === "running",
      )
    );
  };

  return (
    <>
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
            onClick={() => setSelected(certificate)}
          >
            <div className="row">
              <h3>{certificate.name}</h3>
              <StatusBadgeGroup>
                {taskRunning(certificate.name) && (
                  <StatusBadge status="running" label="Running" active />
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
                artifact="fullchain.pem"
                disabled={!certificate.current_version}
              >
                Full chain
              </CertificateDownloadLink>
              <CertificateDownloadLink
                certificateName={certificate.name}
                artifact="private-key.pem"
                disabled={!certificate.current_version}
              >
                Private key
              </CertificateDownloadLink>
              <button
                className="action-button success"
                disabled={taskRunning(certificate.name)}
                onClick={(event) => {
                  event.stopPropagation();
                  void renew(certificate.name);
                }}
              >
                {taskRunning(certificate.name) ? "Running" : "Renew"}
              </button>
            </div>
          </article>
        ))}
      </div>
      {selected && (
        <CertificateDetails
          key={selected.name}
          certificate={selected}
          onClose={() => setSelected(undefined)}
        />
      )}
    </>
  );
}
