import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import type { UseQueryResult } from "@tanstack/react-query";
import { errorText } from "@endosoflow/api";
import { ErrorState, Icon, formatDateTime } from "@endosoflow/ui";
import { change, dateLabel, type Change } from "./format";

/* ---- the frame of a tab ---- */

function TabSkeleton() {
  return (
    <div className="an-grid" aria-busy="true" aria-label="Cargando">
      {Array.from({ length: 6 }, (_, i) => (
        <span key={i} className="skeleton an-skeleton" />
      ))}
    </div>
  );
}

/**
 * Shows a tab's answer. The first load shows placeholders and a failed first load shows an error with a
 * retry; once there is an answer it stays on screen while a new filter or the refresh loads (dimmed),
 * and a failed refresh keeps the last figures and says so.
 */
export function QueryBoundary<T>({
  query,
  what,
  children,
}: {
  query: UseQueryResult<T>;
  /** What was being loaded, for the error title: "el resumen". */
  what: string;
  children: (data: T) => ReactNode;
}) {
  if (!query.data) {
    if (query.isError) {
      return (
        <ErrorState
          title={`No se pudo cargar ${what}`}
          message={errorText(query.error)}
          onRetry={() => query.refetch()}
        />
      );
    }
    return <TabSkeleton />;
  }
  return (
    <div
      className={`an-tab${query.isFetching ? " is-refreshing" : ""}`}
      aria-busy={query.isFetching}
    >
      {query.isError ? (
        <p className="an-stale" role="status">
          No se pudo actualizar. Se muestran los últimos datos recibidos.
        </p>
      ) : null}
      {children(query.data)}
    </div>
  );
}

/** The dates the figures cover, what they are compared with and when they were computed. */
export function PeriodNote({
  meta,
  compare = true,
}: {
  meta: {
    from: string;
    to: string;
    previousFrom: string;
    previousTo: string;
    generatedAt: string;
  };
  compare?: boolean;
}) {
  return (
    <p className="an-period-note">
      {dateLabel(meta.from)} – {dateLabel(meta.to)}
      {compare
        ? ` · comparado con ${dateLabel(meta.previousFrom)} – ${dateLabel(meta.previousTo)}`
        : ""}
      {" · actualizado "}
      {formatDateTime(meta.generatedAt)}
    </p>
  );
}

/* ---- cards ---- */

export function Card({
  title,
  subtitle,
  aside,
  className = "",
  children,
}: {
  title: string;
  subtitle?: string;
  aside?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <section className={`panel an-card ${className}`.trim()}>
      <header className="an-card-head">
        <div>
          <h2 className="an-card-title">{title}</h2>
          {subtitle ? <p className="an-card-sub">{subtitle}</p> : null}
        </div>
        {aside}
      </header>
      {children}
    </section>
  );
}

/** A definition the person can open: how the figure is calculated. */
export function Definition({ children }: { children: ReactNode }) {
  return (
    <details className="an-def">
      <summary aria-label="Cómo se calcula">
        <Icon name="info" size={14} />
      </summary>
      <p>{children}</p>
    </details>
  );
}

export type Tone = "neutral" | "good" | "bad";

/** A movement against the previous period; green or red only when the direction means something. */
export function Delta({
  value,
  previous,
  goodWhen = "neutral",
}: {
  value: number;
  previous: number;
  /** Which direction is good news; "neutral" for figures that are neither (the number of sales created). */
  goodWhen?: "up" | "down" | "neutral";
}) {
  const result: Change = change(value, previous);
  if (result.text === null) return null;
  const tone: Tone =
    result.trend === "flat" || goodWhen === "neutral"
      ? "neutral"
      : result.trend === goodWhen
        ? "good"
        : "bad";
  const arrow =
    result.trend === "up" ? "▲" : result.trend === "down" ? "▼" : "•";
  return (
    <span className={`an-delta an-delta-${tone}`}>
      <span aria-hidden="true">{arrow}</span> {result.text}
      <span className="sr-only"> frente al periodo anterior</span>
    </span>
  );
}

export function KpiCard({
  label,
  value,
  note,
  tag,
  tone = "neutral",
  to,
  definition,
  children,
}: {
  label: string;
  value: string;
  /** A second line: the previous figure, a share, a count. */
  note?: ReactNode;
  /** "Ahora" for a snapshot, "Periodo" for a figure of the chosen dates. */
  tag?: "Ahora" | "Periodo";
  tone?: "neutral" | "late" | "warn" | "done";
  /** Opens the list behind the figure. */
  to?: string;
  definition?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <article className={`panel an-kpi an-kpi-${tone}`}>
      <header className="an-kpi-head">
        <span className="an-kpi-label">{label}</span>
        {tag ? <span className="an-tag">{tag}</span> : null}
        {definition ? <Definition>{definition}</Definition> : null}
      </header>
      <p className="an-kpi-value">
        {to ? (
          <Link to={to} className="an-kpi-link">
            {value}
          </Link>
        ) : (
          value
        )}
      </p>
      {note ? <p className="an-kpi-note">{note}</p> : null}
      {children}
    </article>
  );
}

/* ---- simple, accessible bars (HTML, so every figure is also text) ---- */

export interface Segment {
  label: string;
  value: number;
  color: string;
}

export function Legend({
  items,
}: {
  items: readonly { label: string; color: string }[];
}) {
  return (
    <ul className="an-legend">
      {items.map((item) => (
        <li key={item.label}>
          <span
            className="an-swatch"
            style={{ background: item.color }}
            aria-hidden="true"
          />
          {item.label}
        </li>
      ))}
    </ul>
  );
}

/** One bar split into segments, as wide as its share of `max`. */
export function StackBar({
  segments,
  max,
  label,
}: {
  segments: readonly Segment[];
  max: number;
  /** Read by screen readers: "Hito 3: 4 a tiempo, 1 con retraso". */
  label: string;
}) {
  const total = segments.reduce((sum, s) => sum + s.value, 0);
  return (
    <div
      className="an-stack"
      role="img"
      aria-label={label}
      style={{ width: max > 0 ? `${(total / max) * 100}%` : "0%" }}
    >
      {segments
        .filter((s) => s.value > 0)
        .map((s) => (
          <span
            key={s.label}
            className="an-stack-part"
            style={{ flexGrow: s.value, background: s.color }}
            title={`${s.label}: ${s.value}`}
          />
        ))}
    </div>
  );
}

/** A ranking: a label, a bar and the figures, one row each. */
export function RankList({
  rows,
  empty,
}: {
  rows: readonly {
    key: string;
    label: string;
    value: number;
    /** The figure shown at the right of the bar. */
    display: string;
    detail?: ReactNode;
    color?: string;
  }[];
  empty: string;
}) {
  if (rows.length === 0) return <p className="an-empty">{empty}</p>;
  const max = Math.max(...rows.map((r) => r.value), 1);
  return (
    <ul className="an-rank">
      {rows.map((row) => (
        <li key={row.key}>
          <div className="an-rank-head">
            <span className="an-rank-label">{row.label}</span>
            <span className="an-rank-value">{row.display}</span>
          </div>
          <div className="an-rank-track" aria-hidden="true">
            <span
              className="an-rank-fill"
              style={{
                width: `${(row.value / max) * 100}%`,
                background: row.color ?? "var(--doing)",
              }}
            />
          </div>
          {row.detail ? (
            <div className="an-rank-detail">{row.detail}</div>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

export function EmptyNote({ children }: { children: ReactNode }) {
  return <p className="an-empty">{children}</p>;
}

/** The colors of the dashboard, always the design system's meanings. */
export const colors = {
  onTime: "var(--doing)",
  approaching: "var(--warn)",
  overdue: "var(--brand)",
  idle: "var(--ink-4)",
  done: "var(--done)",
  neutral: "var(--ink-3)",
  soft: "var(--line-strong)",
} as const;
