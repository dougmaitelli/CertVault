import { useEffect, useState } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import "./App.css";
import { api } from "./api/client";
import type {
  ACMEAccount,
  APIKey,
  Certificate,
  Job,
  Session,
} from "./api/types";
import { ConsoleLayout } from "./components/ConsoleLayout";
import { APIKeysPage } from "./pages/APIKeysPage";
import { ACMEAccountsPage } from "./pages/ACMEAccountsPage";
import { AuditLogsPage } from "./pages/AuditLogsPage";
import { CertificatesPage } from "./pages/CertificatesPage";
import { HistoryPage } from "./pages/HistoryPage";
import { LoginPage } from "./pages/LoginPage";
import { appRoutes } from "./routing/routes";

const statusRefreshInterval = 30_000;
const taskRefreshInterval = 2_000;

export function App() {
  const [session, setSession] = useState<Session>();
  const [loading, setLoading] = useState(true);
  const location = useLocation();

  useEffect(() => {
    api<Session>("session")
      .then(setSession)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="splash">CertVault</div>;
  if (!session) {
    if (location.pathname !== "/") return <Navigate to="/" replace />;
    return (
      <LoginPage
        onAuthenticated={async () => setSession(await api<Session>("session"))}
      />
    );
  }
  return <Console session={session} onLogout={() => setSession(undefined)} />;
}

type ConsoleProps = {
  session: Session;
  onLogout: () => void;
};

function Console({ session, onLogout }: ConsoleProps) {
  const [certificates, setCertificates] = useState<Certificate[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [apiKeys, setAPIKeys] = useState<APIKey[]>([]);
  const [acmeAccounts, setACMEAccounts] = useState<ACMEAccount[]>([]);
  const [error, setError] = useState("");
  const [infrastructureHealthy, setInfrastructureHealthy] = useState<
    boolean | undefined
  >();

  const load = (): Promise<void> =>
    Promise.all([
      api<Certificate[]>("certificates"),
      api<Job[]>("jobs"),
      api<APIKey[]>("api-keys"),
      api<ACMEAccount[]>("acme-accounts"),
    ])
      .then(
        ([
          loadedCertificates,
          loadedJobs,
          loadedAPIKeys,
          loadedACMEAccounts,
        ]) => {
          setCertificates(loadedCertificates);
          setJobs(loadedJobs);
          setAPIKeys(loadedAPIKeys);
          setACMEAccounts(loadedACMEAccounts);
          setError("");
        },
      )
      .catch((caught) => setError(String(caught)));

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    const checkInfrastructure = () => {
      void Promise.all([api("health"), api("ready")])
        .then(() => setInfrastructureHealthy(true))
        .catch(() => setInfrastructureHealthy(false));
      void api<ACMEAccount[]>("acme-accounts")
        .then(setACMEAccounts)
        .catch(() => {});
    };

    checkInfrastructure();
    const interval = window.setInterval(
      checkInfrastructure,
      statusRefreshInterval,
    );
    return () => window.clearInterval(interval);
  }, []);

  useEffect(() => {
    const refreshTasks = () => {
      void Promise.all([api<Certificate[]>("certificates"), api<Job[]>("jobs")])
        .then(([loadedCertificates, loadedJobs]) => {
          setCertificates(loadedCertificates);
          setJobs(loadedJobs);
        })
        .catch(() => {});
    };

    const interval = window.setInterval(refreshTasks, taskRefreshInterval);
    return () => window.clearInterval(interval);
  }, []);

  const health =
    infrastructureHealthy === undefined
      ? "checking"
      : !infrastructureHealthy
        ? "failed"
        : certificates.some((certificate) => certificate.status === "error")
          ? "warning"
          : "operational";

  return (
    <Routes>
      <Route
        element={
          <ConsoleLayout
            error={error}
            health={health}
            session={session}
            onLogout={onLogout}
          />
        }
      >
        <Route
          index
          element={<Navigate to={appRoutes.certificates.path} replace />}
        />
        <Route
          path={appRoutes.certificates.path}
          element={
            <CertificatesPage
              certificates={certificates}
              jobs={jobs}
              reload={load}
            />
          }
        />
        <Route
          path={appRoutes.history.path}
          element={<HistoryPage jobs={jobs} />}
        />
        <Route
          path={appRoutes.acmeAccounts.path}
          element={<ACMEAccountsPage accounts={acmeAccounts} reload={load} />}
        />
        <Route path={appRoutes.auditLogs.path} element={<AuditLogsPage />} />
        <Route
          path={appRoutes.apiKeys.path}
          element={
            <APIKeysPage
              apiKeys={apiKeys}
              certificates={certificates}
              reload={load}
            />
          }
        />
        <Route
          path="*"
          element={<Navigate to={appRoutes.certificates.path} replace />}
        />
      </Route>
    </Routes>
  );
}
