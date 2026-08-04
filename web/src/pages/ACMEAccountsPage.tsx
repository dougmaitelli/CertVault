import type { ACMEAccount } from "../api/types";
import { StatusBadge, StatusBadgeGroup } from "../components/StatusBadge";
import "./ACMEAccountsPage.css";

type ACMEAccountsPageProps = {
  accounts: ACMEAccount[];
};

export function ACMEAccountsPage({ accounts }: ACMEAccountsPageProps) {
  if (accounts.length === 0) {
    return (
      <div className="empty-state">
        No ACME accounts have been created. An account appears after the first
        certificate issuance attempt.
      </div>
    );
  }

  return (
    <div className="account-list">
      {accounts.map((account) => (
        <article className={account.current ? "current" : ""} key={account.id}>
          <div className="row">
            <h3>{accountHost(account.directory_url)}</h3>
            <StatusBadgeGroup>
              {account.current && (
                <StatusBadge status="valid" label="Current" />
              )}
              <StatusBadge status={account.status} />
            </StatusBadgeGroup>
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
