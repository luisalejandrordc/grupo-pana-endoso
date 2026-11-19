import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Icon } from "./Icon";
import type { Toast } from "./toast";
import { ToastContext } from "./toastContext";

interface Entry {
  id: number;
  toast: Toast;
}

const MAX_VISIBLE = 4;

/** Errors stay longer than confirmations: they usually need to be read, and sometimes acted on. */
const lifetime = (toast: Toast) => (toast.tone === "error" ? 8000 : 4500);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [entries, setEntries] = useState<Entry[]>([]);
  const next = useRef(0);

  const notify = useCallback((toast: Toast) => {
    const id = (next.current += 1);
    setEntries((old) => [...old, { id, toast }].slice(-MAX_VISIBLE));
  }, []);
  const dismiss = useCallback(
    (id: number) => setEntries((old) => old.filter((entry) => entry.id !== id)),
    [],
  );

  return (
    <ToastContext.Provider value={notify}>
      {children}
      <div className="toast-viewport">
        {entries.map((entry) => (
          <ToastItem key={entry.id} entry={entry} onDismiss={dismiss} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

function ToastItem({
  entry,
  onDismiss,
}: {
  entry: Entry;
  onDismiss: (id: number) => void;
}) {
  const { id, toast } = entry;
  const [paused, setPaused] = useState(false);

  // The timer restarts when the pointer leaves, so a message being read is never taken away.
  useEffect(() => {
    if (paused) return;
    const timer = window.setTimeout(() => onDismiss(id), lifetime(toast));
    return () => window.clearTimeout(timer);
  }, [paused, id, toast, onDismiss]);

  return (
    <div
      className={`toast toast-${toast.tone}`}
      role={toast.tone === "error" ? "alert" : "status"}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <span className="toast-icon">
        <Icon
          name={
            toast.tone === "success"
              ? "check-circle"
              : toast.tone === "error"
                ? "circle-x"
                : "info"
          }
          size={19}
        />
      </span>
      <div>
        <strong>{toast.title}</strong>
        {toast.message && <p>{toast.message}</p>}
      </div>
      <button
        className="icon-button"
        aria-label="Cerrar notificación"
        onClick={() => onDismiss(id)}
      >
        <Icon name="x" size={15} />
      </button>
    </div>
  );
}
