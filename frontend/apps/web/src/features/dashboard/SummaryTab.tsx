import { Link } from "react-router-dom";
import { Badge, formatDateTime } from "@endosoflow/ui";
import type { BadgeTone } from "@endosoflow/ui";
import { ChartCard, Freshness, KpiCard, KpiGrid, TabBody } from "./components";
import { ColumnChart, RowChart, color } from "./charts";
import { attentionLabels, monthLabel, percent } from "./labels";
import { useSummary } from "./queries";
import type { AttentionItem, DashboardParams } from "./api";

const kindTone: Record<AttentionItem["kind"], BadgeTone> = {
  overdue: "late",
  approaching: "warn",
  stalled: "neutral",
};

function whenText(item: AttentionItem) {
  if (item.kind === "stalled" && item.waitingSince)
    return `Esperando desde ${formatDateTime(item.waitingSince)}`;
  if (item.dueDate)
    return `${item.kind === "overdue" ? "Venció" : "Vence"} ${formatDateTime(item.dueDate)}`;
  return "";
}

export function SummaryTab({
  params,
  showAdvisor,
}: {
  params: DashboardParams;
  showAdvisor: boolean;
}) {
  const query = useSummary(params, true);
  return (
    <TabBody query={query}>
      {(data) => {
        const k = data.kpis;
        const trend = data.trend.map((p) => ({
          ...p,
          label: monthLabel(p.month),
        }));
        const hasTrend = trend.some(
          (p) => p.created + p.completed + p.cancelled > 0,
        );
        const milestones = data.milestones.map((m) => ({
          label: `${m.number}. ${m.name}`,
          activeSales: m.activeSales,
        }));
        return (
          <>
            <KpiGrid>
              <KpiCard
                label="Ventas en curso"
                value={k.activeSales}
                snapshot
                to="/sales"
              />
              <KpiCard label="Ventas creadas" value={k.createdInPeriod} />
              <KpiCard
                label="Ventas completadas"
                value={k.completedInPeriod}
                tone="done"
              />
              <KpiCard label="Ventas canceladas" value={k.cancelledInPeriod} />
              <KpiCard
                label="Tasa de cierre"
                value={percent(k.closingRate)}
                hint="Completadas ÷ (completadas + canceladas)"
              />
              <KpiCard
                label="Tareas con retraso"
                value={k.overdueTasks}
                tone={k.overdueTasks > 0 ? "late" : "neutral"}
                hint={`${k.approachingTasks} por vencer · ${k.stalledTasks} sin iniciar`}
                snapshot
              />
            </KpiGrid>

            <div className="dash-grid dash-grid-wide">
              <ChartCard
                title="Evolución mensual"
                subtitle="Ventas creadas, completadas y canceladas por mes"
                empty={!hasTrend}
              >
                <ColumnChart
                  data={trend}
                  xKey="label"
                  summary={`Ventas por mes: ${trend
                    .map(
                      (p) =>
                        `${p.label}, ${p.created} creadas, ${p.completed} completadas, ${p.cancelled} canceladas`,
                    )
                    .join("; ")}`}
                  series={[
                    { key: "created", label: "Creadas", color: color.created },
                    {
                      key: "completed",
                      label: "Completadas",
                      color: color.completed,
                    },
                    {
                      key: "cancelled",
                      label: "Canceladas",
                      color: color.cancelled,
                    },
                  ]}
                />
              </ChartCard>
              <ChartCard
                title="Dónde están las ventas en curso"
                subtitle="Por hito, hoy"
                empty={k.activeSales === 0}
                emptyText="No hay ventas en curso."
              >
                <RowChart
                  data={milestones}
                  labelWidth={170}
                  summary={`Ventas en curso por hito: ${milestones
                    .map((m) => `${m.label}, ${m.activeSales}`)
                    .join("; ")}`}
                  series={[
                    {
                      key: "activeSales",
                      label: "Ventas",
                      color: color.active,
                    },
                  ]}
                />
              </ChartCard>
            </div>

            <ChartCard
              title="Requieren atención"
              subtitle={
                data.attention.total > data.attention.items.length
                  ? `Las ${data.attention.items.length} más urgentes de ${data.attention.total}`
                  : "Tareas con retraso, por vencer o sin iniciar"
              }
              empty={data.attention.total === 0}
              emptyText="Nada pendiente: no hay tareas con retraso ni sin iniciar."
            >
              <div className="table-scroll">
                <table className="table attention-table">
                  <thead>
                    <tr>
                      <th>Venta</th>
                      <th>Cliente</th>
                      <th>Tarea</th>
                      {showAdvisor && <th>Asesor</th>}
                      <th>Estado</th>
                      <th>Plazo</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.attention.items.map((item) => (
                      <tr key={item.saleId}>
                        <td>
                          <Link to={`/sales/${item.saleId}`}>
                            {item.saleCode}
                          </Link>
                          <span className="cell-sub">{item.vehicle}</span>
                        </td>
                        <td>{item.customerName}</td>
                        <td>
                          {item.taskNumber}. {item.taskName}
                        </td>
                        {showAdvisor && <td>{item.advisorName}</td>}
                        <td>
                          <Badge tone={kindTone[item.kind]}>
                            {attentionLabels[item.kind]}
                          </Badge>
                        </td>
                        <td>{whenText(item)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </ChartCard>
            <Freshness at={data.generatedAt} />
          </>
        );
      }}
    </TabBody>
  );
}
