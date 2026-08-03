import { useEffect, useState } from "react";
import { api } from "./api/client";
import type { APIKey, Audit, Certificate, Job, Session } from "./api/types";
import { ConsoleLayout } from "./components/ConsoleLayout";
import { APIKeysPage } from "./pages/APIKeysPage";
import { AuditLogsPage } from "./pages/AuditLogsPage";
import { CertificatesPage } from "./pages/CertificatesPage";
import { HistoryPage } from "./pages/HistoryPage";
import { LoginPage } from "./pages/LoginPage";
import { pageFromPath, pageRoutes, type Page } from "./routing/routes";

export function App() {
  const [session, setSession] = useState<Session>();
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api<Session>("session")
      .then(setSession)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="splash">CertVault</div>;
  if (!session) return <LoginPage />;
  return <Console />;
}

function Console() {
  const [page, setPage] = useState<Page>(() =>
    pageFromPath(window.location.pathname),
  );
  const [certificates, setCertificates] = useState<Certificate[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [apiKeys, setAPIKeys] = useState<APIKey[]>([]);
  const [audits, setAudits] = useState<Audit[]>([]);
  const [error, setError] = useState("");

  const load = (): Promise<void> =>
    Promise.all([
      api<Certificate[]>("certificates"),
      api<Job[]>("jobs"),
      api<APIKey[]>("api-keys"),
      api<Audit[]>("audit"),
    ])
      .then(([loadedCertificates, loadedJobs, loadedAPIKeys, loadedAudits]) => {
        setCertificates(loadedCertificates);
        setJobs(loadedJobs);
        setAPIKeys(loadedAPIKeys);
        setAudits(loadedAudits);
        setError("");
      })
      .catch((caught) => setError(String(caught)));

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    const updatePage = () => setPage(pageFromPath(window.location.pathname));
    window.addEventListener("popstate", updatePage);
    return () => window.removeEventListener("popstate", updatePage);
  }, []);

  const navigate = (nextPage: Page) => {
    const path = pageRoutes[nextPage];
    if (window.location.pathname !== path) {
      window.history.pushState({}, "", path);
    }
    setPage(nextPage);
  };

  return (
    <ConsoleLayout page={page} error={error} onNavigate={navigate}>
      {page === "certificates" && (
        <CertificatesPage certificates={certificates} reload={load} />
      )}
      {page === "history" && <HistoryPage jobs={jobs} />}
      {page === "api keys" && (
        <APIKeysPage
          apiKeys={apiKeys}
          certificates={certificates}
          reload={load}
        />
      )}
      {page === "audit logs" && <AuditLogsPage audits={audits} />}
    </ConsoleLayout>
  );
}
