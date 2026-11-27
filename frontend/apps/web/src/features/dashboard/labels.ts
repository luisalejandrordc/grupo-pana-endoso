import type { Role } from "../../shared/types";
import type { TabId } from "./queries";

export const tabLabels: Record<TabId, string> = {
  summary: "Resumen",
  process: "Proceso",
  market: "Mercado",
  finance: "Finanzas",
  advisors: "Asesores",
  system: "Sistema",
};

/** The tabs a role sees, in order. Advisors never see the team or the system. */
export function tabsFor(role: Role): TabId[] {
  const all: TabId[] = ["summary", "process", "market", "finance"];
  if (role !== "sales_advisor") all.push("advisors");
  if (role === "system_admin") all.push("system");
  return all;
}

export const attentionLabels = {
  overdue: "Con retraso",
  approaching: "Por vencer",
  stalled: "Sin iniciar",
} as const;

export const invoiceLabels: Record<string, string> = {
  toyota: "Toyota",
  commission: "Comisión",
  accessories: "Accesorios",
  release_processing: "Trámite de liberación",
};

/** "2026-11" as "nov 26", for chart axes. */
export function monthLabel(month: string): string {
  const [year, m] = month.split("-");
  const date = new Date(Date.UTC(Number(year), Number(m) - 1, 1));
  const name = new Intl.DateTimeFormat("es-PE", {
    month: "short",
    timeZone: "UTC",
  }).format(date);
  return `${name.replace(".", "")} ${year.slice(2)}`;
}

/** A percentage with one decimal, or a dash. */
export function percent(value: number | null | undefined): string {
  if (value === null || value === undefined) return "—";
  return `${value.toLocaleString("es-PE", { maximumFractionDigits: 1 })} %`;
}

/** A number of hours as a short human text: minutes, hours or days. */
export function durationText(hours: number | null | undefined): string {
  if (hours === null || hours === undefined) return "—";
  if (hours < 1) return `${Math.round(hours * 60)} min`;
  if (hours < 48)
    return `${hours.toLocaleString("es-PE", { maximumFractionDigits: 1 })} h`;
  return `${(hours / 24).toLocaleString("es-PE", { maximumFractionDigits: 1 })} d`;
}

export function daysText(days: number | null | undefined): string {
  if (days === null || days === undefined) return "—";
  return `${days.toLocaleString("es-PE", { maximumFractionDigits: 1 })} d`;
}

/** "2026-11-22" as "22 nov 2026". The text is a plain date, so it is formatted in UTC and no zone can shift it. */
export function dayText(date: string): string {
  return new Intl.DateTimeFormat("es-PE", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(`${date}T00:00:00Z`));
}
