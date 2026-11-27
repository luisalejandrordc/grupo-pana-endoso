import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Badge, ErrorState, formatDateTime } from "@endosoflow/ui";
import { errorText } from "@endosoflow/api";

export type KpiTone = "neutral" | "late" | "warn" | "done";

/**
 * One headline number. A snapshot describes today and ignores the period, so it carries a "Hoy" marker: nobody
 * should read "tasks overdue now" as if it were a figure for the chosen dates.
 */
export function KpiCard({
  label,
  value,
  hint,
  tone = "neutral",
  snapshot = false,
  to,
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: KpiTone;
  snapshot?: boolean;
  /** Where the card leads, when the number has a list behind it. */
  to?: string;
}) {
  const body = (
    <>
      <span className="kpi-label">
        {label}
        {snapshot && <Badge plain>Hoy</Badge>}
      </span>
      <strong className="kpi-value">{value}</strong>
      {hint && <span className="kpi-hint">{hint}</span>}
    </>
  );
  const className = `kpi is-${tone}${to ? " is-link" : ""}`;
  return to ? (
    <Link className={className} to={to}>
      {body}
    </Link>
  ) : (
    <div className={className}>{body}</div>
  );
}

export function KpiGrid({ children }: { children: ReactNode }) {
  return <div className="dash-kpis">{children}</div>;
}

/** A titled panel for a chart or a table, with a line of context and room for a note when there is no data. */
export function ChartCard({
  title,
  subtitle,
  children,
  empty,
  emptyText = "Todavía no hay datos para este periodo.",
  className = "",
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
  empty?: boolean;
  emptyText?: string;
  className?: string;
}) {
  return (
    <section className={`panel chart-card ${className}`}>
      <header className="chart-card-head">
        <h2>{title}</h2>
        {subtitle && <p>{subtitle}</p>}
      </header>
      {empty ? <p className="chart-empty">{emptyText}</p> : children}
    </section>
  );
}

/** What a tab shows while its numbers load: the shape of the page, so nothing jumps when they arrive. */
export function TabSkeleton() {
  return (
    <div className="stack" aria-busy="true" aria-label="Cargando datos">
      <div className="dash-kpis">
        {[0, 1, 2, 3].map((i) => (
          <span key={i} className="skeleton skeleton-kpi" />
        ))}
      </div>
      <span className="skeleton skeleton-chart" />
    </div>
  );
}

export function TabError({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry: () => void;
}) {
  return (
    <ErrorState
      title="No pudimos cargar este panel"
      message={errorText(error)}
      onRetry={onRetry}
    />
  );
}

/** "Actualizado hace..." line with the moment the numbers were computed. */
export function Freshness({ at }: { at: string }) {
  return <p className="dash-fresh">Actualizado: {formatDateTime(at)}</p>;
}

/** Shows the tab's data when it has it, the skeleton on the first load and the error with a retry otherwise. */
export function TabBody<T>({
  query,
  children,
}: {
  query: {
    data: T | undefined;
    isPending: boolean;
    isError: boolean;
    error: unknown;
    isPlaceholderData: boolean;
    refetch: () => unknown;
  };
  children: (data: T) => ReactNode;
}) {
  if (query.isPending) return <TabSkeleton />;
  if (query.isError && query.data === undefined)
    return (
      <TabError error={query.error} onRetry={() => void query.refetch()} />
    );
  return (
    <div
      className={`stack${query.isPlaceholderData ? " is-refreshing" : ""}`}
      aria-busy={query.isPlaceholderData}
    >
      {children(query.data as T)}
    </div>
  );
}
