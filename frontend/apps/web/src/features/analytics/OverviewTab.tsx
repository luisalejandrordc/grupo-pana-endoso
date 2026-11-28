import { Link } from "react-router-dom";
import { Badge, formatMoneyText } from "@endosoflow/ui";
import type { AttentionItem, Overview } from "./api";
import { TrendChart } from "./charts";
import {
  Card,
  Delta,
  EmptyNote,
  KpiCard,
  Legend,
  PeriodNote,
  StackBar,
  colors,
} from "./components";
import {
  change,
  formatCount,
  formatDuration,
  formatPercent,
  formatSpan,
} from "./format";

const MONEY_NOTE =
  "Montos registrados en las tareas, en dólares. No son cifras contables.";

function plural(count: number, one: string, many: string): string {
  return `${formatCount(count)} ${count === 1 ? one : many}`;
}

function attentionBadge(item: AttentionItem) {
  switch (item.reason) {
    case "overdue":
      return <Badge tone="late">Con retraso · {formatSpan(item.hours)}</Badge>;
    case "approaching":
      return <Badge tone="warn">Vence en {formatSpan(item.hours)}</Badge>;
    case "idle":
      return (
        <Badge tone="neutral">Sin iniciar · {formatSpan(item.hours)}</Badge>
      );
  }
}

/** The summary tab: what needs attention now, how the period went, the money and where the work sits. */
export function OverviewTab({
  data,
  showAdvisor,
}: {
  data: Overview;
  /** Managers and Admins see whose sale each row is; an Advisor only has their own. */
  showAdvisor: boolean;
}) {
  const { snapshot, period, money, pipeline, attention, meta } = data;
  const share = (count: number) =>
    snapshot.activeSales > 0
      ? `${formatPercent(count / snapshot.activeSales)} de las activas`
      : "Sin ventas activas";
  const cycleChange =
    period.cycleTime.medianHours !== null &&
    period.cycleTime.previousMedianHours !== null
      ? change(
          period.cycleTime.medianHours,
          period.cycleTime.previousMedianHours,
        )
      : null;
  const ticket = money.averageTicket;
  const maxActive = Math.max(...pipeline.map((m) => m.active), 1);
  const stages = [
    { label: "A tiempo", color: colors.onTime },
    { label: "Por vencer", color: colors.approaching },
    { label: "Con retraso", color: colors.overdue },
    { label: "Sin iniciar", color: colors.idle },
  ];

  return (
    <div className="stack an-sections">
      <PeriodNote meta={meta} />
      <section aria-labelledby="an-now">
        <h2 id="an-now" className="an-section-title">
          Ahora
        </h2>
        <div className="an-kpis">
          <KpiCard
            label="Ventas activas"
            value={formatCount(snapshot.activeSales)}
            tag="Ahora"
            to="/sales?state=active"
            definition="Ventas que no están completadas ni canceladas."
          />
          <KpiCard
            label="Con retraso"
            value={formatCount(snapshot.overdue)}
            tag="Ahora"
            tone={snapshot.overdue > 0 ? "late" : "neutral"}
            note={share(snapshot.overdue)}
            definition="Ventas activas cuya tarea en curso ya pasó su fecha límite."
          />
          <KpiCard
            label="Por vencer"
            value={formatCount(snapshot.approaching)}
            tag="Ahora"
            tone={snapshot.approaching > 0 ? "warn" : "neutral"}
            note={share(snapshot.approaching)}
            definition="Ventas activas cuya tarea en curso ya consumió el 80 % de su plazo recomendado y aún no vence."
          />
          <KpiCard
            label="Sin iniciar"
            value={formatCount(snapshot.idle)}
            tag="Ahora"
            note={share(snapshot.idle)}
            definition="Ventas activas sin tarea en curso cuya siguiente tarea está disponible hace más de 24 horas."
          />
        </div>
      </section>

      <section aria-labelledby="an-period">
        <h2 id="an-period" className="an-section-title">
          En el periodo
        </h2>
        <div className="an-kpis">
          <KpiCard
            label="Creadas"
            value={formatCount(period.created.value)}
            tag="Periodo"
            note={
              <>
                <Delta
                  value={period.created.value}
                  previous={period.created.previous}
                />{" "}
                Antes: {formatCount(period.created.previous)}
              </>
            }
          />
          <KpiCard
            label="Completadas"
            value={formatCount(period.completed.value)}
            tag="Periodo"
            tone="done"
            note={
              <>
                <Delta
                  value={period.completed.value}
                  previous={period.completed.previous}
                  goodWhen="up"
                />{" "}
                Antes: {formatCount(period.completed.previous)}
              </>
            }
          />
          <KpiCard
            label="Canceladas"
            value={formatCount(period.cancelled.value)}
            tag="Periodo"
            note={
              <>
                <Delta
                  value={period.cancelled.value}
                  previous={period.cancelled.previous}
                  goodWhen="down"
                />{" "}
                Antes: {formatCount(period.cancelled.previous)}
              </>
            }
          />
          <KpiCard
            label="Tasa de cierre"
            value={formatPercent(period.closeRate.value)}
            tag="Periodo"
            note={`${formatCount(period.closeRate.numerator)} de ${formatCount(period.closeRate.denominator)} resueltas · antes ${formatPercent(period.closeRate.previous)}`}
            definition="Ventas completadas entre ventas completadas más canceladas en el periodo. Las ventas aún en curso no cuentan."
          />
          <KpiCard
            label="Tiempo de ciclo"
            value={formatDuration(period.cycleTime.medianHours)}
            tag="Periodo"
            note={
              <>
                {cycleChange?.text ? (
                  <>
                    <Delta
                      value={period.cycleTime.medianHours ?? 0}
                      previous={period.cycleTime.previousMedianHours ?? 0}
                      goodWhen="down"
                    />{" "}
                  </>
                ) : null}
                Mediana · P90 {formatDuration(period.cycleTime.p90Hours)} ·{" "}
                {plural(period.cycleTime.sampleSize, "venta", "ventas")}
              </>
            }
            definition="Tiempo desde que se crea una venta hasta que se completa, para las ventas completadas en el periodo."
          />
        </div>
      </section>

      <section aria-labelledby="an-money">
        <h2 id="an-money" className="an-section-title">
          Valor y cobros
        </h2>
        <div className="an-kpis">
          <KpiCard
            label="Valor en curso"
            value={formatMoneyText(money.pipelineValue.amount)}
            tag="Ahora"
            note={`${plural(money.pipelineValue.saleCount, "venta activa", "ventas activas")}${
              money.pipelineValue.estimatedCount > 0
                ? ` · ${money.pipelineValue.estimatedCount} con precio de referencia`
                : ""
            }`}
            definition="Precio final cotizado de las ventas activas; mientras no hay cotización, el precio de referencia del catálogo."
          />
          <KpiCard
            label="Pendiente de pago"
            value={formatMoneyText(money.pendingCollection.amount)}
            tag="Ahora"
            note={plural(money.pendingCollection.saleCount, "venta", "ventas")}
            definition="Precio final cotizado de las ventas activas cuyo pago a Grupo Pana (tarea 4) aún no se completa."
          />
          <KpiCard
            label="Valor de ventas completadas"
            value={formatMoneyText(money.completedValue.value)}
            tag="Periodo"
            tone="done"
            note={
              <>
                <Delta
                  value={Number(money.completedValue.value)}
                  previous={Number(money.completedValue.previous)}
                  goodWhen="up"
                />{" "}
                Antes: {formatMoneyText(money.completedValue.previous)}
              </>
            }
          />
          <KpiCard
            label="Ticket promedio"
            value={formatMoneyText(ticket.value)}
            tag="Periodo"
            note={
              ticket.value !== null && ticket.previous !== null ? (
                <>
                  <Delta
                    value={Number(ticket.value)}
                    previous={Number(ticket.previous)}
                  />{" "}
                  Antes: {formatMoneyText(ticket.previous)}
                </>
              ) : undefined
            }
            definition="Precio final cotizado promedio de las ventas completadas en el periodo."
          />
          <KpiCard
            label="Cobrado"
            value={formatMoneyText(money.collected.value)}
            tag="Periodo"
            note={
              <>
                <Delta
                  value={Number(money.collected.value)}
                  previous={Number(money.collected.previous)}
                />{" "}
                Antes: {formatMoneyText(money.collected.previous)}
              </>
            }
            definition="Montos depositados a Grupo Pana (tarea 4) con fecha de pago en el periodo, en ventas no canceladas."
          />
          <KpiCard
            label="Facturado"
            value={formatMoneyText(money.invoiced.value)}
            tag="Periodo"
            note={
              <>
                <Delta
                  value={Number(money.invoiced.value)}
                  previous={Number(money.invoiced.previous)}
                />{" "}
                Antes: {formatMoneyText(money.invoiced.previous)}
              </>
            }
            definition="Importes de las facturas registradas (tarea 6) con fecha en el periodo, en ventas no canceladas."
          />
          <KpiCard
            label="Valor perdido por cancelaciones"
            value={formatMoneyText(money.lostValue.amount)}
            tag="Periodo"
            note={`${plural(money.lostValue.saleCount, "venta cancelada", "ventas canceladas")}${
              money.lostValue.estimatedCount > 0
                ? ` · ${money.lostValue.estimatedCount} con precio de referencia`
                : ""
            }`}
          />
          {money.internal ? (
            <>
              <KpiCard
                label="Comisión final"
                value={formatMoneyText(money.internal.commissionFinal.value)}
                tag="Periodo"
                tone="done"
                note={
                  <>
                    <Delta
                      value={Number(money.internal.commissionFinal.value)}
                      previous={Number(money.internal.commissionFinal.previous)}
                      goodWhen="up"
                    />{" "}
                    {formatPercent(money.internal.commissionRate.value)} del
                    precio cotizado
                  </>
                }
                definition="Comisión final (tarea 3) de las ventas completadas en el periodo."
              />
              <KpiCard
                label="Comisión inicial en curso"
                value={formatMoneyText(
                  money.internal.commissionPipeline.amount,
                )}
                tag="Ahora"
                note={plural(
                  money.internal.commissionPipeline.saleCount,
                  "venta con comisión",
                  "ventas con comisión",
                )}
                definition="Comisión inicial (tarea 1) de las ventas activas que ya la tienen."
              />
              <KpiCard
                label="Variación de comisión"
                value={formatMoneyText(money.internal.commissionDrift)}
                tag="Periodo"
                tone={
                  Number(money.internal.commissionDrift) < 0
                    ? "warn"
                    : "neutral"
                }
                note="Comisión final menos inicial"
                definition="Suma, en las ventas completadas en el periodo, de la comisión final menos la comisión inicial. Negativa cuando la comisión bajó entre la cotización y la liquidación."
              />
              <KpiCard
                label="Pagado a Toyota"
                value={formatMoneyText(money.internal.paidToToyota.value)}
                tag="Periodo"
                note={
                  <>
                    <Delta
                      value={Number(money.internal.paidToToyota.value)}
                      previous={Number(money.internal.paidToToyota.previous)}
                    />{" "}
                    Antes:{" "}
                    {formatMoneyText(money.internal.paidToToyota.previous)}
                  </>
                }
                definition="Montos depositados a Toyota (tarea 5) con fecha de pago en el periodo, en ventas no canceladas."
              />
              <KpiCard
                label="Diferencias de monto"
                value={formatCount(money.internal.amountMismatches.count)}
                tag="Periodo"
                tone={
                  money.internal.amountMismatches.count > 0 ? "warn" : "neutral"
                }
                note={`Diferencia total ${formatMoneyText(money.internal.amountMismatches.absoluteDifference)}`}
                definition="Pagos y facturas del periodo cuyo monto registrado difiere del esperado."
              />
            </>
          ) : null}
        </div>
        <p className="an-footnote">{MONEY_NOTE}</p>
      </section>

      <div className="an-pair">
        <Card
          title="Ventas activas por hito"
          subtitle="Dónde está el trabajo ahora y cuánto va con retraso"
        >
          {snapshot.activeSales === 0 ? (
            <EmptyNote>No hay ventas activas con estos filtros.</EmptyNote>
          ) : (
            <>
              <Legend items={stages} />
              <ul className="an-pipeline">
                {pipeline.map((m) => (
                  <li key={m.milestoneNumber}>
                    <span className="an-pipe-label">
                      {m.milestoneNumber}. {m.name}
                    </span>
                    <div className="an-pipe-bar">
                      <StackBar
                        max={maxActive}
                        label={`${m.name}: ${m.onTime} a tiempo, ${m.approaching} por vencer, ${m.overdue} con retraso, ${m.idle} sin iniciar`}
                        segments={[
                          {
                            label: "A tiempo",
                            value: m.onTime,
                            color: colors.onTime,
                          },
                          {
                            label: "Por vencer",
                            value: m.approaching,
                            color: colors.approaching,
                          },
                          {
                            label: "Con retraso",
                            value: m.overdue,
                            color: colors.overdue,
                          },
                          {
                            label: "Sin iniciar",
                            value: m.idle,
                            color: colors.idle,
                          },
                        ]}
                      />
                    </div>
                    <span className="an-pipe-count">
                      {formatCount(m.active)}
                      {m.active > 0 ? (
                        <span className="an-pipe-value">
                          {" "}
                          · {formatMoneyText(m.value)}
                        </span>
                      ) : null}
                    </span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </Card>

        <Card
          title="Ventas en el tiempo"
          subtitle="Creadas, completadas y canceladas en el periodo"
        >
          <TrendChart data={data.trend} granularity={meta.granularity} />
        </Card>
      </div>

      <Card
        title="Requieren atención"
        subtitle="Con retraso primero, luego por vencer y sin iniciar"
        aside={
          attention.total > 0 ? (
            <span className="an-card-count">
              {Math.min(attention.items.length, attention.total)} de{" "}
              {formatCount(attention.total)}
            </span>
          ) : null
        }
      >
        {attention.items.length === 0 ? (
          <EmptyNote>
            Nada requiere atención ahora. Ninguna venta activa está con retraso,
            por vencer o sin iniciar.
          </EmptyNote>
        ) : (
          <div className="table-scroll">
            <table className="table an-table">
              <thead>
                <tr>
                  <th>Venta</th>
                  <th>Cliente</th>
                  <th>Vehículo</th>
                  <th>Tarea</th>
                  {showAdvisor ? <th>Asesor</th> : null}
                  <th>Situación</th>
                  <th className="an-num">Valor</th>
                </tr>
              </thead>
              <tbody>
                {attention.items.map((item) => (
                  <tr key={item.saleId}>
                    <td>
                      <Link to={`/sales/${item.saleId}`} className="code">
                        {item.code}
                      </Link>
                    </td>
                    <td>{item.customerName}</td>
                    <td>{item.vehicle}</td>
                    <td>
                      {item.taskNumber}. {item.taskName}
                    </td>
                    {showAdvisor ? <td>{item.advisorName}</td> : null}
                    <td>{attentionBadge(item)}</td>
                    <td className="an-num">{formatMoneyText(item.value)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
