import { useState } from "react";
import { api } from "../api/client";
import type { APIKey, Certificate } from "../api/types";
import { CreateAPIKeyDialog } from "../dialogs/CreateAPIKeyDialog";
import { formatDate } from "../utils/date";

type APIKeysPageProps = {
  apiKeys: APIKey[];
  certificates: Certificate[];
  reload: () => Promise<void>;
};

export function APIKeysPage({
  apiKeys,
  certificates,
  reload,
}: APIKeysPageProps) {
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [token, setToken] = useState("");

  async function revoke(id: number) {
    if (confirm("Revoke this API key?")) {
      await api(`api-keys/${id}`, { method: "DELETE" });
      await reload();
    }
  }

  return (
    <>
      <div className="bar">
        <p>Machine credentials are stored as hashes and shown only once.</p>
        <button onClick={() => setShowCreateDialog(true)}>
          Create API key
        </button>
      </div>
      {token && (
        <div className="token">
          <b>Copy this token now — it cannot be shown again.</b>
          <code>{token}</code>
          <button onClick={() => void navigator.clipboard.writeText(token)}>
            Copy
          </button>
        </div>
      )}
      <div className="table">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Prefix</th>
              <th>Access</th>
              <th>Created</th>
              <th>Last used</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {apiKeys.map((apiKey) => (
              <tr key={apiKey.id}>
                <td>{apiKey.name}</td>
                <td>
                  <code>{apiKey.prefix}</code>
                </td>
                <td>{apiKey.certificates.join(", ")}</td>
                <td>{formatDate(apiKey.created_at)}</td>
                <td>{formatDate(apiKey.last_used_at)}</td>
                <td>
                  {!apiKey.revoked ? (
                    <button
                      className="danger"
                      onClick={() => void revoke(apiKey.id)}
                    >
                      Revoke
                    </button>
                  ) : (
                    "Revoked"
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {showCreateDialog && (
        <CreateAPIKeyDialog
          certificates={certificates}
          onClose={() => setShowCreateDialog(false)}
          onCreated={async (createdToken) => {
            setToken(createdToken);
            setShowCreateDialog(false);
            await reload();
          }}
        />
      )}
    </>
  );
}
