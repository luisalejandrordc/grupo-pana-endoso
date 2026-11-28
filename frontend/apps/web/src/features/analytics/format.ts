/** Number formatting for the dashboard. Everything is Spanish (Peru), and a missing value is a dash. */

const integer = new Intl.NumberFormat("es-PE");
const one = new Intl.NumberFormat("es-PE", { maximumFractionDigits: 1 });

export function formatCount(value: number): string {
  return integer.format(value);
}

/** A ratio from 0 to 1 as a percentage; null (no denominator) is a dash. */
export function formatPercent(value: number | null | undefined): string {
  if (value === null || value === undefined) return "—";
  return `${one.format(value * 100)} %`;
}

/** Hours up to two days, days beyond: "5,5 h", "3,2 d". */
export function formatDuration(hours: number | null | undefined): string {
  if (hours === null || hours === undefined) return "—";
  if (hours < 48) return `${one.format(hours)} h`;
  return `${one.format(hours / 24)} d`;
}

/** The size of a delay or a wait, rounded for a list: "3 h", "2 d". */
export function formatSpan(hours: number): string {
  if (hours < 48) return `${Math.max(1, Math.round(hours))} h`;
  return `${Math.round(hours / 24)} d`;
}

export type Trend = "up" | "down" | "flat";

export interface Change {
  trend: Trend;
  /** "+12 %", "−3 %", "Sin cambio"; null when there is no earlier figure to compare with. */
  text: string | null;
}

/** How a figure moved against the previous period. */
export function change(value: number, previous: number): Change {
  if (previous === 0) {
    if (value === 0) return { trend: "flat", text: null };
    return { trend: "up", text: "Nuevo" };
  }
  const delta = ((value - previous) / Math.abs(previous)) * 100;
  if (Math.abs(delta) < 0.5) return { trend: "flat", text: "Sin cambio" };
  const sign = delta > 0 ? "+" : "−";
  return {
    trend: delta > 0 ? "up" : "down",
    text: `${sign}${one.format(Math.abs(delta))} %`,
  };
}

/** The same for decimal text amounts such as "1250.00". */
export function changeMoney(value: string, previous: string): Change {
  return change(Number(value), Number(previous));
}

/** A short axis label for money: 1,2 mil, 3,4 M. */
export function compactMoney(value: number): string {
  const abs = Math.abs(value);
  if (abs >= 1_000_000) return `$${one.format(value / 1_000_000)} M`;
  if (abs >= 1_000) return `$${one.format(value / 1_000)} mil`;
  return `$${one.format(value)}`;
}

/** "2026-03-01" → "1 mar", or "mar 2026" for month buckets. */
export function bucketLabel(
  bucket: string,
  granularity: "day" | "week" | "month",
): string {
  const date = new Date(`${bucket}T12:00:00Z`);
  if (granularity === "month") {
    return new Intl.DateTimeFormat("es-PE", {
      month: "short",
      year: "2-digit",
      timeZone: "UTC",
    })
      .format(date)
      .replace(".", "");
  }
  return new Intl.DateTimeFormat("es-PE", {
    day: "numeric",
    month: "short",
    timeZone: "UTC",
  })
    .format(date)
    .replace(".", "");
}

/** "2026-03-01" → "1 mar 2026". The text is a calendar date, so no time zone may move it. */
export function dateLabel(date: string): string {
  return new Intl.DateTimeFormat("es-PE", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  })
    .format(new Date(`${date}T12:00:00Z`))
    .replace(/\./g, "");
}
