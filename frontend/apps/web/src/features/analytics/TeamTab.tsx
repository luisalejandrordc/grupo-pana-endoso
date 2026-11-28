import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Badge, formatMoneyText } from "@endosoflow/ui";
import type { AdvisorPerformance, TeamData } from "./api";
import { Card, EmptyNote, PeriodNote, RankList, colors } from "./components";
import { formatCount, formatDuration, formatPercent } from "./format";

type SortKey =
  | "name"
  | "active"
  | "overdue"
  | "idle"
  | "created"
  | "completed"
  | "cancelled"
  | "closeRate"
  | "cycle"
  | "onTime"
  | "pipeline"
  | "completedValue"
  | "commission";

interface Column {
  key: SortKey;
  label: string;
  /** The value to order by; null sorts last whichever the direction. */
  value: (a: AdvisorPerformance) => number | string | null;
  render: (a: AdvisorPerformance) => string;
}

const columns: Column[] = [
  {
    key: "active",
    label: "Activas",
    value: (a) => a.active,
    render: (a) => formatCount(a.active),
  },
  {
    key: "overdue",
    label: "Con retraso",
    value: (a) => a.overdue,
    render: (a) => formatCount(a.overdue),
  },
  {
    key: "idle",
    label: "Sin iniciar",
    value: (a) => a.idle,
    render: (a) => formatCount(a.idle),
  },
  {
    key: "created",
    label: "Creadas",
    value: (a) => a.created,
    render: (a) => formatCount(a.created),
  },
  {
    key: "completed",
    label: "Completadas",
    value: (a) => a.completed,
    render: (a) => formatCount(a.completed),
  },
  {
    key: "cancelled",
    label: "Canceladas",
    value: (a) => a.cancelled,
    render: (a) => formatCount(a.cancelled),
  },
  {
    key: "closeRate",
    label: "Cierre",
    value: (a) => a.closeRate.value,
    render: (a) => formatPercent(a.closeRate.value),
  },
  {
    key: "cycle",
    label: "Ciclo mediano",
    value: (a) => a.cycleMedianHours,
    render: (a) => formatDuration(a.cycleMedianHours),
  },
  {
    key: "onTime",
    label: "Plazos cumplidos",
    value: (a) => a.onTimeRate.value,
    render: (a) => formatPercent(a.onTimeRate.value),
  },
  {
    key: "pipeline",
    label: "Valor en curso",
    value: (a) => Number(a.pipelineValue),
    render: (a) => formatMoneyText(a.pipelineValue),
  },
  {
    key: "completedValue",
    label: "Valor completado",
    value: (a) => Number(a.completedValue),
    render: (a) => formatMoneyText(a.completedValue),
  },
  {
    key: "commission",
    label: "Comisión",
    value: (a) => Number(a.commission),
    render: (a) => formatMoneyText(a.commission),
  },
];

function compare(
  a: number | string | null,
  b: number | string | null,
  direction: 1 | -1,
): number {
  if (a === null && b === null) return 0;
  if (a === null) return 1;
  if (b === null) return -1;
  if (typeof a === "string" || typeof b === "string") {
    return String(a).localeCompare(String(b), "es") * direction;
  }
  return (a - b) * direction;
}

/** One row per advisor: who carries what, who closes and who is slipping. Only managers and admins see it. */
export function TeamTab({ data }: { data: TeamData }) {
  const [sort, setSort] = useState<{ key: SortKey; direction: 1 | -1 }>({
    key: "active",
    direction: -1,
  });

  const rows = useMemo(() => {
    const column = columns.find((c) => c.key === sort.key);
    const list = [...data.advisors];
    list.sort((a, b) =>
      sort.key === "name"
        ? compare(a.name, b.name, sort.direction)
        : column
          ? compare(column.value(a), column.value(b), sort.direction)
          : 0,
    );
    return list;
  }, [data.advisors, sort]);

  function pick(key: SortKey) {
    setSort((current) =>
      current.key === key
        ? { key, direction: current.direction === 1 ? -1 : 1 }
        : { key, direction: key === "name" ? 1 : -1 },
    );
  }

  function ariaSort(key: SortKey): "ascending" | "descending" | "none" {
    if (sort.key !== key) return "none";
    return sort.direction === 1 ? "ascending" : "descending";
  }

  const loaded = [...data.advisors]
    .filter((a) => a.active > 0)
    .sort((a, b) => b.active - a.active);

  return (
    <div className="stack an-sections">
      <PeriodNote meta={data.meta} />
      <Card
        title="Carga de trabajo"
        subtitle="Ventas activas por asesor ahora, para repartir mejor el trabajo"
      >
        <RankList
          empty="No hay ventas activas con estos filtros."
          rows={loaded.map((a) => ({
            key: a.advisorId,
            label: a.name,
            value: a.active,
            display: formatCount(a.active),
            color: a.overdue > 0 ? colors.overdue : colors.onTime,
            detail:
              a.overdue > 0 || a.idle > 0
                ? `${formatCount(a.overdue)} con retraso · ${formatCount(a.idle)} sin iniciar`
                : undefined,
          }))}
        />
      </Card>

      <Card
        title="Asesores"
        subtitle="Las ventas se atribuyen al asesor asignado hoy, aunque hayan sido reasignadas"
      >
        {rows.length === 0 ? (
          <EmptyNote>Ningún asesor tiene ventas con estos filtros.</EmptyNote>
        ) : (
          <div className="table-scroll">
            <table className="table an-table an-team">
              <thead>
                <tr>
                  <th aria-sort={ariaSort("name")}>
                    <button
                      type="button"
                      className="an-sort"
                      onClick={() => pick("name")}
                    >
                      Asesor
                    </button>
                  </th>
                  {columns.map((c) => (
                    <th
                      key={c.key}
                      className="an-num"
                      aria-sort={ariaSort(c.key)}
                    >
                      <button
                        type="button"
                        className="an-sort"
                        onClick={() => pick(c.key)}
                      >
                        {c.label}
                      </button>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((a) => (
                  <tr key={a.advisorId}>
                    <td>
                      <Link
                        to={`/sales?advisor=${a.advisorId}`}
                        className="an-person"
                      >
                        {a.name}
                      </Link>
                      {a.isActive ? null : (
                        <>
                          {" "}
                          <Badge tone="neutral" plain>
                            Inactivo
                          </Badge>
                        </>
                      )}
                    </td>
                    {columns.map((c) => (
                      <td key={c.key} className="an-num">
                        {c.render(a)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="an-footnote">
          El cierre es completadas entre completadas más canceladas del periodo.
          Los plazos cumplidos cuentan solo las tareas que tienen plazo. Los
          montos son registrados en las tareas, en dólares; no son cifras
          contables.
        </p>
      </Card>
    </div>
  );
}
