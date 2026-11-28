import { useCallback, useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import type { Role } from "../../shared/types";
import type { CustomerType, SalesFilters, Temperature } from "./api";
import type { TabId } from "./labels";

export type PeriodId = "30d" | "90d" | "year" | "lastYear" | "custom";

export const periodLabels: Record<PeriodId, string> = {
  "30d": "Últimos 30 días",
  "90d": "Últimos 90 días",
  year: "Este año",
  lastYear: "Año anterior",
  custom: "Personalizado",
};

export const periodIds = Object.keys(periodLabels) as PeriodId[];

/** Everything the filter bar can hold, as it lives in the address. */
export interface FilterState {
  tab: TabId;
  period: PeriodId;
  from: string;
  to: string;
  advisor: string;
  customerType: string;
  model: string;
  temperature: string;
}

/** A change to the filters; an empty text removes that parameter from the address. */
export type FilterPatch = { [K in keyof FilterState]?: string };

const LIMA_OFFSET_MS = 5 * 60 * 60 * 1000;
const DATE = /^\d{4}-\d{2}-\d{2}$/;

/** The Lima calendar date of an instant, as "YYYY-MM-DD". Lima has no daylight saving time. */
export function limaDate(instant: Date): string {
  return new Date(instant.getTime() - LIMA_OFFSET_MS)
    .toISOString()
    .slice(0, 10);
}

function shiftDays(date: string, days: number): string {
  const d = new Date(`${date}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** The two dates a preset stands for, counted from today in Lima. */
export function periodDates(
  period: PeriodId,
  today: string,
  from: string,
  to: string,
): { from?: string; to?: string } {
  const year = Number(today.slice(0, 4));
  switch (period) {
    case "30d":
      return { from: shiftDays(today, -29), to: today };
    case "90d":
      return {}; // the server's default
    case "year":
      return { from: `${year}-01-01`, to: today };
    case "lastYear":
      return { from: `${year - 1}-01-01`, to: `${year - 1}-12-31` };
    case "custom":
      return {
        from: DATE.test(from) ? from : undefined,
        to: DATE.test(to) ? to : undefined,
      };
  }
}

/** The tabs a role may open, in order. */
export function tabsFor(role: Role): TabId[] {
  if (role === "system_admin")
    return ["summary", "process", "commercial", "team", "system"];
  if (role === "sales_manager")
    return ["summary", "process", "commercial", "team"];
  return ["summary", "process", "commercial"];
}

function parseTab(value: string | null, role: Role): TabId {
  const allowed = tabsFor(role);
  return allowed.find((tab) => tab === value) ?? "summary";
}

function parsePeriod(value: string | null): PeriodId {
  return periodIds.find((id) => id === value) ?? "90d";
}

/** Reads the filters from the address and writes changes back, so a view can be bookmarked and shared. */
export function useAnalyticsFilters(role: Role) {
  const [params, setParams] = useSearchParams();

  const state: FilterState = useMemo(
    () => ({
      tab: parseTab(params.get("tab"), role),
      period: parsePeriod(params.get("period")),
      from: params.get("from") ?? "",
      to: params.get("to") ?? "",
      advisor: params.get("advisor") ?? "",
      customerType: params.get("customerType") ?? "",
      model: params.get("model") ?? "",
      temperature: params.get("temperature") ?? "",
    }),
    [params, role],
  );

  const update = useCallback(
    (patch: FilterPatch) => {
      setParams(
        (current) => {
          const next = new URLSearchParams(current);
          for (const [key, value] of Object.entries(patch)) {
            if (value === "" || value === undefined) next.delete(key);
            else next.set(key, value);
          }
          return next;
        },
        { replace: true },
      );
    },
    [setParams],
  );

  return { state, update };
}

/** The query that goes to the API for the sales tabs. */
export function salesQuery(state: FilterState, today: string): SalesFilters {
  return {
    ...periodDates(state.period, today, state.from, state.to),
    advisorId: state.advisor || undefined,
    customerType: (state.customerType || undefined) as CustomerType | undefined,
    modelId: state.model || undefined,
    temperature: (state.temperature || undefined) as Temperature | undefined,
  };
}

/** Whether a custom range is complete and in order, so a half-typed one does not reach the server. */
export function customRangeIsValid(state: FilterState): boolean {
  if (state.period !== "custom") return true;
  return DATE.test(state.from) && DATE.test(state.to) && state.from <= state.to;
}
