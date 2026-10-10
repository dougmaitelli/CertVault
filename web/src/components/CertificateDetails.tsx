import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Certificate, CertificateVersion } from "../api/types";
import { formatDate, formatRemainingValidity } from "../utils/date";
import "./CertificateDetails.css";
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
  const versionRequest = `${certificate.name}:${certificate.current_version?.id ?? "pending"}`;
  const [versionResult, setVersionResult] = useState<{
    request: string;
    versions: CertificateVersion[];
    error?: string;
  }>();
  const versionsLoading = versionResult?.request !== versionRequest;
  const versions = versionsLoading ? [] : versionResult.versions;
  const versionsError = versionsLoading ? "" : versionResult.error;

  useEffect(() => {
    const controller = new AbortController();

    void api<CertificateVersion[]>(
      `certificates/${encodeURIComponent(certificate.name)}/versions`,
      { signal: controller.signal },
    )
      .then((versions) => {
        if (!controller.signal.aborted) {
          setVersionResult({ request: versionRequest, versions });
        }
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setVersionResult({
            request: versionRequest,
            versions: [],
            error:
              error instanceof Error
                ? error.message
                : "Unable to load versions",
          });
        }
      });

    return () => controller.abort();
  }, [certificate.name, versionRequest]);

  return (
    <Modal onClose={onClose}>
      <small>CERTIFICATE DETAILS</small>
      <h2>{certificate.name}</h2>
      <h4>
        {certificate.current_version
          ? "Issued subject alternative names"
          : "Configured subject alternative names"}
      </h4>
      {(certificate.current_version?.domains ?? certificate.domains).map(
        (domain) => (
          <code className="domain" key={domain}>
            {domain}
          </code>
        ),
      )}
      <h4>Configured subject alternative names</h4>
      <p>{certificate.domains.join(", ")}</p>
      <h4>Configured key</h4>
      <p>{certificate.key_type.toUpperCase()}</p>
      <h4>Validity</h4>
      <p>
        {formatDate(certificate.current_version?.not_before)} —{" "}
        {formatDate(certificate.current_version?.not_after)}{" "}
        {formatRemainingValidity(certificate.current_version?.not_after)}
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
          artifact="certificate.crt"
          disabled={!certificate.current_version}
        >
          Certificate
        </CertificateDownloadLink>
        <CertificateDownloadLink
          certificateName={certificate.name}
          artifact="chain.crt"
          disabled={!certificate.current_version}
        >
          Chain
        </CertificateDownloadLink>
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
      </div>
      <h4>Version history</h4>
      {versionsLoading && <p className="version-message">Loading versions…</p>}
      {versionsError && <p className="error">{versionsError}</p>}
      {!versionsLoading && !versionsError && versions.length === 0 && (
        <p className="version-message">No certificate versions stored.</p>
      )}
      {versions.length > 0 && (
        <ol className="version-timeline">
          {versions.map((version) => {
            const current = version.id === certificate.current_version?.id;
            return (
              <li className={current ? "current" : ""} key={version.id}>
                <div className="version-heading">
                  <strong>
                    {current ? "Current version" : "Previous version"}
                  </strong>
                  <time dateTime={version.created_at}>
                    {formatDate(version.created_at)}
                  </time>
                </div>
                <p>
                  Valid {formatDate(version.not_before)} —{" "}
                  {formatDate(version.not_after)}{" "}
                  {formatRemainingValidity(version.not_after)}
                </p>
                <span>{version.issuer}</span>
                <code>{version.domains.join(", ")}</code>
              </li>
            );
          })}
        </ol>
      )}
    </Modal>
  );
}
