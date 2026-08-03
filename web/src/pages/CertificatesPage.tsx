import { useState } from "react";
import { api } from "../api/client";
import type { Certificate } from "../api/types";
import { CertificateDetails } from "../components/CertificateDetails";
import { CertificateDownloadLink } from "../components/CertificateDownloadLink";
import { Stat } from "../components/Stat";
import { formatDate } from "../utils/date";

type CertificatesPageProps = {
  certificates: Certificate[];
  reload: () => Promise<void>;
};

export function CertificatesPage({
  certificates,
  reload,
}: CertificatesPageProps) {
  const [selected, setSelected] = useState<Certificate>();

  const renew = async (name: string) => {
    await api(`certificates/${name}/renew`, { method: "POST" });
    await reload();
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
              <span className={`status ${certificate.status}`}>
                {certificate.status}
              </span>
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
                className="success"
                onClick={(event) => {
                  event.stopPropagation();
                  void renew(certificate.name);
                }}
              >
                Renew
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
