import { useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { Icon, PageHeader, Tabs, type TabItem } from "@endosoflow/ui";
import type { Role } from "../../shared/types";
import { useAdvisors, useSelectableCatalog } from "../sales/queries";
import { AdvisorsTab } from "./AdvisorsTab";
import { FilterBar } from "./FilterBar";
import { FinanceTab } from "./FinanceTab";
import { MarketTab } from "./MarketTab";
import { ProcessTab } from "./ProcessTab";
import { SummaryTab } from "./SummaryTab";
import { SystemTab } from "./SystemTab";
import {
  filtersToSearch,
  limaToday,
  parseFilters,
  rangeOf,
  toParams,
  type DashboardFilters,
} from "./filters";
import { dayText, tabLabels, tabsFor } from "./labels";
import type { TabId } from "./queries";

/**
 * The BI dashboard. What a role may see is decided by the server; this page only hides the tabs a role could
 * not open anyway, so nobody is offered a screen that answers "forbidden".
 */
export function DashboardPage({ role }: { role: Role }) {
  const [search, setSearch] = useSearchParams();
  const client = useQueryClient();
  const management = role !== "sales_advisor";
  const allowed = tabsFor(role);
  const rawTab = search.get("tab");
  const tab: TabId = allowed.includes(rawTab as TabId)
    ? (rawTab as TabId)
    : "summary";

  const filters = parseFilters(search);
  const today = limaToday();
  const params = toParams(filters, today, management);
  const range = rangeOf(filters, today);

  const advisors = useAdvisors(management);
  const catalog = useSelectableCatalog();

  function changeFilters(next: DashboardFilters) {
    setSearch(filtersToSearch(next, search), { replace: true });
  }

  function changeTab(next: TabId) {
    const copy = new URLSearchParams(search);
    if (next === "summary") copy.delete("tab");
    else copy.set("tab", next);
    setSearch(copy, { replace: true });
  }

  const items: TabItem<TabId>[] = allowed.map((id) => ({
    id,
    label: tabLabels[id],
  }));

  return (
    <div className="stack">
      <PageHeader
        title="Panel"
        description={
          role === "sales_advisor"
            ? "Tu cartera de ventas: cómo avanza y qué necesita atención."
            : "Indicadores del negocio y de la operación."
        }
        actions={
          <button
            type="button"
            className="button button-secondary"
            onClick={() =>
              void client.invalidateQueries({ queryKey: ["dashboard"] })
            }
          >
            <Icon name="refresh" size={15} /> Actualizar
          </button>
        }
      />

      <Tabs
        items={items}
        value={tab}
        onChange={changeTab}
        label="Secciones del panel"
      />

      <FilterBar
        filters={filters}
        onChange={changeFilters}
        today={today}
        advisors={management ? advisors.data : undefined}
        models={catalog.data ?? []}
        periodOnly={tab === "system"}
      />
      <p className="dash-period">
        Periodo: {dayText(range.from)} – {dayText(range.to)}
        {tab !== "system" && (
          <>
            {" "}
            · Los indicadores marcados con «Hoy» muestran la situación actual y
            no cambian con el periodo.
          </>
        )}
      </p>

      <div role="tabpanel" aria-label={tabLabels[tab]}>
        {tab === "summary" && (
          <SummaryTab params={params} showAdvisor={management} />
        )}
        {tab === "process" && <ProcessTab params={params} />}
        {tab === "market" && <MarketTab params={params} />}
        {tab === "finance" && <FinanceTab params={params} />}
        {tab === "advisors" && <AdvisorsTab params={params} />}
        {tab === "system" && <SystemTab params={params} />}
      </div>
    </div>
  );
}
