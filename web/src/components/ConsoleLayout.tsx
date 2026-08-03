import type { ReactNode } from "react";
import { pageRoutes, type Page } from "../routing/routes";

type ConsoleLayoutProps = {
  children: ReactNode;
  error: string;
  page: Page;
  onNavigate: (page: Page) => void;
};

export function ConsoleLayout({
  children,
  error,
  page,
  onNavigate,
}: ConsoleLayoutProps) {
  return (
    <div className="shell">
      <aside>
        <header>
          <span>CV</span>
          <b>CertVault</b>
        </header>
        <nav>
          {(Object.keys(pageRoutes) as Page[]).map((item) => (
            <a
              className={page === item ? "active" : ""}
              href={pageRoutes[item]}
              onClick={(event) => {
                event.preventDefault();
                onNavigate(item);
              }}
              key={item}
            >
              {item}
            </a>
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
            <small>CertVault</small>
            <h1>{page}</h1>
          </div>
          <div className="health">
            <i /> Operational
          </div>
        </div>
        {error && <div className="error">{error}</div>}
        {children}
      </main>
    </div>
  );
}
