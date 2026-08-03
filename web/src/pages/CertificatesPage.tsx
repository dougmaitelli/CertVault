import { useState } from "react";
import { api } from "../api/client";
import type { Certificate, Job } from "../api/types";
import { CertificateDetails } from "../components/CertificateDetails";
import { CertificateDownloadLink } from "../components/CertificateDownloadLink";
import { Stat } from "../components/Stat";
import { formatDate } from "../utils/date";

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
      <div className="grid">
        {certificates.map((certificate) => (
          <article
            key={certificate.name}
            onClick={() => setSelected(certificate)}
          >
            <div className="row">
              <h3>{certificate.name}</h3>
              <div className="certificate-statuses">
                {taskRunning(certificate.name) && (
                  <span className="status running task-running">
                    <i /> Running
                  </span>
                )}
                <span className={`status ${certificate.status}`}>
                  {certificate.status}
                </span>
              </div>
            </div>
            <code className="certificate-domains">
              {certificate.domains.join(", ")}
            </code>
            <dl>
              <div>
                <dt>Expires</dt>
                <dd>{formatDate(certificate.current_version?.not_after)}</dd>
              </div>
              <div>
                <dt>Key</dt>
                <dd>{certificate.key_type.toUpperCase()}</dd>
              </div>
            </dl>
            {certificate.last_error && (
              <p className="error">{certificate.last_error}</p>
            )}
            <div className="actions">
              <CertificateDownloadLink
                certificateName={certificate.name}
                artifact="fullchain.pem"
              >
                Full chain
              </CertificateDownloadLink>
              <CertificateDownloadLink
                certificateName={certificate.name}
                artifact="private-key.pem"
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
          certificate={selected}
          onClose={() => setSelected(undefined)}
        />
      )}
    </>
  );
}
