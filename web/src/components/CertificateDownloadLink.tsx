import type { ReactNode } from "react";

type CertificateArtifact =
  "certificate.crt" | "chain.crt" | "fullchain.crt" | "private.key";

type CertificateDownloadLinkProps = {
  artifact: CertificateArtifact;
  certificateName: string;
  children: ReactNode;
  disabled?: boolean;
};

export function CertificateDownloadLink({
  artifact,
  certificateName,
  children,
  disabled = false,
}: CertificateDownloadLinkProps) {
  const path = `/api/v1/certificates/${encodeURIComponent(certificateName)}/${artifact}`;

  return (
    <a
      className="action-button"
      href={disabled ? undefined : path}
      download={`${certificateName}-${artifact}`}
      aria-disabled={disabled}
      tabIndex={disabled ? -1 : undefined}
      onClick={(event) => {
        event.stopPropagation();
        if (disabled) event.preventDefault();
      }}
    >
      {children}
    </a>
  );
}
