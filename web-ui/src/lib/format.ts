export function fmtMoney(usd: number): string {
  if (usd === 0) return "$0";
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  return `$${usd.toFixed(2)}`;
}

export function fmtDuration(startISO?: string | null, endISO?: string | null): string {
  if (!startISO || !endISO) return "—";
  const ms = new Date(endISO).getTime() - new Date(startISO).getTime();
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60_000).toFixed(1)}m`;
}

export function statusVariant(s: string): "success" | "warning" | "danger" | "info" | "default" {
  switch (s) {
    case "completed":
      return "success";
    case "running":
      return "info";
    case "queued":
      return "warning";
    case "failed":
    case "cancelled":
      return "danger";
    default:
      return "default";
  }
}
