import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  analyticsApi,
  type GroupBy,
  type PeriodOnly,
  type SalesFilters,
} from "./api";

const REFRESH_MS = 60_000;

export const analyticsKeys = {
  all: ["analytics"] as const,
  overview: (filters: SalesFilters) =>
    ["analytics", "overview", filters] as const,
  process: (filters: SalesFilters) =>
    ["analytics", "process", filters] as const,
  commercial: (filters: SalesFilters, groupBy: GroupBy) =>
    ["analytics", "commercial", filters, groupBy] as const,
  team: (filters: SalesFilters) => ["analytics", "team", filters] as const,
  system: (period: PeriodOnly) => ["analytics", "system", period] as const,
};

/*
 * Each tab asks for its own endpoint and only while it is open. The previous answer stays on screen while a
 * new filter loads, so the figures do not flash away, and the page refreshes itself once a minute.
 */
const live = {
  staleTime: 0,
  refetchInterval: REFRESH_MS,
  placeholderData: keepPreviousData,
} as const;

export function useOverview(filters: SalesFilters, enabled: boolean) {
  return useQuery({
    queryKey: analyticsKeys.overview(filters),
    queryFn: () => analyticsApi.overview(filters),
    enabled,
    ...live,
  });
}

export function useProcess(filters: SalesFilters, enabled: boolean) {
  return useQuery({
    queryKey: analyticsKeys.process(filters),
    queryFn: () => analyticsApi.process(filters),
    enabled,
    ...live,
  });
}

export function useCommercial(
  filters: SalesFilters,
  groupBy: GroupBy,
  enabled: boolean,
) {
  return useQuery({
    queryKey: analyticsKeys.commercial(filters, groupBy),
    queryFn: () => analyticsApi.commercial(filters, groupBy),
    enabled,
    ...live,
  });
}

export function useTeam(filters: SalesFilters, enabled: boolean) {
  return useQuery({
    queryKey: analyticsKeys.team(filters),
    queryFn: () => analyticsApi.team(filters),
    enabled,
    ...live,
  });
}

export function useSystem(period: PeriodOnly, enabled: boolean) {
  return useQuery({
    queryKey: analyticsKeys.system(period),
    queryFn: () => analyticsApi.system(period),
    enabled,
    ...live,
  });
}

export function useAdvisors(enabled: boolean) {
  return useQuery({
    queryKey: ["staff", "advisors"] as const,
    queryFn: () => analyticsApi.advisors(),
    enabled,
    select: (result) => result.items,
  });
}

export function useModels() {
  return useQuery({
    queryKey: ["catalog", "selectable"] as const,
    queryFn: () => analyticsApi.models(),
    select: (result) => result.items,
  });
}
