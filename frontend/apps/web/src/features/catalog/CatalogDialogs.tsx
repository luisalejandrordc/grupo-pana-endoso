import { useState, type ReactNode } from "react";
import { Field } from "../../shared/ui/Field";
import { Modal } from "../../shared/ui/Modal";

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "Inténtalo nuevamente.";
}

/** A yes/no question before a change that is easy to regret. The caller does the work and may throw. */
export function ConfirmDialog({
  title,
  description,
  children,
  confirmLabel,
  danger = false,
  onConfirm,
  onClose,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  onConfirm: () => Promise<unknown>;
  onClose: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function confirm() {
    setBusy(true);
    setError("");
    try {
      await onConfirm();
      onClose();
    } catch (e) {
      setError(messageOf(e));
      setBusy(false);
    }
  }

  return (
    <Modal
      title={title}
      description={description}
      onClose={onClose}
      footer={
        <>
          <button className="button button-secondary" onClick={onClose}>
            Cancelar
          </button>
          <button
            className={`button ${danger ? "button-danger" : "button-primary"}`}
            disabled={busy}
            onClick={confirm}
          >
            {busy ? "Guardando…" : confirmLabel}
          </button>
        </>
      }
    >
      {children}
      {error && (
        <div className="form-alert" role="alert">
          {error}
        </div>
      )}
    </Modal>
  );
}

/** Permanent deletion: Admin only, and the reason goes to the audit trail. */
export function ReasonDialog({
  title,
  description,
  warning,
  onConfirm,
  onClose,
}: {
  title: string;
  description?: string;
  warning: string;
  onConfirm: (reason: string) => Promise<unknown>;
  onClose: () => void;
}) {
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function confirm() {
    if (!reason.trim()) {
      setError("Escribe el motivo de la eliminación.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onConfirm(reason.trim());
      onClose();
    } catch (e) {
      setError(messageOf(e));
      setBusy(false);
    }
  }

  return (
    <Modal
      title={title}
      description={description}
      onClose={onClose}
      footer={
        <>
          <button className="button button-secondary" onClick={onClose}>
            Cancelar
          </button>
          <button
            className="button button-danger"
            disabled={busy}
            onClick={confirm}
          >
            {busy ? "Eliminando…" : "Eliminar definitivamente"}
          </button>
        </>
      }
    >
      <p>{warning}</p>
      <Field label="Motivo" error={error}>
        <textarea
          rows={3}
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          placeholder="Por ejemplo: registro creado por error"
        />
      </Field>
    </Modal>
  );
}
