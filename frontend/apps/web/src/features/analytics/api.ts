import { api } from "../../shared/api/client";
import { queryString, type Operations, type Schemas } from "@endosoflow/api";

export type Overview = Schemas["AnalyticsOverview"];
export type ProcessData = Schemas["AnalyticsProcess"];
export type CommercialData = Schemas["AnalyticsCommercial"];
export type TeamData = Schemas["AnalyticsTeam"];
export type SystemData = Schemas["AnalyticsSystem"];
export type TaskTiming = Schemas["TaskTiming"];
export type MilestoneLoad = Schemas["MilestoneLoad"];
export type AttentionItem = Schemas["AttentionItem"];
export type AdvisorPerformance = Schemas["AdvisorPerformance"];
export type CountMetric = Schemas["CountMetric"];
export type MoneyMetric = Schemas["MoneyMetric"];
export type Ratio = Schemas["Ratio"];
export type Temperature = Schemas["SaleTemperature"];
export type CustomerType = Schemas["CustomerType"];
export type CancelCategory = Schemas["CancellationCategory"];
export type Advisor = Schemas["Advisor"];
export type SelectableModel = Schemas["SelectableModel"];

/** The filters every sales tab accepts, straight from the generated contract. */
export type SalesFilters = NonNullable<
  Operations["getAnalyticsOverview"]["parameters"]["query"]
>;
export type GroupBy = NonNullable<
  NonNullable<
    Operations["getAnalyticsCommercial"]["parameters"]["query"]
  >["groupBy"]
>;
export type PeriodOnly = NonNullable<
  Operations["getAnalyticsSystem"]["parameters"]["query"]
>;

export const analyticsApi = {
  overview: (filters: SalesFilters) =>
    api.request<Overview>(`/analytics/overview${queryString({ ...filters })}`),
  process: (filters: SalesFilters) =>
    api.request<ProcessData>(
      `/analytics/process${queryString({ ...filters })}`,
    ),
  commercial: (filters: SalesFilters, groupBy: GroupBy) =>
    api.request<CommercialData>(
      `/analytics/commercial${queryString({ ...filters, groupBy })}`,
    ),
  team: (filters: SalesFilters) =>
    api.request<TeamData>(`/analytics/team${queryString({ ...filters })}`),
  system: (period: PeriodOnly) =>
    api.request<SystemData>(`/analytics/system${queryString({ ...period })}`),
  advisors: () => api.request<{ items: Advisor[] }>("/advisors"),
  models: () =>
    api.request<{ items: SelectableModel[] }>("/catalog/selectable"),
};
