import { api } from "../../shared/api/client";
import { queryString, type Operations, type Schemas } from "@endosoflow/api";

export type DashboardSummary = Schemas["DashboardSummary"];
export type DashboardProcess = Schemas["DashboardProcess"];
export type DashboardMarket = Schemas["DashboardMarket"];
export type DashboardFinance = Schemas["DashboardFinance"];
export type DashboardAdvisors = Schemas["DashboardAdvisors"];
export type DashboardSystem = Schemas["DashboardSystem"];
export type AttentionItem = Schemas["DashboardAttentionItem"];
export type TaskStats = Schemas["DashboardTaskStats"];
export type MixItem = Schemas["DashboardMixItem"];
export type AdvisorRow = Schemas["DashboardAdvisorRow"];

/** The query every tab shares, straight from the contract so a new filter shows up here by itself. */
export type DashboardParams = NonNullable<
  Operations["getDashboardSummary"]["parameters"]["query"]
>;

const read =
  <T>(tab: string) =>
  (params: DashboardParams) =>
    api.request<T>(`/dashboard/${tab}${queryString({ ...params })}`);

export const dashboardApi = {
  summary: read<DashboardSummary>("summary"),
  process: read<DashboardProcess>("process"),
  market: read<DashboardMarket>("market"),
  finance: read<DashboardFinance>("finance"),
  advisors: read<DashboardAdvisors>("advisors"),
  system: read<DashboardSystem>("system"),
};
