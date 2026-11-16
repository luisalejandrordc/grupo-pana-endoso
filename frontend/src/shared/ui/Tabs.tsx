import { useRef, type KeyboardEvent } from "react";

export interface TabItem<T extends string> {
  id: T;
  label: string;
  /** Shown beside the label; hidden from the accessible name so the tab is still called by its label. */
  count?: number;
}

/**
 * A row of tabs. Arrow keys, Home and End move the focus between them and Enter or Space picks one
 * (manual activation, so a tab that asks "discard your changes?" is never triggered by browsing).
 */
export function Tabs<T extends string>({
  items,
  value,
  onChange,
  label,
  variant = "underline",
}: {
  items: readonly TabItem<T>[];
  value: T;
  onChange: (id: T) => void;
  label: string;
  /** "underline" for the sections of a page, "segmented" for the filters of a list. */
  variant?: "underline" | "segmented";
}) {
  const list = useRef<HTMLDivElement>(null);

  function onKeyDown(event: KeyboardEvent) {
    const tabs = [
      ...(list.current?.querySelectorAll<HTMLButtonElement>('[role="tab"]') ??
        []),
    ];
    const at = tabs.findIndex((tab) => tab === document.activeElement);
    if (at < 0) return;
    const next =
      event.key === "ArrowRight"
        ? (at + 1) % tabs.length
        : event.key === "ArrowLeft"
          ? (at - 1 + tabs.length) % tabs.length
          : event.key === "Home"
            ? 0
            : event.key === "End"
              ? tabs.length - 1
              : -1;
    if (next < 0) return;
    event.preventDefault();
    tabs[next].focus();
  }

  return (
    <div
      ref={list}
      className={variant === "segmented" ? "segmented" : "tabs"}
      role="tablist"
      aria-label={label}
      onKeyDown={onKeyDown}
    >
      {items.map((item) => (
        <button
          key={item.id}
          role="tab"
          aria-label={item.count === undefined ? undefined : item.label}
          aria-selected={value === item.id}
          tabIndex={value === item.id ? 0 : -1}
          className={value === item.id ? "is-active" : ""}
          onClick={() => onChange(item.id)}
        >
          {item.label}
          {item.count !== undefined && (
            <span className="tab-count" aria-hidden="true">
              {item.count}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}
