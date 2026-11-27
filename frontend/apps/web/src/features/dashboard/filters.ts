import type { DashboardParams } from "./api";
import { customerTypes, type CustomerType } from "../customers/labels";
import type { Temperature } from "../sales/api";

export type Preset = "30d" | "90d" | "month" | "year" | "custom";

export const presetLabels: Record<Preset, string> = {
  "30d": "Últimos 30 días",
  "90d": "Últimos 90 días",
  month: "Este mes",
  year: "Este año",
  custom: "Personalizado",
};

export const presets = Object.keys(presetLabels) as Preset[];

/** What the person picked. It lives in the address, so a view can be shared and the back button works. */
export interface DashboardFilters {
  preset: Preset;
  /** Only read when the preset is "custom". */
  from?: string;
  to?: string;
  advisorId?: string;
  customerType?: CustomerType;
  temperature?: Temperature;
  modelId?: string;
}

export const DEFAULT_PRESET: Preset = "90d";

const DATE = /^\d{4}-\d{2}-\d{2}$/;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const TEMPERATURES: Temperature[] = ["hot", "warm", "cold"];

/** Today's calendar date in Lima, as YYYY-MM-DD. */
export function limaToday(now: Date = new Date()): string {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "America/Lima",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(now);
}

/** A calendar date moved by a number of days (dates are plain strings, so no time zone can shift them). */
export function addDays(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** The first and last day of a preset, relative to a day. */
export function presetRange(
  preset: Exclude<Preset, "custom">,
  today: string,
): { from: string; to: string } {
  switch (preset) {
    case "30d":
      return { from: addDays(today, -29), to: today };
    case "90d":
      return { from: addDays(today, -89), to: today };
    case "month":
      return { from: `${today.slice(0, 7)}-01`, to: today };
    case "year":
      return { from: `${today.slice(0, 4)}-01-01`, to: today };
  }
}

/** Reads the filters from the address, ignoring anything that is not valid. */
export function parseFilters(params: URLSearchParams): DashboardFilters {
  const rawPreset = params.get("period");
  const from = params.get("from");
  const to = params.get("to");
  const custom =
    rawPreset === "custom" &&
    from !== null &&
    to !== null &&
    DATE.test(from) &&
    DATE.test(to) &&
    from <= to;
  const preset: Preset = custom
    ? "custom"
    : presets.includes(rawPreset as Preset) && rawPreset !== "custom"
      ? (rawPreset as Preset)
      : DEFAULT_PRESET;
  const type = params.get("type");
  const temperature = params.get("temp");
  const advisor = params.get("advisor");
  const model = params.get("model");
  return {
    preset,
    from: custom ? from! : undefined,
    to: custom ? to! : undefined,
    advisorId: advisor && UUID.test(advisor) ? advisor : undefined,
    customerType: customerTypes.includes(type as CustomerType)
      ? (type as CustomerType)
      : undefined,
    temperature: TEMPERATURES.includes(temperature as Temperature)
      ? (temperature as Temperature)
      : undefined,
    modelId: model && UUID.test(model) ? model : undefined,
  };
}

/** Writes the filters into an address, leaving out whatever is the default. */
export function filtersToSearch(
  filters: DashboardFilters,
  into: URLSearchParams,
): URLSearchParams {
  const next = new URLSearchParams(into);
  for (const key of [
    "period",
    "from",
    "to",
    "advisor",
    "type",
    "temp",
    "model",
  ])
    next.delete(key);
  if (filters.preset !== DEFAULT_PRESET) next.set("period", filters.preset);
  if (filters.preset === "custom" && filters.from && filters.to) {
    next.set("from", filters.from);
    next.set("to", filters.to);
  }
  if (filters.advisorId) next.set("advisor", filters.advisorId);
  if (filters.customerType) next.set("type", filters.customerType);
  if (filters.temperature) next.set("temp", filters.temperature);
  if (filters.modelId) next.set("model", filters.modelId);
  return next;
}

/** The dates a filter stands for, given today. */
export function rangeOf(filters: DashboardFilters, today: string) {
  if (filters.preset === "custom" && filters.from && filters.to)
    return { from: filters.from, to: filters.to };
  return presetRange(
    filters.preset === "custom" ? DEFAULT_PRESET : filters.preset,
    today,
  );
}

/** The API query for a set of filters. An Advisor never sends an advisor: the server scopes them anyway. */
export function toParams(
  filters: DashboardFilters,
  today: string,
  canPickAdvisor: boolean,
): DashboardParams {
  const { from, to } = rangeOf(filters, today);
  return {
    from,
    to,
    advisorId: canPickAdvisor ? filters.advisorId : undefined,
    customerType: filters.customerType,
    temperature: filters.temperature,
    modelId: filters.modelId,
  };
}

/** True when anything other than the default period is chosen. */
export function isFiltered(filters: DashboardFilters): boolean {
  return (
    filters.preset !== DEFAULT_PRESET ||
    !!filters.advisorId ||
    !!filters.customerType ||
    !!filters.temperature ||
    !!filters.modelId
  );
}
