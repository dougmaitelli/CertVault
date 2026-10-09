import { useCallback, useEffect, useState } from "react";
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
      try {
        const data = await api<T>(resource, { signal });
        if (signal?.aborted) return;
        setData(data);
        if (resource === "certificates") setCertificatesLoaded(true);
        setDataErrors((current) => ({ ...current, [resource]: "" }));
      } catch (caught) {
        if (signal?.aborted) return;
        setDataErrors((current) => ({
          ...current,
          [resource]: `Unable to refresh ${resourceLabels[resource]}; displayed data may be outdated. ${String(caught)}`,
        }));
      }
    },
    [],
  );

  const load = useCallback(
    (signal?: AbortSignal): Promise<void> =>
      Promise.all([
        refreshResource("certificates", setCertificates, signal),
        refreshResource("api-keys", setAPIKeys, signal),
        refreshResource("acme-accounts", setACMEAccounts, signal),
      ]).then(() => {}),
    [refreshResource],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    const controller = new AbortController();
    const checkInfrastructure = () => {
      void Promise.all([
        api<Health>("health", { signal: controller.signal }),
        api("ready", { signal: controller.signal }),
      ])
        .then(([health]) => {
          if (controller.signal.aborted) return;
          setAppVersion(health.version);
          setInfrastructureHealthy(true);
        })
        .catch(() => {
          if (!controller.signal.aborted) setInfrastructureHealthy(false);
        });
      void refreshResource("acme-accounts", setACMEAccounts, controller.signal);
    };

    checkInfrastructure();
    const interval = window.setInterval(
      checkInfrastructure,
      statusRefreshInterval,
    );
    return () => {
      controller.abort();
      window.clearInterval(interval);
    };
  }, [refreshResource]);

  useEffect(() => {
    const controller = new AbortController();
    const refreshTasks = () => {
      void refreshResource("certificates", setCertificates, controller.signal);
    };

    const interval = window.setInterval(refreshTasks, taskRefreshInterval);
    return () => {
      controller.abort();
      window.clearInterval(interval);
    };
  }, [refreshResource]);

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
            <CertificatesPage certificates={certificates} reload={load} />
          }
        />
        <Route path={appRoutes.history.path} element={<HistoryPage />} />
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
