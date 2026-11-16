import type { ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

/** What a list or a page shows when there is nothing to show yet, or nothing matches. */
export function EmptyState({
  icon = "search",
  title,
  description,
  actions,
  tone = "neutral",
  page = false,
}: {
  icon?: IconName;
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  tone?: "neutral" | "error";
  /** A full-page state, on its own card. */
  page?: boolean;
}) {
  return (
    <div
      className={`empty${tone === "error" ? " is-error" : ""}${page ? " empty-page" : ""}`}
      role={tone === "error" ? "alert" : undefined}
    >
      <span className="empty-icon">
        <Icon name={icon} size={22} />
      </span>
      <strong>{title}</strong>
      {description && <p>{description}</p>}
      {actions && <div className="empty-actions">{actions}</div>}
    </div>
  );
}

/** A failed request, with the way to try again. */
export function ErrorState({
  title,
  message,
  onRetry,
  page = false,
  actions,
}: {
  title: string;
  message?: ReactNode;
  onRetry?: () => void;
  page?: boolean;
  actions?: ReactNode;
}) {
  return (
    <EmptyState
      tone="error"
      icon="alert"
      page={page}
      title={title}
      description={message}
      actions={
        <>
          {onRetry && (
            <button className="button button-secondary" onClick={onRetry}>
              <Icon name="refresh" size={15} /> Reintentar
            </button>
          )}
          {actions}
        </>
      }
    />
  );
}
