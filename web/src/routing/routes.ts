export const defaultPage = "certificates";

export const pageRoutes = {
  certificates: "/certificates",
  history: "/history",
  "audit logs": "/audit-logs",
  "api keys": "/api-keys",
} as const;

export type Page = keyof typeof pageRoutes;

export function pageFromPath(path: string): Page {
  return (
    (Object.entries(pageRoutes).find(([, route]) => route === path)?.[0] as
      Page | undefined) ?? defaultPage
  );
}
