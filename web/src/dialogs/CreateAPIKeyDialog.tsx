import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import type { APIKeyCreationResponse, Certificate } from "../api/types";
import { Checkbox } from "../components/Checkbox";
import { Modal } from "../components/Modal";
import { MultiSelect } from "../components/MultiSelect";

const allCertificatesAccess = "*";
const certificateReadScopes = ["certificates:read", "private_keys:read"];

type CreateAPIKeyDialogProps = {
  certificates: Certificate[];
  onClose: () => void;
  onCreated: (token: string) => Promise<void>;
};

export function CreateAPIKeyDialog({
  certificates,
  onClose,
  onCreated,
}: CreateAPIKeyDialogProps) {
  const [name, setName] = useState("");
  const [anyCertificate, setAnyCertificate] = useState(false);
  const [selectedCertificates, setSelectedCertificates] = useState<string[]>(
    [],
  );

  async function create(event: FormEvent) {
    event.preventDefault();
    const output = await api<APIKeyCreationResponse>("api-keys", {
      method: "POST",
      body: JSON.stringify({
        name,
        scopes: certificateReadScopes,
        certificates: anyCertificate
          ? [allCertificatesAccess]
          : selectedCertificates,
      }),
    });
    await onCreated(output.token);
  }

  return (
    <Modal onClose={onClose}>
      <h2>Create API key</h2>
      <form onSubmit={(event) => void create(event)}>
        <label>
          Name
          <input
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            placeholder="nas-01"
          />
        </label>
        <fieldset className="certificate-access">
          <legend>Certificate access</legend>
          <Checkbox
            checked={anyCertificate}
            onChange={(event) => setAnyCertificate(event.target.checked)}
            required={certificates.length === 0}
          >
            Any certificate, including certificates added later
          </Checkbox>
          {certificates.length > 0 && !anyCertificate && (
            <MultiSelect
              label="Certificates"
              options={certificates.map((certificate) => ({
                label: certificate.name,
                value: certificate.name,
              }))}
              selected={selectedCertificates}
              onChange={setSelectedCertificates}
            />
          )}
        </fieldset>
        <button className="action-button success">Create key</button>
      </form>
    </Modal>
  );
}
