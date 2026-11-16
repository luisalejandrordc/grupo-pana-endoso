import { useEffect, useId, useRef, type ReactNode } from "react";
import { Icon } from "./Icon";

const FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

/**
 * A dialog. Escape and a click on the backdrop close it, focus moves inside when it opens and goes back to
 * where it was when it closes, Tab stays inside it, and the page behind does not scroll.
 */
export function Modal({
  title,
  description,
  children,
  onClose,
  footer,
  wide = false,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
  /** A wider window for screens that hold tables or image grids. */
  wide?: boolean;
}) {
  const titleId = useId();
  const dialog = useRef<HTMLElement>(null);
  const close = useRef(onClose);
  close.current = onClose;

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const node = dialog.current;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    // Focus the first field when there is one, otherwise the first control.
    const first =
      node?.querySelector<HTMLElement>(
        ".modal-content input,.modal-content textarea,.modal-content select",
      ) ?? node?.querySelector<HTMLElement>(FOCUSABLE);
    first?.focus();

    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.stopPropagation();
        close.current();
        return;
      }
      if (event.key !== "Tab" || !node) return;
      const items = [...node.querySelectorAll<HTMLElement>(FOCUSABLE)];
      if (items.length === 0) return;
      const head = items[0];
      const tail = items[items.length - 1];
      if (event.shiftKey && document.activeElement === head) {
        event.preventDefault();
        tail.focus();
      } else if (!event.shiftKey && document.activeElement === tail) {
        event.preventDefault();
        head.focus();
      }
    }
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
      previous?.focus?.();
    };
  }, []);

  return (
    <div
      className="modal-backdrop"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <section
        ref={dialog}
        className={wide ? "modal modal-wide" : "modal"}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <div className="modal-heading">
          <div>
            <h2 id={titleId}>{title}</h2>
            {description && <p>{description}</p>}
          </div>
          <button
            className="icon-button"
            aria-label="Cerrar ventana"
            onClick={onClose}
          >
            <Icon name="x" />
          </button>
        </div>
        <div className="modal-content">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </section>
    </div>
  );
}
