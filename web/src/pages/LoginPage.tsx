import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import "./LoginPage.css";
import type { AuthenticationMethods } from "../api/types";

type LoginPageProps = {
  onAuthenticated: () => Promise<void>;
};

export function LoginPage({ onAuthenticated }: LoginPageProps) {
  const [methods, setMethods] = useState<AuthenticationMethods>();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    void fetch("/auth/methods")
      .then((response) => {
        if (!response.ok)
          throw new Error("Unable to load authentication methods");
        return response.json() as Promise<AuthenticationMethods>;
      })
      .then(setMethods)
      .catch((caught) => setError(String(caught)));
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    try {
      await fetch("/auth/bootstrap", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
      }).then((response) => {
        if (!response.ok) throw Error("Invalid token");
      });
      await onAuthenticated();
    } catch (caught) {
      setError(String(caught));
    }
  }

  const bootstrapForm = (
    <form onSubmit={(event) => void submit(event)}>
      <label>
        Break-glass administrator token
        <input
          type="password"
          value={token}
          onChange={(event) => setToken(event.target.value)}
          autoFocus={!methods?.oidc}
          required
        />
      </label>
      <button className="action-button success">Sign in</button>
    </form>
  );

  return (
    <main className="login">
      <section>
        <div className="brand-logo">CV</div>
        <h1>CertVault</h1>
        <p>Central certificate management for your network.</p>

        {methods?.oidc && (
          <a className="action-button success oidc" href="/auth/login">
            Sign in with OIDC
          </a>
        )}

        {methods?.bootstrap && methods.oidc ? (
          <details className="break-glass">
            <summary>Use break-glass token</summary>
            {bootstrapForm}
          </details>
        ) : (
          methods?.bootstrap && bootstrapForm
        )}

        {methods && !methods.oidc && !methods.bootstrap && (
          <div className="error">No authentication method is configured.</div>
        )}
        {error && <div className="error">{error}</div>}

        <a
          className="source-link"
          href="https://github.com/dougmaitelli/CertVault"
          target="_blank"
          rel="noreferrer"
        >
          Source · AGPL-3.0
        </a>
      </section>
    </main>
  );
}
