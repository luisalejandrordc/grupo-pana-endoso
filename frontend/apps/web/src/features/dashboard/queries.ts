import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { dashboardApi, type DashboardParams } from "./api";

export type TabId =
  "summary" | "process" | "market" | "finance" | "advisors" | "system";

/**
 * Every tab is one request, made only while the tab is open (`enabled`). The numbers are aggregates, so they
 * may be a minute old; the previous answer stays on screen while a new filter loads.
 */
function useTab<T>(
  tab: TabId,
  params: DashboardParams,
  enabled: boolean,
  fetcher: (params: DashboardParams) => Promise<T>,
) {
  return useQuery({
    queryKey: ["dashboard", tab, params] as const,
    queryFn: () => fetcher(params),
    enabled,
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  });
}

export const useSummary = (p: DashboardParams, on: boolean) =>
  useTab("summary", p, on, dashboardApi.summary);
export const useProcess = (p: DashboardParams, on: boolean) =>
  useTab("process", p, on, dashboardApi.process);
export const useMarket = (p: DashboardParams, on: boolean) =>
  useTab("market", p, on, dashboardApi.market);
export const useFinance = (p: DashboardParams, on: boolean) =>
  useTab("finance", p, on, dashboardApi.finance);
export const useAdvisorTable = (p: DashboardParams, on: boolean) =>
  useTab("advisors", p, on, dashboardApi.advisors);
export const useSystem = (p: DashboardParams, on: boolean) =>
  useTab("system", p, on, dashboardApi.system);
