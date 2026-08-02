import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import "./style.css";

type Version = {
  id: number;
  not_before: string;
  not_after: string;
  created_at: string;
  domains: string[];
  serial: string;
  issuer: string;
  fingerprint_sha256: string;
};
type Cert = {
  name: string;
  domains: string[];
  key_type: string;
  status: string;
  last_error?: string;
  current_version?: Version;
};
type APIKey = {
  id: number;
  name: string;
  prefix: string;
  scopes: string[];
  certificates: string[];
  created_at: string;
  last_used_at?: string;
  revoked: boolean;
};
type Job = {
  id: number;
  certificate_name: string;
  kind: string;
  status: string;
  error: string;
  started_at: string;
  finished_at?: string;
};

type Session = {
  name: string;
  admin: boolean;
};

type APIKeyCreationResponse = {
  api_key: APIKey;
  token: string;
};

type Problem = {
  detail?: string;
};

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch("/api/v1/" + path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });
  if (!r.ok) {
    const payload: unknown = await r.json().catch(() => undefined);
    const problem = payload as Problem | undefined;
    throw new Error(problem?.detail ?? r.statusText);
  }
  if (r.status === 204) {
    return undefined as T;
  }
  const payload: unknown = await r.json();
  return payload as T;
}
const fmt = (v?: string) => (v ? new Date(v).toLocaleString() : "Never");

function Login() {
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    try {
      await fetch("/auth/bootstrap", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
      }).then((r) => {
        if (!r.ok) throw Error("Invalid token");
      });
      location.reload();
    } catch (e) {
      setError(String(e));
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
              onChange={(e) => setToken(e.target.value)}
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
function App() {
  const [session, setSession] = useState<Session>();
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    api<Session>("session")
      .then(setSession)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);
  if (loading) return <div className="splash">CertVault</div>;
  if (!session) return <Login />;
  return <Console />;
}
function Console() {
  const [page, setPage] = useState("certificates");
  const [certs, setCerts] = useState<Cert[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [error, setError] = useState("");
  const load = (): Promise<void> =>
    Promise.all([
      api<Cert[]>("certificates"),
      api<Job[]>("jobs"),
      api<APIKey[]>("api-keys"),
    ])
      .then(([c, j, k]) => {
        setCerts(c);
        setJobs(j);
        setKeys(k);
      })
      .catch((e) => setError(String(e)));
  useEffect(() => {
    void load();
  }, []);
  return (
    <div className="shell">
      <aside>
        <header>
          <span>CV</span>
          <b>CertVault</b>
        </header>
        <nav>
          {["certificates", "history", "api keys"].map((x) => (
            <button
              className={page === x ? "active" : ""}
              onClick={() => setPage(x)}
              key={x}
            >
              {x}
            </button>
          ))}
        </nav>
        <footer>
          <button
            onClick={() => {
              void fetch("/auth/logout", { method: "POST" }).then(() =>
                location.reload(),
              );
            }}
          >
            Sign out
          </button>
        </footer>
      </aside>
      <main>
        <div className="top">
          <div>
            <small>HOMELAB PKI</small>
            <h1>{page}</h1>
          </div>
          <div className="health">
            <i /> Operational
          </div>
        </div>
        {error && <div className="error">{error}</div>}
        {page === "certificates" && (
          <Certificates certs={certs} reload={load} />
        )}{" "}
        {page === "history" && <History jobs={jobs} />}{" "}
        {page === "api keys" && (
          <Keys keys={keys} certs={certs} reload={load} />
        )}
      </main>
    </div>
  );
}
function Certificates({
  certs,
  reload,
}: {
  certs: Cert[];
  reload: () => Promise<void>;
}) {
  const [selected, setSelected] = useState<Cert>();
  const renew = async (name: string) => {
    await api(`certificates/${name}/renew`, { method: "POST" });
    await reload();
  };
  return (
    <>
      <div className="stats">
        <Stat n={certs.length} label="Managed certificates" />
        <Stat
          n={certs.filter((c) => c.status === "valid").length}
          label="Healthy"
        />
        <Stat
          n={certs.filter((c) => c.status === "error").length}
          label="Needs attention"
        />
      </div>
      <div className="grid">
        {certs.map((c) => (
          <article key={c.name} onClick={() => setSelected(c)}>
            <div className="row">
              <h3>{c.name}</h3>
              <span className={"status " + c.status}>{c.status}</span>
            </div>
            <code>{c.domains.join(", ")}</code>
            <dl>
              <div>
                <dt>Expires</dt>
                <dd>{fmt(c.current_version?.not_after)}</dd>
              </div>
              <div>
                <dt>Key</dt>
                <dd>{c.key_type.toUpperCase()}</dd>
              </div>
            </dl>
            {c.last_error && <p className="error">{c.last_error}</p>}
            <div className="actions">
              <a href={`/api/v1/certificates/${c.name}/fullchain.pem`}>
                Full chain
              </a>
              <a href={`/api/v1/certificates/${c.name}/private-key.pem`}>
                Private key
              </a>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  void renew(c.name);
                }}
              >
                Renew
              </button>
            </div>
          </article>
        ))}
      </div>
      {selected && (
        <div className="modal" onClick={() => setSelected(undefined)}>
          <section onClick={(e) => e.stopPropagation()}>
            <button className="close" onClick={() => setSelected(undefined)}>
              ×
            </button>
            <small>CERTIFICATE DETAILS</small>
            <h2>{selected.name}</h2>
            <h4>Subject alternative names</h4>
            {selected.domains.map((d) => (
              <code className="domain" key={d}>
                {d}
              </code>
            ))}
            <h4>Validity</h4>
            <p>
              {fmt(selected.current_version?.not_before)} —{" "}
              {fmt(selected.current_version?.not_after)}
            </p>
            {selected.current_version && (
              <>
                <h4>Issuer and identity</h4>
                <p>{selected.current_version.issuer}</p>
                <code className="domain">
                  Serial: {selected.current_version.serial}
                </code>
                <code className="domain">
                  SHA-256: {selected.current_version.fingerprint_sha256}
                </code>
              </>
            )}
            <h4>Downloads</h4>
            <div className="actions">
              <a href={`/api/v1/certificates/${selected.name}/certificate.pem`}>
                Certificate
              </a>
              <a href={`/api/v1/certificates/${selected.name}/chain.pem`}>
                Chain
              </a>
              <a href={`/api/v1/certificates/${selected.name}/fullchain.pem`}>
                Full chain
              </a>
              <a href={`/api/v1/certificates/${selected.name}/private-key.pem`}>
                Private key
              </a>
            </div>
          </section>
        </div>
      )}
    </>
  );
}
function Stat({ n, label }: { n: number; label: string }) {
  return (
    <div>
      <strong>{n}</strong>
      <span>{label}</span>
    </div>
  );
}
function History({ jobs }: { jobs: Job[] }) {
  return (
    <div className="table">
      <table>
        <thead>
          <tr>
            <th>Certificate</th>
            <th>Operation</th>
            <th>Status</th>
            <th>Started</th>
            <th>Result</th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((j) => (
            <tr key={j.id}>
              <td>{j.certificate_name}</td>
              <td>{j.kind}</td>
              <td>
                <span className={"status " + j.status}>{j.status}</span>
              </td>
              <td>{fmt(j.started_at)}</td>
              <td>{j.error ? j.error : fmt(j.finished_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
function Keys({
  keys,
  certs,
  reload,
}: {
  keys: APIKey[];
  certs: Cert[];
  reload: () => Promise<void>;
}) {
  const [show, setShow] = useState(false);
  const [token, setToken] = useState("");
  const [name, setName] = useState("");
  async function create(e: React.FormEvent) {
    e.preventDefault();
    const out = await api<APIKeyCreationResponse>("api-keys", {
      method: "POST",
      body: JSON.stringify({
        name,
        scopes: ["certificates:read", "private_keys:read"],
        certificates: certs.map((c) => c.name),
      }),
    });
    setToken(out.token);
    setShow(false);
    await reload();
  }
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
        <button onClick={() => setShow(true)}>Create API key</button>
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
            {keys.map((k) => (
              <tr key={k.id}>
                <td>{k.name}</td>
                <td>
                  <code>{k.prefix}</code>
                </td>
                <td>{k.certificates.join(", ")}</td>
                <td>{fmt(k.created_at)}</td>
                <td>{fmt(k.last_used_at)}</td>
                <td>
                  {!k.revoked ? (
                    <button
                      className="danger"
                      onClick={() => void revoke(k.id)}
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
      {show && (
        <div className="modal">
          <section>
            <button className="close" onClick={() => setShow(false)}>
              ×
            </button>
            <h2>Create API key</h2>
            <form onSubmit={(event) => void create(event)}>
              <label>
                Name
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                  placeholder="nas-01"
                />
              </label>
              <p>
                This key can download certificates and private keys for all
                configured certificates.
              </p>
              <button>Create key</button>
            </form>
          </section>
        </div>
      )}
    </>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
