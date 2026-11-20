import type { ReactNode } from "react";
import { Callout } from "./Callout";
import { Modal } from "./Modal";

/**
 * Asks "are you sure?" before something that is hard to undo. The caller owns the work: it passes `busy`
 * while the request runs and `error` when it fails, so the dialog stays open and says why.
 */
export function ConfirmDialog({
  title,
  description,
  children,
  confirmLabel,
  busyLabel,
  tone = "default",
  busy = false,
  error,
  onConfirm,
  onClose,
}: {
  title: string;
  description?: string;
  /** What will happen, in a sentence or two. */
  children?: ReactNode;
  confirmLabel: string;
  /** The confirm button's text while `busy`; defaults to the label followed by an ellipsis. */
  busyLabel?: string;
  /** "danger" for what cannot be taken back easily. */
  tone?: "default" | "danger";
  busy?: boolean;
  error?: string | null;
  onConfirm: () => void;
  onClose: () => void;
}) {
  return (
    <Modal
      title={title}
      description={description}
      onClose={busy ? () => undefined : onClose}
      footer={
        <>
          <button
            type="button"
            className="button button-secondary"
            onClick={onClose}
            disabled={busy}
          >
            Cancelar
          </button>
          <button
            type="button"
            className={`button ${tone === "danger" ? "button-danger" : "button-primary"}`}
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? (busyLabel ?? `${confirmLabel}…`) : confirmLabel}
          </button>
        </>
      }
    >
      {children}
      {error && <Callout tone="error">{error}</Callout>}
    </Modal>
  );
}
