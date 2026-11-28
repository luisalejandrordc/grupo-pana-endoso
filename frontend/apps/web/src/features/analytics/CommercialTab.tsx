import { useState } from "react";
import { Tabs, formatMoneyText, type TabItem } from "@endosoflow/ui";
import type { CommercialData, GroupBy, SalesFilters } from "./api";
import { ValueChart } from "./charts";
import {
  Card,
  EmptyNote,
  Legend,
  PeriodNote,
  QueryBoundary,
  RankList,
  StackBar,
  colors,
} from "./components";
import { formatCount, formatPercent } from "./format";
import {
  cancelLabels,
  categoryLabels,
  customerTypeLabels,
  groupLabels,
  invoiceKindLabels,
  labelFor,
  temperatureLabels,
} from "./labels";
import { useCommercial } from "./queries";

const groupTabs: TabItem<GroupBy>[] = (
  ["model", "version", "color", "category"] as const
).map((id) => ({ id, label: groupLabels[id] }));

const partColors = {
  toyota: "var(--ink-3)",
  commission: "var(--done)",
  accessories: "var(--doing)",
  release: "var(--warn)",
} as const;

/** What sells, what is lost and the money behind it. It owns the vehicle grouping, which changes its query. */
export function CommercialTab({ filters }: { filters: SalesFilters }) {
  const [groupBy, setGroupBy] = useState<GroupBy>("model");
  const query = useCommercial(filters, groupBy, true);
  return (
    <QueryBoundary query={query} what="los datos comerciales">
      {(data) => (
        <CommercialView data={data} groupBy={groupBy} onGroupBy={setGroupBy} />
      )}
    </QueryBoundary>
  );
}

function CommercialView({
  data,
  groupBy,
  onGroupBy,
}: {
  data: CommercialData;
  groupBy: GroupBy;
  onGroupBy: (group: GroupBy) => void;
}) {
  const {
    vehicles,
    customerTypes,
    temperatures,
    cancellations,
    priceComposition,
    invoicedByKind,
    meta,
  } = data;
  const withCommission = (commission: string | null | undefined) =>
    commission === null || commission === undefined
      ? ""
      : ` · Comisión ${formatMoneyText(commission)}`;
  const parts = priceComposition
    ? [
        {
          key: "toyota",
          label: "Toyota",
          value: Number(priceComposition.toyota),
          color: partColors.toyota,
        },
        {
          key: "commission",
          label: "Comisión",
          value: Number(priceComposition.commission),
          color: partColors.commission,
        },
        {
          key: "accessories",
          label: "Accesorios",
          value: Number(priceComposition.accessories),
          color: partColors.accessories,
        },
        {
          key: "release",
          label: "Trámite de liberación",
          value: Number(priceComposition.releaseProcessing),
          color: partColors.release,
        },
      ]
    : [];
  const partsTotal = parts.reduce((sum, p) => sum + p.value, 0);

  return (
    <div className="stack an-sections">
      <PeriodNote meta={meta} />
      <Card
        title="Vehículos"
        subtitle="Ventas creadas en el periodo, las diez con más ventas"
        aside={
          <Tabs
            items={groupTabs}
            value={groupBy}
            onChange={onGroupBy}
            label="Agrupar vehículos por"
            variant="segmented"
          />
        }
      >
        <RankList
          empty="No se crearon ventas en este periodo."
          rows={vehicles.map((v) => ({
            key: v.key,
            label:
              groupBy === "category"
                ? labelFor(categoryLabels, v.label)
                : v.label,
            value: v.created,
            display: `${formatCount(v.created)} ${v.created === 1 ? "venta" : "ventas"}`,
            detail: `${formatCount(v.completed)} completadas · ${formatCount(v.active)} en curso · ${formatCount(v.cancelled)} canceladas · Valor ${formatMoneyText(v.quotedValue)}${
              v.averageTicket !== null
                ? ` · Ticket ${formatMoneyText(v.averageTicket)}`
                : ""
            }${withCommission(v.commission)}`,
          }))}
        />
        {groupBy === "category" && vehicles.length > 0 ? (
          <p className="an-footnote">
            Un modelo con varias categorías cuenta en cada una.
          </p>
        ) : null}
      </Card>

      <div className="an-pair">
        <Card
          title="Tipo de cliente"
          subtitle="Ventas creadas y su tasa de cierre"
        >
          <RankList
            empty="No se crearon ventas en este periodo."
            rows={customerTypes.map((c) => ({
              key: c.customerType,
              label: labelFor(customerTypeLabels, c.customerType),
              value: c.created,
              display: formatCount(c.created),
              detail: `Cierre ${formatPercent(c.closeRate.value)} (${formatCount(c.closeRate.numerator)} de ${formatCount(c.closeRate.denominator)}) · Valor ${formatMoneyText(c.quotedValue)}${withCommission(c.commission)}`,
            }))}
          />
        </Card>

        <Card
          title="Prioridad y resultado"
          subtitle="¿La prioridad elegida al crear la venta anticipa el resultado?"
        >
          <div className="table-scroll">
            <table className="table an-table">
              <thead>
                <tr>
                  <th>Prioridad</th>
                  <th className="an-num">Creadas</th>
                  <th className="an-num">Completadas</th>
                  <th className="an-num">Canceladas</th>
                  <th className="an-num">En curso</th>
                  <th className="an-num">Cierre</th>
                </tr>
              </thead>
              <tbody>
                {temperatures.map((t) => (
                  <tr key={t.temperature}>
                    <td>{labelFor(temperatureLabels, t.temperature)}</td>
                    <td className="an-num">{formatCount(t.created)}</td>
                    <td className="an-num">{formatCount(t.completed)}</td>
                    <td className="an-num">{formatCount(t.cancelled)}</td>
                    <td className="an-num">{formatCount(t.inProgress)}</td>
                    <td className="an-num">
                      <strong>{formatPercent(t.closeRate.value)}</strong>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="an-footnote">
            El cierre es completadas entre completadas más canceladas; las
            ventas en curso aún no cuentan.
          </p>
        </Card>
      </div>

      <Card
        title="Valor en el tiempo"
        subtitle="Valor de las ventas creadas y completadas por periodo"
      >
        <ValueChart data={data.valueTrend} granularity={meta.granularity} />
        <p className="an-footnote">
          Montos registrados en las tareas, en dólares. No son cifras contables.
        </p>
      </Card>

      <div className="an-pair">
        <Card
          title="Cancelaciones"
          subtitle={
            cancellations.total.saleCount === 0
              ? "Ventas canceladas en el periodo"
              : `${formatCount(cancellations.total.saleCount)} ${cancellations.total.saleCount === 1 ? "venta cancelada" : "ventas canceladas"} · ${formatMoneyText(cancellations.total.amount)} de valor`
          }
        >
          {cancellations.total.saleCount === 0 ? (
            <EmptyNote>No hubo cancelaciones en este periodo.</EmptyNote>
          ) : (
            <div className="stack">
              <div>
                <h3 className="an-subtitle">Por motivo</h3>
                <RankList
                  empty=""
                  rows={cancellations.byCategory.map((c) => ({
                    key: c.category,
                    label: labelFor(cancelLabels, c.category),
                    value: c.count,
                    display: `${formatCount(c.count)} · ${formatMoneyText(c.value)}`,
                    color: colors.idle,
                  }))}
                />
              </div>
              <div>
                <h3 className="an-subtitle">Tarea en la que estaba la venta</h3>
                <RankList
                  empty=""
                  rows={cancellations.byTask.map((t) => ({
                    key: String(t.taskNumber),
                    label: `${t.taskNumber}. ${t.name}`,
                    value: t.count,
                    display: formatCount(t.count),
                    color: colors.idle,
                  }))}
                />
              </div>
            </div>
          )}
        </Card>

        <div className="stack">
          {priceComposition ? (
            <Card
              title="Adónde va el precio"
              subtitle={`Ventas completadas en el periodo (${formatCount(priceComposition.saleCount)})`}
            >
              {priceComposition.saleCount === 0 || partsTotal <= 0 ? (
                <EmptyNote>
                  No se completó ninguna venta en este periodo.
                </EmptyNote>
              ) : (
                <>
                  <Legend items={parts} />
                  <StackBar
                    max={partsTotal}
                    label={`Composición del precio: ${parts.map((p) => `${p.label} ${formatPercent(p.value / partsTotal)}`).join(", ")}`}
                    segments={parts}
                  />
                  <ul className="an-breakdown">
                    {parts.map((p) => (
                      <li key={p.key}>
                        <span>{p.label}</span>
                        <span>
                          {formatMoneyText(p.value.toFixed(2))} ·{" "}
                          {formatPercent(p.value / partsTotal)}
                        </span>
                      </li>
                    ))}
                  </ul>
                </>
              )}
            </Card>
          ) : null}

          <Card
            title="Facturado por tipo"
            subtitle="Facturas con fecha en el periodo"
          >
            <RankList
              empty="No hay facturas en este periodo."
              rows={invoicedByKind.map((k) => ({
                key: k.kind,
                label: labelFor(invoiceKindLabels, k.kind),
                value: Number(k.amount),
                display: formatMoneyText(k.amount),
                detail: `${formatCount(k.count)} ${k.count === 1 ? "factura" : "facturas"}`,
                color: colors.done,
              }))}
            />
          </Card>
        </div>
      </div>
    </div>
  );
}
