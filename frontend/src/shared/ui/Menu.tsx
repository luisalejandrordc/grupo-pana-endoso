import { useEffect, useRef, useState, type ReactNode } from "react";
import { Icon } from "./Icon";

export interface MenuItem {
  label: string;
  onSelect: () => void;
  icon?: Parameters<typeof Icon>[0]["name"];
  danger?: boolean;
}

/** A button that opens a short list of actions. Closes on a click outside, on Escape and after a choice. */
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

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent) {
        if (event.key === "Escape") setOpen(false);
        return;
      }
      if (root.current && !root.current.contains(event.target as Node))
        setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);

  if (items.length === 0) return null;
  return (
    <div className="menu" ref={root}>
      <button
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
