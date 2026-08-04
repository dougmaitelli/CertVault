import { NavLink, Outlet, useLocation } from "react-router-dom";
import type { Session } from "../api/types";
import { navigationRoutes } from "../routing/routes";

type ConsoleLayoutProps = {
  error: string;
  health: "checking" | "operational" | "warning" | "failed";
  session: Session;
  onLogout: () => void;
};

export function ConsoleLayout({
  error,
  health,
  session,
  onLogout,
}: ConsoleLayoutProps) {
  const currentLocation = useLocation();
  const pageTitle =
    navigationRoutes.find((route) => route.path === currentLocation.pathname)
      ?.label ?? "CertVault";

  return (
    <div className="shell">
      <aside>
        <header>
          <span>CV</span>
          <b>CertVault</b>
        </header>
        <nav>
          {navigationRoutes.map((route) => (
            <NavLink
              className={({ isActive }) => (isActive ? "active" : "")}
              to={route.path}
              key={route.path}
            >
              {route.label}
            </NavLink>
          ))}
        </nav>
        <footer>
          <div className="user-profile">
            {session.picture ? (
              <img src={session.picture} alt="" referrerPolicy="no-referrer" />
            ) : (
              <span className="user-avatar" aria-hidden="true">
                {session.name.charAt(0).toUpperCase()}
              </span>
            )}
            <div>
              <strong>{session.name}</strong>
              {session.email && session.email !== session.name && (
                <small>{session.email}</small>
              )}
            </div>
          </div>
          <button
            onClick={() => {
              void fetch("/auth/logout", { method: "POST" }).then(
                (response) => {
                  if (response.ok) onLogout();
                },
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
            <small>CertVault</small>
            <h1>{pageTitle}</h1>
          </div>
          <div className={`health ${health}`} role="status" aria-live="polite">
            <i /> {healthLabel(health)}
          </div>
        </div>
        {error && <div className="error">{error}</div>}
        <Outlet />
      </main>
    </div>
  );
}

function healthLabel(health: ConsoleLayoutProps["health"]): string {
  switch (health) {
    case "checking":
      return "Checking";
    case "warning":
      return "Warning";
    case "failed":
      return "Failed";
    default:
      return "Operational";
  }
}
