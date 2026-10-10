import { useCallback, useEffect, useRef, useState } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router";
import "./App.css";
import { api, APIError, onAuthenticationExpired } from "./api/client";
import type {
  ACMEAccount,
  APIKey,
  Certificate,
  Health,
  Session,
} from "./api/types";
import { ConsoleLayout } from "./components/ConsoleLayout";
import { APIKeysPage } from "./pages/APIKeysPage";
import { ACMEAccountsPage } from "./pages/ACMEAccountsPage";
import { AuditLogsPage } from "./pages/AuditLogsPage";
import { CertificatesPage } from "./pages/CertificatesPage";
import { HistoryPage } from "./pages/HistoryPage";
import { LoginPage } from "./pages/LoginPage";
import { useSerialPolling } from "./hooks/useSerialPolling";
import { appRoutes } from "./routing/routes";

const statusRefreshInterval = 30_000;
const taskRefreshInterval = 2_000;
const resourceLabels = {
  certificates: "certificates",
  "api-keys": "API keys",
  "acme-accounts": "ACME accounts",
};
type DataResource = keyof typeof resourceLabels;

export function App() {
  const [session, setSession] = useState<Session>();
  const [loading, setLoading] = useState(true);
  const [sessionNotice, setSessionNotice] = useState("");
  const location = useLocation();

  useEffect(() => {
    const controller = new AbortController();
    api<Session>("session", { signal: controller.signal })
      .then((loaded) => {
        if (!controller.signal.aborted) setSession(loaded);
      })
      .catch((caught: unknown) => {
        if (
          !controller.signal.aborted &&
          !(caught instanceof APIError && caught.status === 401)
        ) {
          setSessionNotice(`Unable to check your session: ${String(caught)}`);
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!session) return;
    return onAuthenticationExpired(() => {
      setSession(undefined);
      setSessionNotice("Your session expired. Sign in again.");
    });
  }, [session]);

  if (loading) return <div className="splash">CertVault</div>;
  if (!session) {
    if (location.pathname !== "/") return <Navigate to="/" replace />;
    return (
      <LoginPage
        notice={sessionNotice}
        onAuthenticated={async () => {
          setSession(await api<Session>("session"));
          setSessionNotice("");
        }}
      />
    );
  }
  return (
    <Console
      session={session}
      onLogout={() => {
        setSession(undefined);
        setSessionNotice("");
      }}
    />
  );
}

type ConsoleProps = {
  session: Session;
  onLogout: () => void;
};

function Console({ session, onLogout }: ConsoleProps) {
  const location = useLocation();
  const requests = useRef(new Map<DataResource, AbortController>());
  const [certificates, setCertificates] = useState<Certificate[]>([]);
  const [apiKeys, setAPIKeys] = useState<APIKey[]>([]);
  const [acmeAccounts, setACMEAccounts] = useState<ACMEAccount[]>([]);
  const [dataErrors, setDataErrors] = useState<
    Partial<Record<DataResource, string>>
  >({});
  const [certificatesLoaded, setCertificatesLoaded] = useState(false);
  const [appVersion, setAppVersion] = useState("dev");
  const [infrastructureHealthy, setInfrastructureHealthy] = useState<
    boolean | undefined
  >();

  const refreshResource = useCallback(
    async <T,>(
      resource: DataResource,
      setData: (data: T) => void,
      signal?: AbortSignal,
    ): Promise<void> => {
      // A mutation or route refresh supersedes an older request for this resource.
      requests.current.get(resource)?.abort();
      const controller = new AbortController();
      requests.current.set(resource, controller);
      const requestSignal = AbortSignal.any([
        controller.signal,
        AbortSignal.timeout(15_000),
        ...(signal ? [signal] : []),
      ]);
      try {
        const data = await api<T>(resource, { signal: requestSignal });
        if (requestSignal.aborted) return;
        setData(data);
        if (resource === "certificates") setCertificatesLoaded(true);
        setDataErrors((current) => ({ ...current, [resource]: "" }));
      } catch (caught) {
        if (controller.signal.aborted || signal?.aborted) return;
        setDataErrors((current) => ({
          ...current,
          [resource]: `Unable to refresh ${resourceLabels[resource]}; displayed data may be outdated. ${String(caught)}`,
        }));
      } finally {
        if (requests.current.get(resource) === controller)
          requests.current.delete(resource);
      }
    },
    [],
  );

  const refreshCertificates = useCallback(
    (signal?: AbortSignal) =>
      refreshResource("certificates", setCertificates, signal),
    [refreshResource],
  );
  const refreshAPIKeys = useCallback(
    (signal?: AbortSignal) => refreshResource("api-keys", setAPIKeys, signal),
    [refreshResource],
  );
  const refreshAccounts = useCallback(
    (signal?: AbortSignal) =>
      refreshResource("acme-accounts", setACMEAccounts, signal),
    [refreshResource],
  );

  useEffect(() => {
    const pending = requests.current;
    return () => {
      pending.forEach((controller) => controller.abort());
    };
  }, []);

  const checkInfrastructure = useCallback(async (signal: AbortSignal) => {
    const requestSignal = AbortSignal.any([
      signal,
      AbortSignal.timeout(15_000),
    ]);
    try {
      const [health] = await Promise.all([
        api<Health>("health", { signal: requestSignal }),
        api("ready", { signal: requestSignal }),
        api<Session>("session", { signal: requestSignal }),
      ]);
      if (requestSignal.aborted) return;
      setAppVersion(health.version);
      setInfrastructureHealthy(true);
    } catch {
      if (!signal.aborted) setInfrastructureHealthy(false);
    }
  }, []);
  useSerialPolling(checkInfrastructure, statusRefreshInterval);

  const activeIssuance = certificates.some((certificate) =>
    ["queued", "running"].includes(certificate.latest_job?.status ?? ""),
  );
  const needsCertificates = [
    appRoutes.certificates.path,
    appRoutes.apiKeys.path,
  ].some((path) => path === location.pathname);
  useSerialPolling(
    refreshCertificates,
    activeIssuance
      ? taskRefreshInterval
      : needsCertificates || !certificatesLoaded
        ? statusRefreshInterval
        : null,
  );
  useSerialPolling(
    refreshAPIKeys,
    location.pathname === appRoutes.apiKeys.path ? statusRefreshInterval : null,
  );
  useSerialPolling(
    refreshAccounts,
    location.pathname === appRoutes.acmeAccounts.path
      ? statusRefreshInterval
      : null,
  );

  const error = Object.values(dataErrors).filter(Boolean).join(" ");

  const health =
    infrastructureHealthy === undefined
      ? "checking"
      : !infrastructureHealthy
        ? "failed"
        : error ||
            certificates.some((certificate) => certificate.status === "error")
          ? "warning"
          : !certificatesLoaded
            ? "checking"
            : "operational";

  return (
    <Routes>
      <Route
        element={
          <ConsoleLayout
            error={error}
            health={health}
            version={appVersion}
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
              reload={refreshCertificates}
            />
          }
        />
        <Route path={appRoutes.history.path} element={<HistoryPage />} />
        <Route
          path={appRoutes.acmeAccounts.path}
          element={
            <ACMEAccountsPage
              accounts={acmeAccounts}
              reload={refreshAccounts}
            />
          }
        />
        <Route path={appRoutes.auditLogs.path} element={<AuditLogsPage />} />
        <Route
          path={appRoutes.apiKeys.path}
          element={
            <APIKeysPage
              apiKeys={apiKeys}
              certificates={certificates}
              reload={refreshAPIKeys}
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
