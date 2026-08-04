import type { Certificate } from "../api/types";
import { formatDate } from "../utils/date";
import { CertificateDownloadLink } from "./CertificateDownloadLink";
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
      <div className="action-buttons">
        <CertificateDownloadLink
          certificateName={certificate.name}
          artifact="certificate.pem"
        >
          Certificate
        </CertificateDownloadLink>
        <CertificateDownloadLink
          certificateName={certificate.name}
          artifact="chain.pem"
        >
          Chain
        </CertificateDownloadLink>
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
      </div>
    </Modal>
  );
}
