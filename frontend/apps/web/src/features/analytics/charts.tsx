import type { ReactNode } from "react";
import {
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Legend, colors } from "./components";
import { bucketLabel, compactMoney, formatCount } from "./format";
import { formatMoneyText } from "@endosoflow/ui";

/*
 * The two time-series charts. Everything else on the dashboard is plain HTML bars, so every figure is
 * also text. Each chart sits in a figure with a text summary, and its data is available as a table.
 */

type Granularity = "day" | "week" | "month";

const axisTick = { fill: "var(--ink-3)", fontSize: 12 } as const;

interface TooltipRow {
  name?: string;
  value?: number | string;
  color?: string;
}

function ChartTooltip({
  active,
  payload,
  label,
  format,
}: {
  active?: boolean;
  payload?: TooltipRow[];
  label?: string | number;
  format: (value: number) => string;
}) {
  if (!active || !payload || payload.length === 0) return null;
  return (
    <div className="an-tooltip">
      <p className="an-tooltip-title">{label}</p>
      {payload.map((row) => (
        <p key={row.name} className="an-tooltip-row">
          <span
            className="an-swatch"
            style={{ background: row.color }}
            aria-hidden="true"
          />
          {row.name}: <strong>{format(Number(row.value ?? 0))}</strong>
        </p>
      ))}
    </div>
  );
}

function ChartFigure({
  summary,
  legend,
  table,
  children,
}: {
  summary: string;
  legend: readonly { label: string; color: string }[];
  table: ReactNode;
  children: ReactNode;
}) {
  return (
    <figure className="an-figure">
      <Legend items={legend} />
      <div className="an-chart" role="img" aria-label={summary}>
        {children}
      </div>
      <details className="an-table-toggle">
        <summary>Ver los datos como tabla</summary>
        <div className="table-scroll">{table}</div>
      </details>
    </figure>
  );
}

export interface TrendPoint {
  bucket: string;
  created: number;
  completed: number;
  cancelled: number;
}

/** Sales created, completed and cancelled per bucket of the period. */
export function TrendChart({
  data,
  granularity,
}: {
  data: readonly TrendPoint[];
  granularity: Granularity;
}) {
  const rows = data.map((d) => ({
    ...d,
    label: bucketLabel(d.bucket, granularity),
  }));
  const total = (key: "created" | "completed" | "cancelled") =>
    data.reduce((sum, d) => sum + d[key], 0);
  const legend = [
    { label: "Creadas", color: colors.onTime },
    { label: "Completadas", color: colors.done },
    { label: "Canceladas", color: colors.idle },
  ];
  return (
    <ChartFigure
      summary={`Ventas por ${granularity === "day" ? "día" : granularity === "week" ? "semana" : "mes"}: ${formatCount(total("created"))} creadas, ${formatCount(total("completed"))} completadas y ${formatCount(total("cancelled"))} canceladas en el periodo.`}
      legend={legend}
      table={
        <table className="table an-table">
          <thead>
            <tr>
              <th>Periodo</th>
              <th className="an-num">Creadas</th>
              <th className="an-num">Completadas</th>
              <th className="an-num">Canceladas</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.bucket}>
                <td>{r.label}</td>
                <td className="an-num">{r.created}</td>
                <td className="an-num">{r.completed}</td>
                <td className="an-num">{r.cancelled}</td>
              </tr>
            ))}
          </tbody>
        </table>
      }
    >
      <ResponsiveContainer width="100%" height={260}>
        <ComposedChart
          data={rows}
          margin={{ top: 8, right: 8, left: -14, bottom: 0 }}
        >
          <CartesianGrid stroke="var(--line)" vertical={false} />
          <XAxis
            dataKey="label"
            tick={axisTick}
            tickLine={false}
            axisLine={{ stroke: "var(--line-strong)" }}
            interval="preserveStartEnd"
            minTickGap={18}
          />
          <YAxis
            allowDecimals={false}
            tick={axisTick}
            tickLine={false}
            axisLine={false}
            width={44}
          />
          <Tooltip
            cursor={{ fill: "var(--surface-3)" }}
            content={<ChartTooltip format={formatCount} />}
          />
          <Bar
            dataKey="completed"
            name="Completadas"
            fill={colors.done}
            radius={[3, 3, 0, 0]}
            maxBarSize={26}
          />
          <Bar
            dataKey="cancelled"
            name="Canceladas"
            fill={colors.idle}
            radius={[3, 3, 0, 0]}
            maxBarSize={26}
          />
          <Line
            dataKey="created"
            name="Creadas"
            stroke={colors.onTime}
            strokeWidth={2.5}
            dot={false}
            activeDot={{ r: 4 }}
          />
        </ComposedChart>
      </ResponsiveContainer>
    </ChartFigure>
  );
}

export interface ValuePoint {
  bucket: string;
  createdValue: string;
  completedValue: string;
  /** Final commission of the completed sales; null when the caller may not see it. */
  commission: string | null;
}

/** Money per bucket: the value of the sales created, the value completed and, for managers, the commission. */
export function ValueChart({
  data,
  granularity,
}: {
  data: readonly ValuePoint[];
  granularity: Granularity;
}) {
  const hasCommission = data.some((d) => d.commission !== null);
  const rows = data.map((d) => ({
    label: bucketLabel(d.bucket, granularity),
    bucket: d.bucket,
    created: Number(d.createdValue),
    completed: Number(d.completedValue),
    commission: d.commission === null ? null : Number(d.commission),
  }));
  const sum = (pick: (r: (typeof rows)[number]) => number) =>
    rows.reduce((total, r) => total + pick(r), 0);
  const legend = [
    { label: "Valor de ventas creadas", color: colors.onTime },
    { label: "Valor de ventas completadas", color: colors.done },
    ...(hasCommission
      ? [{ label: "Comisión final", color: colors.approaching }]
      : []),
  ];
  const money = (n: number) => formatMoneyText(n.toFixed(2));
  return (
    <ChartFigure
      summary={`Valor por periodo: ${money(sum((r) => r.created))} en ventas creadas y ${money(sum((r) => r.completed))} en ventas completadas${hasCommission ? `, con ${money(sum((r) => r.commission ?? 0))} de comisión final` : ""}.`}
      legend={legend}
      table={
        <table className="table an-table">
          <thead>
            <tr>
              <th>Periodo</th>
              <th className="an-num">Creadas</th>
              <th className="an-num">Completadas</th>
              {hasCommission ? <th className="an-num">Comisión</th> : null}
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.bucket}>
                <td>{r.label}</td>
                <td className="an-num">{money(r.created)}</td>
                <td className="an-num">{money(r.completed)}</td>
                {hasCommission ? (
                  <td className="an-num">{money(r.commission ?? 0)}</td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
      }
    >
      <ResponsiveContainer width="100%" height={260}>
        <ComposedChart
          data={rows}
          margin={{ top: 8, right: 8, left: 0, bottom: 0 }}
        >
          <CartesianGrid stroke="var(--line)" vertical={false} />
          <XAxis
            dataKey="label"
            tick={axisTick}
            tickLine={false}
            axisLine={{ stroke: "var(--line-strong)" }}
            interval="preserveStartEnd"
            minTickGap={18}
          />
          <YAxis
            tick={axisTick}
            tickLine={false}
            axisLine={false}
            width={62}
            tickFormatter={compactMoney}
          />
          <Tooltip
            cursor={{ fill: "var(--surface-3)" }}
            content={<ChartTooltip format={money} />}
          />
          <Bar
            dataKey="completed"
            name="Ventas completadas"
            fill={colors.done}
            radius={[3, 3, 0, 0]}
            maxBarSize={26}
          />
          <Line
            dataKey="created"
            name="Ventas creadas"
            stroke={colors.onTime}
            strokeWidth={2.5}
            dot={false}
            activeDot={{ r: 4 }}
          />
          {hasCommission ? (
            <Line
              dataKey="commission"
              name="Comisión final"
              stroke={colors.approaching}
              strokeWidth={2.5}
              dot={false}
              activeDot={{ r: 4 }}
              connectNulls
            />
          ) : null}
        </ComposedChart>
      </ResponsiveContainer>
    </ChartFigure>
  );
}
