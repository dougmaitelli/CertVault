import type { Session } from "../api/types";
import "./UserSummary.css";

type UserSummaryProps = {
  session: Session;
  onLogout: () => void;
};

export function UserSummary({ session, onLogout }: UserSummaryProps) {
  function logout() {
    void fetch("/auth/logout", { method: "POST" }).then((response) => {
      if (response.ok) {
        onLogout();
      }
    });
  }

  return (
    <div className="user-summary">
      {session.picture ? (
        <img src={session.picture} alt="" referrerPolicy="no-referrer" />
      ) : (
        <span className="user-summary-avatar" aria-hidden="true">
          {session.name.charAt(0).toUpperCase()}
        </span>
      )}
      <div className="user-summary-identity">
        <strong>{session.name}</strong>
        {session.email && session.email !== session.name && (
          <small>{session.email}</small>
        )}
      </div>
      <button aria-label="Sign out" title="Sign out" onClick={logout}>
        <svg
          aria-hidden="true"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M10 17l5-5-5-5" />
          <path d="M15 12H3" />
          <path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4" />
        </svg>
      </button>
    </div>
  );
}
