import { Badge, Callout, formatMoneyText } from "@endosoflow/ui";
import { ChartCard, Freshness, KpiCard, KpiGrid, TabBody } from "./components";
import { ColumnChart, color } from "./charts";
import { invoiceLabels, monthLabel } from "./labels";
import { useFinance } from "./queries";
import type { DashboardParams } from "./api";

const usd = (v: unknown) => `$${Number(v).toLocaleString("es-PE")}`;
const amount = (text: string) => Number(text);

export function FinanceTab({ params }: { params: DashboardParams }) {
  const query = useFinance(params, true);
  return (
    <TabBody query={query}>
      {(data) => {
        const { pipeline, completed, cash } = data;
        const monthly = data.monthly.map((m) => ({
          label: monthLabel(m.month),
          moneyIn: amount(m.moneyIn),
          moneyOut: amount(m.moneyOut),
          commission: amount(m.commission),
        }));
        const hasMonthly = monthly.some(
          (m) => m.moneyIn + m.moneyOut + m.commission > 0,
        );
        const heldNegative = amount(cash.heldInActiveSales) < 0;
        return (
          <>
            <Callout tone="quiet" icon="info">
              Son montos registrados en las tareas ya completadas, no ingresos
              contables. Un monto solo cuenta cuando la tarea que lo confirma
              está completada.
            </Callout>

            <h2 className="dash-section">En curso (hoy)</h2>
            <KpiGrid>
              <KpiCard
                label="Valor cotizado en curso"
                value={formatMoneyText(pipeline.quotedValue)}
                hint={`${pipeline.sales} venta(s) con cotización completada`}
                snapshot
              />
              <KpiCard
                label="Comisión final"
                value={formatMoneyText(pipeline.finalCommission)}
                hint="Ventas con liquidación completada"
                snapshot
              />
              <KpiCard
                label="Comisión provisional"
                value={formatMoneyText(pipeline.provisionalCommission)}
                hint="Comisión inicial, aún sin liquidar"
                snapshot
              />
              <KpiCard
                label="Retenido en ventas activas"
                value={formatMoneyText(cash.heldInActiveSales)}
                tone={heldNegative ? "warn" : "neutral"}
                hint="Cobrado a clientes menos pagado a Toyota"
                snapshot
              />
              <KpiCard
                label="Cobrado en ventas canceladas"
                value={formatMoneyText(cash.inCancelledSales)}
                tone={amount(cash.inCancelledSales) > 0 ? "warn" : "neutral"}
                hint="Posible devolución pendiente"
                snapshot
              />
            </KpiGrid>

            <h2 className="dash-section">Movimiento del periodo</h2>
            <KpiGrid>
              <KpiCard
                label="Dinero cobrado"
                value={formatMoneyText(cash.moneyIn)}
                hint="Pagos de clientes a Grupo Pana"
              />
              <KpiCard
                label="Pagado a Toyota"
                value={formatMoneyText(cash.moneyOut)}
              />
              <KpiCard
                label="Neto"
                value={formatMoneyText(cash.net)}
                tone={amount(cash.net) < 0 ? "warn" : "done"}
                hint="Cobrado menos pagado"
              />
              <KpiCard
                label="Pagos con diferencia"
                value={cash.paymentMismatches}
                tone={cash.paymentMismatches > 0 ? "warn" : "neutral"}
                hint="Monto depositado distinto del esperado"
              />
            </KpiGrid>

            <ChartCard
              title="Dinero y comisión por mes"
              subtitle="Cobrado y pagado según la fecha del pago; comisión según el mes en que se completó la venta"
              empty={!hasMonthly}
            >
              <ColumnChart
                data={monthly}
                xKey="label"
                format={usd}
                summary={`Dinero por mes: ${monthly
                  .map(
                    (m) =>
                      `${m.label}, cobrado ${m.moneyIn}, pagado ${m.moneyOut}, comisión ${m.commission}`,
                  )
                  .join("; ")}`}
                series={[
                  { key: "moneyIn", label: "Cobrado", color: color.created },
                  {
                    key: "moneyOut",
                    label: "Pagado a Toyota",
                    color: color.neutral,
                  },
                  {
                    key: "commission",
                    label: "Comisión",
                    color: color.completed,
                  },
                ]}
              />
            </ChartCard>

            <div className="dash-grid">
              <ChartCard
                title="Ventas completadas"
                subtitle={`${completed.sales} venta(s) en el periodo`}
                empty={completed.sales === 0}
                emptyText="No se completaron ventas en este periodo."
              >
                <dl className="dash-defs">
                  <dt>Valor cotizado</dt>
                  <dd>{formatMoneyText(completed.quotedValue)}</dd>
                  <dt>Comisión</dt>
                  <dd>{formatMoneyText(completed.commission)}</dd>
                  <dt>Comisión promedio por venta</dt>
                  <dd>{formatMoneyText(completed.averageCommission)}</dd>
                  <dt>Accesorios</dt>
                  <dd>{formatMoneyText(completed.accessories)}</dd>
                  <dt>Trámite de liberación</dt>
                  <dd>{formatMoneyText(completed.releaseProcessing)}</dd>
                  <dt>Monto final a Toyota</dt>
                  <dd>{formatMoneyText(completed.toyotaAmount)}</dd>
                </dl>
              </ChartCard>
              <ChartCard
                title="Facturas del periodo"
                subtitle="Según la fecha de la factura, con la facturación completada"
              >
                <div className="table-scroll">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Factura</th>
                        <th className="num">Cantidad</th>
                        <th className="num">Monto</th>
                        <th className="num">Con diferencia</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.invoices.map((i) => (
                        <tr key={i.kind}>
                          <td>{invoiceLabels[i.kind] ?? i.kind}</td>
                          <td className="num">{i.count}</td>
                          <td className="num">{formatMoneyText(i.amount)}</td>
                          <td className="num">
                            {i.mismatches > 0 ? (
                              <Badge tone="warn">{i.mismatches}</Badge>
                            ) : (
                              0
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </ChartCard>
            </div>
            <Freshness at={data.generatedAt} />
          </>
        );
      }}
    </TabBody>
  );
}
