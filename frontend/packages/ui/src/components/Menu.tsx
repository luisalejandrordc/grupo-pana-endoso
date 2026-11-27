import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Icon, type IconName } from "./Icon";

export interface MenuItem {
  label: string;
  onSelect: () => void;
  icon?: IconName;
  danger?: boolean;
  disabled?: boolean;
}

interface Position {
  right: number;
  top?: number;
  bottom?: number;
}

// Matches the CSS (a 38px row, 6px padding around the list), so the list can be placed before it is drawn.
const ROW = 38;
const PADDING = 14;
const GAP = 6;

/**
 * A button that opens a short list of actions. Arrow keys move through the items, Escape and a click
 * outside close it, and the focus goes back to the button.
 *
 * The list is drawn on the page, not inside the button's container, so a menu in the last row of a table
 * is never cut off by the table's scrolling. It opens upward when there is no room below.
 */
export function Menu({
  label,
  items,
  children,
  triggerClassName = "button button-secondary",
}: {
  label: string;
  items: MenuItem[];
  children: ReactNode;
  /** How the button looks. Secondary by default; a top bar uses a quieter one. */
  triggerClassName?: string;
}) {
  const [position, setPosition] = useState<Position | null>(null);
  const open = position !== null;
  const trigger = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLDivElement>(null);

  function openMenu() {
    const rect = trigger.current?.getBoundingClientRect();
    if (!rect) return;
    const height = items.length * ROW + PADDING;
    const room = window.innerHeight - rect.bottom;
    const flip = room < height + GAP * 2 && rect.top > height + GAP * 2;
    setPosition({
      right: Math.max(8, window.innerWidth - rect.right),
      ...(flip
        ? { bottom: window.innerHeight - rect.top + GAP }
        : { top: rect.bottom + GAP }),
    });
  }

  useEffect(() => {
    if (!open) return;
    const buttons = () => [
      ...(list.current?.querySelectorAll<HTMLButtonElement>(
        "[role=menuitem]:not(:disabled)",
      ) ?? []),
    ];
    buttons()[0]?.focus();

    function close(returnFocus: boolean) {
      setPosition(null);
      if (returnFocus) trigger.current?.focus();
    }
    function onPointer(event: MouseEvent) {
      const target = event.target as Node;
      if (!list.current?.contains(target) && !trigger.current?.contains(target))
        close(false);
    }
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") return close(true);
      if (event.key === "Tab") return close(false);
      const all = buttons();
      const at = all.findIndex((item) => item === document.activeElement);
      const move =
        event.key === "ArrowDown"
          ? (at + 1) % all.length
          : event.key === "ArrowUp"
            ? (at - 1 + all.length) % all.length
            : event.key === "Home"
              ? 0
              : event.key === "End"
                ? all.length - 1
                : -1;
      if (move < 0) return;
      event.preventDefault();
      all[move]?.focus();
    }
    // The list is placed once, so it must not drift from its button when the page moves.
    const away = () => close(false);
    document.addEventListener("mousedown", onPointer);
    document.addEventListener("keydown", onKey);
    window.addEventListener("scroll", away, true);
    window.addEventListener("resize", away);
    return () => {
      document.removeEventListener("mousedown", onPointer);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", away, true);
      window.removeEventListener("resize", away);
    };
  }, [open]);

  if (items.length === 0) return null;
  return (
    <div className="menu">
      <button
        ref={trigger}
        className={triggerClassName}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={label}
        onClick={() => (open ? setPosition(null) : openMenu())}
      >
        {children}
        <Icon name="chevron-down" size={15} />
      </button>
      {open &&
        createPortal(
          <div
            ref={list}
            className="menu-list"
            role="menu"
            aria-label={label}
            style={position}
          >
            {items.map((item) => (
              <button
                key={item.label}
                role="menuitem"
                disabled={item.disabled}
                className={item.danger ? "is-danger" : ""}
                onClick={() => {
                  setPosition(null);
                  trigger.current?.focus();
                  item.onSelect();
                }}
              >
                {item.icon && <Icon name={item.icon} size={16} />}
                {item.label}
              </button>
            ))}
          </div>,
          document.body,
        )}
    </div>
  );
}
