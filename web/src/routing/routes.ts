export const appRoutes = {
  certificates: { label: "certificates", path: "/certificates" },
  history: { label: "history", path: "/history" },
  acmeAccounts: { label: "ACME accounts", path: "/acme-accounts" },
  auditLogs: { label: "audit logs", path: "/audit-logs" },
  apiKeys: { label: "api keys", path: "/api-keys" },
} as const;

export const navigationRoutes = Object.values(appRoutes);
