import { useState } from "react";
import { api } from "../api/client";
import type { ACMEAccount } from "../api/types";
import { StatusBadge, StatusBadgeGroup } from "../components/StatusBadge";
import { ConfirmationDialog } from "../dialogs/ConfirmationDialog";
import "./ACMEAccountsPage.css";

type ACMEAccountsPageProps = {
  accounts: ACMEAccount[];
  reload: () => Promise<void>;
};

export function ACMEAccountsPage({ accounts, reload }: ACMEAccountsPageProps) {
  const [accountToDelete, setAccountToDelete] = useState<ACMEAccount>();

  async function deleteAccount(account: ACMEAccount) {
    await api(`acme-accounts/${encodeURIComponent(account.id)}`, {
      method: "DELETE",
    });
    await reload();
  }

  if (accounts.length === 0) {
    return (
      <div className="empty-state">
        No ACME accounts have been created. An account appears after the first
        certificate issuance attempt.
      </div>
    );
  }

  return (
    <>
      <div className="account-list">
        {accounts.map((account) => (
          <article
            className={account.current ? "current" : ""}
            key={account.id}
          >
            <div className="row">
              <h3>{accountHost(account.directory_url)}</h3>
              <div className="account-actions">
                <StatusBadgeGroup>
                  {account.current && (
                    <StatusBadge status="valid" label="Current" />
                  )}
                  <StatusBadge status={account.status} />
                </StatusBadgeGroup>
                {!account.current && (
                  <button
                    className="action-button delete"
                    onClick={() => setAccountToDelete(account)}
                  >
                    Delete
                  </button>
                )}
              </div>
            </div>
            <dl>
              <div>
                <dt>Email</dt>
                <dd>{account.email}</dd>
              </div>
              <div>
                <dt>Directory URL</dt>
                <dd>
                  <code>
                    {account.directory_url ?? "Unknown (legacy account)"}
                  </code>
                </dd>
              </div>
              <div>
                <dt>Registration URL</dt>
                <dd>
                  <code>{account.registration_url ?? "Not registered"}</code>
                </dd>
              </div>
            </dl>
          </article>
        ))}
      </div>
      {accountToDelete && (
        <ConfirmationDialog
          title="Permanently delete ACME account?"
          message={`This will remove the locally stored registration and private account key for ${accountHost(accountToDelete.directory_url)}. Existing certificates will remain unchanged. This action cannot be undone.`}
          confirmLabel="Delete account"
          onClose={() => setAccountToDelete(undefined)}
          onConfirm={() => deleteAccount(accountToDelete)}
        />
      )}
    </>
  );
}

function accountHost(directoryURL?: string): string {
  if (!directoryURL) return "Legacy ACME account";
  try {
    return new URL(directoryURL).hostname;
  } catch {
    return directoryURL;
  }
}
