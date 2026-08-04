export function formatDate(value?: string): string {
  return value ? new Date(value).toLocaleString() : "Never";
}

const millisecondsPerDay = 24 * 60 * 60 * 1000;

export function formatRemainingValidity(value?: string): string {
  if (!value) {
    return "";
  }

  const remaining = new Date(value).getTime() - Date.now();
  if (!Number.isFinite(remaining)) {
    return "";
  }
  if (remaining <= 0) {
    return "(expired)";
  }

  const days = Math.ceil(remaining / millisecondsPerDay);
  return `(${days} ${days === 1 ? "day" : "days"})`;
}
