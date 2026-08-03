import type { ReactNode } from "react";

type CertificateArtifact =
  "certificate.pem" | "chain.pem" | "fullchain.pem" | "private-key.pem";

type CertificateDownloadLinkProps = {
  artifact: CertificateArtifact;
  certificateName: string;
  children: ReactNode;
};

export function CertificateDownloadLink({
  artifact,
  certificateName,
  children,
}: CertificateDownloadLinkProps) {
  return (
    <a
      href={`/api/v1/certificates/${certificateName}/${artifact}`}
      download={`${certificateName}-${artifact}`}
      onClick={(event) => event.stopPropagation()}
    >
      {children}
    </a>
  );
}
