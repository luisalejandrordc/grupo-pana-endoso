import { Icon } from "@endosoflow/ui";
import { customerTypeLabels, customerTypes } from "../customers/labels";
import { temperatureLabels } from "../sales/labels";
import {
  isFiltered,
  presetLabels,
  presets,
  rangeOf,
  type DashboardFilters,
  type Preset,
} from "./filters";
import type { Advisor, SelectableModel, Temperature } from "../sales/api";
import type { CustomerType } from "../customers/labels";

/**
 * The filters of the dashboard. They change what the numbers are about, never what a role may see: an Advisor
 * has no advisor filter because the server already limits them to their own sales.
 */
export function FilterBar({
  filters,
  onChange,
  today,
  advisors,
  models,
  periodOnly = false,
}: {
  filters: DashboardFilters;
  onChange: (next: DashboardFilters) => void;
  today: string;
  /** Present only for Managers and Admins. */
  advisors?: Advisor[];
  models: SelectableModel[];
  /** The system tab has no sales, so only the period applies to it. */
  periodOnly?: boolean;
}) {
  const range = rangeOf(filters, today);

  function setPreset(preset: Preset) {
    if (preset === "custom")
      onChange({ ...filters, preset, from: range.from, to: range.to });
    else onChange({ ...filters, preset, from: undefined, to: undefined });
  }

  function setDate(which: "from" | "to", value: string) {
    if (!value) return;
    const from = which === "from" ? value : range.from;
    const to = which === "to" ? value : range.to;
    // Keep the range valid: moving one end past the other drags the other along.
    onChange({
      ...filters,
      preset: "custom",
      from: from <= to ? from : value,
      to: from <= to ? to : value,
    });
  }

  return (
    <div className="dash-filters" role="group" aria-label="Filtros del panel">
      <label className="filter-control">
        <span>Periodo</span>
        <select
          value={filters.preset}
          onChange={(e) => setPreset(e.target.value as Preset)}
        >
          {presets.map((p) => (
            <option key={p} value={p}>
              {presetLabels[p]}
            </option>
          ))}
        </select>
      </label>

      {filters.preset === "custom" && (
        <>
          <label className="filter-control">
            <span>Desde</span>
            <input
              type="date"
              value={range.from}
              max={today}
              onChange={(e) => setDate("from", e.target.value)}
            />
          </label>
          <label className="filter-control">
            <span>Hasta</span>
            <input
              type="date"
              value={range.to}
              max={today}
              onChange={(e) => setDate("to", e.target.value)}
            />
          </label>
        </>
      )}

      {!periodOnly && advisors && (
        <label className="filter-control">
          <span>Asesor</span>
          <select
            value={filters.advisorId ?? ""}
            onChange={(e) =>
              onChange({ ...filters, advisorId: e.target.value || undefined })
            }
          >
            <option value="">Todos</option>
            {advisors.map((a) => (
              <option key={a.id} value={a.id}>
                {a.firstName} {a.lastName}
              </option>
            ))}
          </select>
        </label>
      )}

      {!periodOnly && (
        <>
          <label className="filter-control">
            <span>Cliente</span>
            <select
              value={filters.customerType ?? ""}
              onChange={(e) =>
                onChange({
                  ...filters,
                  customerType: (e.target.value || undefined) as
                    CustomerType | undefined,
                })
              }
            >
              <option value="">Todos</option>
              {customerTypes.map((t) => (
                <option key={t} value={t}>
                  {customerTypeLabels[t]}
                </option>
              ))}
            </select>
          </label>

          <label className="filter-control">
            <span>Temperatura</span>
            <select
              value={filters.temperature ?? ""}
              onChange={(e) =>
                onChange({
                  ...filters,
                  temperature: (e.target.value || undefined) as
                    Temperature | undefined,
                })
              }
            >
              <option value="">Todas</option>
              {(Object.keys(temperatureLabels) as Temperature[]).map((t) => (
                <option key={t} value={t}>
                  {temperatureLabels[t]}
                </option>
              ))}
            </select>
          </label>

          <label className="filter-control">
            <span>Modelo</span>
            <select
              value={filters.modelId ?? ""}
              onChange={(e) =>
                onChange({ ...filters, modelId: e.target.value || undefined })
              }
            >
              <option value="">Todos</option>
              {models.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name}
                </option>
              ))}
            </select>
          </label>
        </>
      )}

      {isFiltered(filters) && (
        <button
          type="button"
          className="button button-ghost"
          onClick={() => onChange({ preset: "90d" })}
        >
          <Icon name="filter-x" size={15} /> Limpiar filtros
        </button>
      )}
    </div>
  );
}
