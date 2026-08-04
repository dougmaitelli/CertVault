export const appRoutes = {
  certificates: { label: "certificates", path: "/certificates" },
  acmeAccounts: { label: "ACME accounts", path: "/acme-accounts" },
  history: { label: "history", path: "/history" },
  auditLogs: { label: "audit logs", path: "/audit-logs" },
  apiKeys: { label: "api keys", path: "/api-keys" },
} as const;

export const navigationRoutes = Object.values(appRoutes);
