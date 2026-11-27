import type { ReactElement } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

/*
 * Every chart is a thin wrapper over Recharts that reads its colors from the design tokens, so the dashboard
 * matches the rest of the app. Each one is announced to screen readers by a plain-text summary (`summary`),
 * because the drawing itself is not readable by them.
 */

export const color = {
  created: "var(--doing)",
  completed: "var(--done)",
  cancelled: "var(--brand)",
  neutral: "var(--ink-4)",
  active: "var(--doing)",
  warn: "var(--warn)",
};

const axis = { fontSize: 12, fill: "var(--ink-3)" };
const grid = "var(--line)";

export interface Series {
  key: string;
  label: string;
  color: string;
}

const number = (v: unknown) =>
  typeof v === "number" ? v.toLocaleString("es-PE") : String(v ?? "");

function Frame({
  summary,
  height,
  children,
}: {
  summary: string;
  height: number;
  children: ReactElement;
}) {
  return (
    <div role="img" aria-label={summary} className="chart-frame">
      <ResponsiveContainer width="100%" height={height}>
        {children}
      </ResponsiveContainer>
    </div>
  );
}

/** Vertical grouped or stacked bars over a category axis (months, tasks...). */
export function ColumnChart({
  data,
  xKey,
  series,
  summary,
  stacked = false,
  height = 260,
  format = number,
}: {
  data: Record<string, string | number>[];
  xKey: string;
  series: Series[];
  summary: string;
  stacked?: boolean;
  height?: number;
  format?: (value: unknown) => string;
}) {
  return (
    <Frame summary={summary} height={height}>
      <BarChart
        data={data}
        margin={{ top: 8, right: 8, left: -8, bottom: 0 }}
        accessibilityLayer
      >
        <CartesianGrid stroke={grid} vertical={false} />
        <XAxis
          dataKey={xKey}
          tick={axis}
          tickLine={false}
          axisLine={{ stroke: grid }}
          interval={0}
        />
        <YAxis
          tick={axis}
          tickLine={false}
          axisLine={false}
          allowDecimals={false}
          tickFormatter={(v) => format(v)}
        />
        <Tooltip
          formatter={(v) => format(v)}
          cursor={{ fill: "var(--surface-3)" }}
        />
        {series.length > 1 && (
          <Legend iconType="circle" wrapperStyle={{ fontSize: 12 }} />
        )}
        {series.map((s) => (
          <Bar
            key={s.key}
            dataKey={s.key}
            name={s.label}
            fill={s.color}
            stackId={stacked ? "all" : undefined}
            radius={stacked ? 0 : [4, 4, 0, 0]}
            maxBarSize={36}
          />
        ))}
      </BarChart>
    </Frame>
  );
}

/** Horizontal bars, one row per label, for rankings and for "where are the sales now". */
export function RowChart({
  data,
  series,
  summary,
  stacked = false,
  labelWidth = 150,
  rowHeight = 34,
}: {
  data: Record<string, string | number>[];
  series: Series[];
  summary: string;
  stacked?: boolean;
  labelWidth?: number;
  rowHeight?: number;
}) {
  const height = Math.max(120, data.length * rowHeight + 36);
  return (
    <Frame summary={summary} height={height}>
      <BarChart
        data={data}
        layout="vertical"
        margin={{ top: 4, right: 16, left: 0, bottom: 0 }}
        accessibilityLayer
      >
        <CartesianGrid stroke={grid} horizontal={false} />
        <XAxis
          type="number"
          tick={axis}
          tickLine={false}
          axisLine={false}
          allowDecimals={false}
        />
        <YAxis
          type="category"
          dataKey="label"
          width={labelWidth}
          tick={axis}
          tickLine={false}
          axisLine={{ stroke: grid }}
          interval={0}
        />
        <Tooltip
          formatter={(v) => number(v)}
          cursor={{ fill: "var(--surface-3)" }}
        />
        {series.length > 1 && (
          <Legend iconType="circle" wrapperStyle={{ fontSize: 12 }} />
        )}
        {series.map((s) => (
          <Bar
            key={s.key}
            dataKey={s.key}
            name={s.label}
            fill={s.color}
            stackId={stacked ? "all" : undefined}
            radius={stacked ? 0 : [0, 4, 4, 0]}
            maxBarSize={22}
          />
        ))}
      </BarChart>
    </Frame>
  );
}
