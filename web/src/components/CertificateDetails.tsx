import type { Certificate } from "../api/types";
import { formatDate } from "../utils/date";
import { Modal } from "./Modal";

type CertificateDetailsProps = {
  certificate: Certificate;
  onClose: () => void;
};

export function CertificateDetails({
  certificate,
  onClose,
}: CertificateDetailsProps) {
  return (
    <Modal onClose={onClose}>
      <small>CERTIFICATE DETAILS</small>
      <h2>{certificate.name}</h2>
      <h4>Subject alternative names</h4>
      {certificate.domains.map((domain) => (
        <code className="domain" key={domain}>
          {domain}
        </code>
      ))}
      <h4>Validity</h4>
      <p>
        {formatDate(certificate.current_version?.not_before)} —{" "}
        {formatDate(certificate.current_version?.not_after)}
      </p>
      {certificate.current_version && (
        <>
          <h4>Issuer and identity</h4>
          <p>{certificate.current_version.issuer}</p>
          <code className="domain">
            Serial: {certificate.current_version.serial}
          </code>
          <code className="domain">
            SHA-256: {certificate.current_version.fingerprint_sha256}
          </code>
        </>
      )}
      <h4>Downloads</h4>
      <div className="actions">
        <a href={`/api/v1/certificates/${certificate.name}/certificate.pem`}>
          Certificate
        </a>
        <a href={`/api/v1/certificates/${certificate.name}/chain.pem`}>Chain</a>
        <a href={`/api/v1/certificates/${certificate.name}/fullchain.pem`}>
          Full chain
        </a>
        <a href={`/api/v1/certificates/${certificate.name}/private-key.pem`}>
          Private key
        </a>
      </div>
    </Modal>
  );
}
