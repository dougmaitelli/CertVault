export function formatDate(value?: string): string {
  return value ? new Date(value).toLocaleString() : "Never";
}
