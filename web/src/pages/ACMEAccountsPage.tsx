import type { ACMEAccount } from "../api/types";

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
    <div className="account-grid">
      {accounts.map((account) => (
        <article className={account.current ? "current" : ""} key={account.id}>
          <div className="row">
            <h3>{accountHost(account.directory_url)}</h3>
            <div className="certificate-statuses">
              {account.current && <span className="status valid">Current</span>}
              <span className={`status ${account.status}`}>
                {account.status}
              </span>
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
