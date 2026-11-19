import { useEffect, useRef, useState, type ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

export interface MenuItem {
  label: string;
  onSelect: () => void;
  icon?: IconName;
  danger?: boolean;
}

/**
 * A button that opens a short list of actions. Arrow keys move through the items, Escape and a click
 * outside close it, and the focus goes back to the button.
 */
export function Menu({
  label,
  items,
  children,
}: {
  label: string;
  items: MenuItem[];
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const buttons = () => [
      ...(root.current?.querySelectorAll<HTMLButtonElement>(
        "[role=menuitem]",
      ) ?? []),
    ];
    buttons()[0]?.focus();

    function close(returnFocus: boolean) {
      setOpen(false);
      if (returnFocus) trigger.current?.focus();
    }
    function onPointer(event: MouseEvent) {
      if (root.current && !root.current.contains(event.target as Node))
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
    document.addEventListener("mousedown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  if (items.length === 0) return null;
  return (
    <div className="menu" ref={root}>
      <button
        ref={trigger}
        className="button button-secondary"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={label}
        onClick={() => setOpen((value) => !value)}
      >
        {children}
        <Icon name="chevron-down" size={15} />
      </button>
      {open && (
        <div className="menu-list" role="menu">
          {items.map((item) => (
            <button
              key={item.label}
              role="menuitem"
              className={item.danger ? "is-danger" : ""}
              onClick={() => {
                setOpen(false);
                trigger.current?.focus();
                item.onSelect();
              }}
            >
              {item.icon && <Icon name={item.icon} size={16} />}
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
