import { useState, type FormEvent } from "react";

export function LoginPage() {
  const [token, setToken] = useState("");
  const [error, setError] = useState("");

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
      location.reload();
    } catch (caught) {
      setError(String(caught));
    }
  }

  return (
    <main className="login">
      <section>
        <div className="brand">CV</div>
        <h1>CertVault</h1>
        <p>Central certificate management for your network.</p>
        <form onSubmit={(event) => void submit(event)}>
          <label>
            Bootstrap administrator token
            <input
              type="password"
              value={token}
              onChange={(event) => setToken(event.target.value)}
              autoFocus
            />
          </label>
          {error && <div className="error">{error}</div>}
          <button>Sign in</button>
        </form>
        <a className="oidc" href="/auth/login">
          Sign in with OIDC
        </a>
      </section>
    </main>
  );
}
