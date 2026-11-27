import { Link } from "react-router-dom";
import { Badge, InitialsTile, formatMoneyText } from "@endosoflow/ui";
import { ChartCard, Freshness, TabBody } from "./components";
import { RowChart, color } from "./charts";
import { daysText, percent } from "./labels";
import { useAdvisorTable } from "./queries";
import type { DashboardParams } from "./api";

export function AdvisorsTab({ params }: { params: DashboardParams }) {
  const query = useAdvisorTable(params, true);
  return (
    <TabBody query={query}>
      {(data) => {
        const chart = data.items.map((a) => ({
          label: a.name,
          completed: a.completedInPeriod,
          cancelled: a.cancelledInPeriod,
        }));
        return (
          <>
            <ChartCard
              title="Completadas y canceladas por asesor"
              subtitle="En el periodo"
              empty={chart.every((a) => a.completed + a.cancelled === 0)}
              emptyText="Ningún asesor completó ni canceló ventas en este periodo."
            >
              <RowChart
                data={chart}
                stacked
                labelWidth={170}
                summary={`Ventas por asesor: ${chart
                  .map(
                    (a) =>
                      `${a.label}, ${a.completed} completadas, ${a.cancelled} canceladas`,
                  )
                  .join("; ")}`}
                series={[
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
              title="El equipo, lado a lado"
              subtitle="Cada fila abre las ventas de ese asesor"
              empty={data.items.length === 0}
              emptyText="Ningún asesor tiene ventas con estos filtros."
            >
              <div className="table-scroll">
                <table className="table">
                  <thead>
                    <tr>
                      <th>Asesor</th>
                      <th className="num">En curso</th>
                      <th className="num">Creadas</th>
                      <th className="num">Completadas</th>
                      <th className="num">Canceladas</th>
                      <th className="num">Tasa de cierre</th>
                      <th className="num">Ciclo (mediana)</th>
                      <th className="num">Con retraso</th>
                      <th className="num">Cartera cotizada</th>
                      <th className="num">Comisión</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((a) => (
                      <tr key={a.advisorId}>
                        <td>
                          <Link
                            className="entity-cell"
                            to={`/sales?advisor=${a.advisorId}&state=all`}
                          >
                            <InitialsTile name={a.name} />
                            <span>
                              {a.name}
                              {!a.isActive && <Badge plain>Inactivo</Badge>}
                            </span>
                          </Link>
                        </td>
                        <td className="num">{a.activeSales}</td>
                        <td className="num">{a.createdInPeriod}</td>
                        <td className="num">{a.completedInPeriod}</td>
                        <td className="num">{a.cancelledInPeriod}</td>
                        <td className="num">{percent(a.closingRate)}</td>
                        <td className="num">{daysText(a.medianCycleDays)}</td>
                        <td className="num">
                          {a.overdueTasks > 0 ? (
                            <Badge tone="late">{a.overdueTasks}</Badge>
                          ) : (
                            0
                          )}
                        </td>
                        <td className="num">
                          {formatMoneyText(a.pipelineValue)}
                        </td>
                        <td className="num">{formatMoneyText(a.commission)}</td>
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
