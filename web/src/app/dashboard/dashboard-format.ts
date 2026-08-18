export function formatDashboardTime(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "Never";
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
