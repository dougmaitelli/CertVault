import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { APIKey, Certificate } from "../api/types";
import { CreateAPIKeyDialog } from "../dialogs/CreateAPIKeyDialog";
import { formatDate } from "../utils/date";

const copyFeedbackDuration = 2000;

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
  const [copied, setCopied] = useState(false);
  const copyFeedbackTimeout = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (copyFeedbackTimeout.current !== null) {
        window.clearTimeout(copyFeedbackTimeout.current);
      }
    },
    [],
  );

  async function copyToken() {
    await navigator.clipboard.writeText(token);
    setCopied(true);
    if (copyFeedbackTimeout.current !== null) {
      window.clearTimeout(copyFeedbackTimeout.current);
    }
    copyFeedbackTimeout.current = window.setTimeout(
      () => setCopied(false),
      copyFeedbackDuration,
    );
  }

  async function revoke(id: number) {
    if (confirm("Revoke this API key?")) {
      await api(`api-keys/${id}/revoke`, { method: "POST" });
      await reload();
    }
  }

  async function deleteKey(id: number) {
    if (confirm("Permanently delete this revoked API key?")) {
      await api(`api-keys/${id}`, { method: "DELETE" });
      await reload();
    }
  }

  return (
    <>
      <div className="bar">
        <p>Machine credentials are stored as hashes and shown only once.</p>
        <button
          className="action-button success"
          onClick={() => setShowCreateDialog(true)}
        >
          Create API key
        </button>
      </div>
      {token && (
        <div className="token">
          <b>Copy this token now — it cannot be shown again.</b>
          <code>{token}</code>
          <button
            className={`action-button ${copied ? "copied" : ""}`}
            onClick={() => void copyToken()}
            aria-live="polite"
          >
            {copied ? "Copied!" : "Copy"}
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
                      className="action-button danger"
                      onClick={() => void revoke(apiKey.id)}
                    >
                      Revoke
                    </button>
                  ) : (
                    <button
                      className="action-button delete"
                      onClick={() => void deleteKey(apiKey.id)}
                    >
                      Delete
                    </button>
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
