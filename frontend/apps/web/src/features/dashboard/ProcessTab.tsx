import { Badge } from "@endosoflow/ui";
import { ChartCard, Freshness, KpiCard, KpiGrid, TabBody } from "./components";
import { ColumnChart, RowChart, color } from "./charts";
import { daysText, durationText, percent } from "./labels";
import { useProcess } from "./queries";
import { cancelLabels } from "../sales/labels";
import type { DashboardParams } from "./api";

export function ProcessTab({ params }: { params: DashboardParams }) {
  const query = useProcess(params, true);
  return (
    <TabBody query={query}>
      {(data) => {
        const load = data.tasks.map((t) => ({
          label: `${t.number}. ${t.name}`,
          activeSales: t.activeSales,
        }));
        const withSla = data.tasks.filter((t) => t.recommendedHours !== null);
        const sla = withSla.map((t) => ({
          label: `T${t.number}`,
          onTime: t.onTime,
          late: t.late,
        }));
        const hasSla = sla.some((t) => t.onTime + t.late > 0);
        const byCategory = data.cancellations.byCategory.map((c) => ({
          label: cancelLabels[c.category],
          count: c.count,
        }));
        const byTask = data.cancellations.byTask.map((c) => ({
          label: `${c.number}. ${c.name}`,
          count: c.count,
        }));
        return (
          <>
            <KpiGrid>
              <KpiCard
                label="Ciclo completo (mediana)"
                value={daysText(data.cycleTime.medianDays)}
                hint={`De crear a completar la venta · ${data.cycleTime.sampleSize} venta(s)`}
              />
              <KpiCard
                label="Ciclo completo (p90)"
                value={daysText(data.cycleTime.p90Days)}
                hint="9 de cada 10 ventas terminan antes"
              />
              <KpiCard
                label="Tareas reabiertas"
                value={data.reopenedTasks}
                tone={data.reopenedTasks > 0 ? "warn" : "neutral"}
                hint="Señal de retrabajo"
              />
              <KpiCard
                label="Cancelaciones"
                value={data.cancellations.total}
                hint="En el periodo"
              />
            </KpiGrid>

            <ChartCard
              title="Cuello de botella"
              subtitle="Ventas en curso por tarea actual, hoy"
              empty={load.every((t) => t.activeSales === 0)}
              emptyText="No hay ventas en curso."
            >
              <RowChart
                data={load}
                labelWidth={210}
                summary={`Ventas en curso por tarea: ${load
                  .filter((t) => t.activeSales > 0)
                  .map((t) => `${t.label}, ${t.activeSales}`)
                  .join("; ")}`}
                series={[
                  { key: "activeSales", label: "Ventas", color: color.active },
                ]}
              />
            </ChartCard>

            <ChartCard
              title="Cumplimiento de plazos"
              subtitle="Tareas completadas a tiempo o con retraso (solo las 6 tareas con plazo)"
              empty={!hasSla}
              emptyText="Aún no se completaron tareas con plazo en este periodo."
            >
              <ColumnChart
                data={sla}
                xKey="label"
                stacked
                summary={`Cumplimiento por tarea: ${withSla
                  .map(
                    (t) =>
                      `tarea ${t.number}, ${t.onTime} a tiempo, ${t.late} con retraso`,
                  )
                  .join("; ")}`}
                series={[
                  { key: "onTime", label: "A tiempo", color: color.completed },
                  { key: "late", label: "Con retraso", color: color.cancelled },
                ]}
              />
            </ChartCard>

            <ChartCard
              title="Tiempos por tarea"
              subtitle="Trabajo: desde que se inicia hasta que se completa. Espera: desde que puede iniciar hasta que alguien la inicia. Medianas de las tareas completadas en el periodo."
            >
              <div className="table-scroll">
                <table className="table">
                  <thead>
                    <tr>
                      <th>Tarea</th>
                      <th className="num">Plazo</th>
                      <th className="num">Completadas</th>
                      <th className="num">Trabajo</th>
                      <th className="num">Espera</th>
                      <th className="num">A tiempo</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.tasks.map((t) => (
                      <tr key={t.number}>
                        <td>
                          {t.number}. {t.name}
                        </td>
                        <td className="num">
                          {t.recommendedHours === null
                            ? "—"
                            : durationText(t.recommendedHours)}
                        </td>
                        <td className="num">{t.completedInPeriod}</td>
                        <td className="num">
                          {durationText(t.medianWorkHours)}
                        </td>
                        <td className="num">
                          {durationText(t.medianWaitHours)}
                        </td>
                        <td className="num">
                          {t.onTimeRate === null ? (
                            "—"
                          ) : (
                            <Badge tone={t.onTimeRate >= 80 ? "done" : "warn"}>
                              {percent(t.onTimeRate)}
                            </Badge>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </ChartCard>

            <div className="dash-grid">
              <ChartCard
                title="Cancelaciones por motivo"
                empty={byCategory.length === 0}
                emptyText="No hubo cancelaciones en este periodo."
              >
                <RowChart
                  data={byCategory}
                  labelWidth={210}
                  summary={`Cancelaciones por motivo: ${byCategory.map((c) => `${c.label}, ${c.count}`).join("; ")}`}
                  series={[
                    { key: "count", label: "Ventas", color: color.cancelled },
                  ]}
                />
              </ChartCard>
              <ChartCard
                title="Cancelaciones por tarea"
                subtitle="La tarea en la que estaba la venta al cancelarse"
                empty={byTask.length === 0}
                emptyText="No hubo cancelaciones en este periodo."
              >
                <RowChart
                  data={byTask}
                  labelWidth={210}
                  summary={`Cancelaciones por tarea: ${byTask.map((c) => `${c.label}, ${c.count}`).join("; ")}`}
                  series={[
                    { key: "count", label: "Ventas", color: color.cancelled },
                  ]}
                />
              </ChartCard>
            </div>
            <Freshness at={data.generatedAt} />
          </>
        );
      }}
    </TabBody>
  );
}
