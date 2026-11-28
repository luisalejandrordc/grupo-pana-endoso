import { useMemo } from "react";
import { PageHeader, Tabs, type TabItem } from "@endosoflow/ui";
import type { Role } from "../../shared/types";
import { CommercialTab } from "./CommercialTab";
import { OverviewTab } from "./OverviewTab";
import { ProcessTab } from "./ProcessTab";
import { QueryBoundary } from "./components";
import {
  customRangeIsValid,
  limaDate,
  periodDates,
  periodIds,
  periodLabels,
  salesQuery,
  tabsFor,
  useAnalyticsFilters,
  type PeriodId,
} from "./filters";
import {
  customerTypeLabels,
  tabLabels,
  temperatureLabels,
  type TabId,
} from "./labels";
import {
  useAdvisors,
  useModels,
  useOverview,
  useProcess,
  useSystem,
  useTeam,
} from "./queries";
import { SystemTab } from "./SystemTab";
import { TeamTab } from "./TeamTab";

/** Business intelligence: how the sales are going, where the work stalls and what it is worth. */
export function AnalyticsPage({ role }: { role: Role }) {
  const { state, update } = useAnalyticsFilters(role);
  const today = limaDate(new Date());
  const isAdvisor = role === "sales_advisor";
  const valid = customRangeIsValid(state);
  const tab = state.tab;

  const filters = useMemo(() => salesQuery(state, today), [state, today]);
  const periodOnly = useMemo(
    () => periodDates(state.period, today, state.from, state.to),
    [state.period, state.from, state.to, today],
  );

  const overview = useOverview(filters, tab === "summary" && valid);
  const process = useProcess(filters, tab === "process" && valid);
  const team = useTeam(filters, tab === "team" && valid);
  const system = useSystem(periodOnly, tab === "system" && valid);
  const advisors = useAdvisors(!isAdvisor && tab !== "system");
  const models = useModels();

  const tabItems: TabItem<TabId>[] = tabsFor(role).map((id) => ({
    id,
    label: id === "summary" && isAdvisor ? "Mi panel" : tabLabels[id],
  }));

  const showAdvisorFilter = !isAdvisor && tab !== "team" && tab !== "system";
  const showSalesFilters = tab !== "system";
  const filtered =
    state.advisor !== "" ||
    state.customerType !== "" ||
    state.model !== "" ||
    state.temperature !== "" ||
    state.period !== "90d";

  return (
    <div className="stack an-page">
      <PageHeader
        title="Analítica"
        description={
          isAdvisor
            ? "Tus ventas: qué necesita atención, cómo vas y cuánto valen."
            : "Cómo van las ventas, dónde se atasca el trabajo y cuánto valen."
        }
      />

      <Tabs
        items={tabItems}
        value={tab}
        onChange={(id) => update({ tab: id === "summary" ? "" : id })}
        label="Secciones de analítica"
      />

      <div className="an-filters">
        <div className="filter-bar">
          <select
            aria-label="Periodo"
            className="filter-select"
            value={state.period}
            onChange={(event) =>
              update({
                period: event.target.value === "90d" ? "" : event.target.value,
                from: "",
                to: "",
              })
            }
          >
            {periodIds.map((id: PeriodId) => (
              <option key={id} value={id}>
                {periodLabels[id]}
              </option>
            ))}
          </select>
          {state.period === "custom" ? (
            <>
              <label className="filter-control">
                Desde
                <input
                  type="date"
                  value={state.from}
                  max={state.to || today}
                  onChange={(event) => update({ from: event.target.value })}
                />
              </label>
              <label className="filter-control">
                Hasta
                <input
                  type="date"
                  value={state.to}
                  min={state.from || undefined}
                  max={today}
                  onChange={(event) => update({ to: event.target.value })}
                />
              </label>
            </>
          ) : null}
          {showAdvisorFilter ? (
            <select
              aria-label="Asesor"
              className="filter-select"
              value={state.advisor}
              onChange={(event) => update({ advisor: event.target.value })}
            >
              <option value="">Todos los asesores</option>
              {advisors.data?.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.firstName} {a.lastName}
                </option>
              ))}
            </select>
          ) : null}
          {showSalesFilters ? (
            <>
              <select
                aria-label="Tipo de cliente"
                className="filter-select"
                value={state.customerType}
                onChange={(event) =>
                  update({ customerType: event.target.value })
                }
              >
                <option value="">Todo tipo de cliente</option>
                {Object.entries(customerTypeLabels).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
              <select
                aria-label="Modelo"
                className="filter-select"
                value={state.model}
                onChange={(event) => update({ model: event.target.value })}
              >
                <option value="">Todos los modelos</option>
                {models.data?.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </select>
              <select
                aria-label="Prioridad"
                className="filter-select"
                value={state.temperature}
                onChange={(event) =>
                  update({ temperature: event.target.value })
                }
              >
                <option value="">Toda prioridad</option>
                {Object.entries(temperatureLabels).map(([value, label]) => (
                  <option key={value} value={value}>
                    Prioridad {label.toLowerCase()}
                  </option>
                ))}
              </select>
            </>
          ) : null}
          {filtered ? (
            <button
              type="button"
              className="button button-ghost"
              onClick={() =>
                update({
                  period: "",
                  from: "",
                  to: "",
                  advisor: "",
                  customerType: "",
                  model: "",
                  temperature: "",
                })
              }
            >
              Limpiar filtros
            </button>
          ) : null}
        </div>
      </div>

      {!valid ? (
        <p className="an-empty" role="status">
          Elige las dos fechas del periodo, la inicial no puede ser posterior a
          la final.
        </p>
      ) : tab === "summary" ? (
        <QueryBoundary query={overview} what="el resumen">
          {(data) => <OverviewTab data={data} showAdvisor={!isAdvisor} />}
        </QueryBoundary>
      ) : tab === "process" ? (
        <QueryBoundary query={process} what="los datos del proceso">
          {(data) => <ProcessTab data={data} />}
        </QueryBoundary>
      ) : tab === "commercial" ? (
        <CommercialTab filters={filters} />
      ) : tab === "team" ? (
        <QueryBoundary query={team} what="los datos del equipo">
          {(data) => <TeamTab data={data} />}
        </QueryBoundary>
      ) : (
        <QueryBoundary query={system} what="los datos del sistema">
          {(data) => <SystemTab data={data} />}
        </QueryBoundary>
      )}
    </div>
  );
}
