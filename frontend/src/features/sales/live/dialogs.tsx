import { useState, type ReactNode } from "react";
import { Field } from "../../../shared/ui/Field";
import { Icon } from "../../../shared/ui/Icon";
import { Modal } from "../../../shared/ui/Modal";
import type { Advisor, ApiSale, CancelCategory } from "./api";
import {
  cancelCategories,
  cancelLabels,
  errorText,
  fieldErrors,
} from "./labels";

/** The footer every dialog shares: cancel on the left, the confirming action on the right. */
function DialogFooter({
  confirmLabel,
  busy,
  disabled,
  danger,
  onClose,
  onConfirm,
}: {
  confirmLabel: string;
  busy: boolean;
  disabled?: boolean;
  danger?: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <>
      <button
        className="button button-secondary"
        onClick={onClose}
        disabled={busy}
      >
        Volver
      </button>
      <button
        className={`button ${danger ? "button-danger" : "button-primary"}`}
        onClick={onConfirm}
        disabled={busy || disabled}
      >
        {busy ? "Procesando…" : confirmLabel}
      </button>
    </>
  );
}

/** Runs the confirming action and keeps the dialog open, with the server's message, when it is refused. */
function useAction(run: () => Promise<unknown>, onDone: () => void) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fields, setFields] = useState<Record<string, string>>({});
  async function submit() {
    setBusy(true);
    setError(null);
    setFields({});
    try {
      await run();
      onDone();
    } catch (caught) {
      setError(errorText(caught));
      setFields(fieldErrors(caught));
    } finally {
      setBusy(false);
    }
  }
  return { busy, error, fields, submit };
}

function ErrorLine({ message }: { message: string | null }) {
  if (!message) return null;
  return (
    <div className="form-alert" role="alert">
      <Icon name="alert" size={16} /> {message}
    </div>
  );
}

/** Asks for a written reason (restart, reopen, correction, replacement). */
export function ReasonDialog({
  title,
  description,
  label,
  confirmLabel,
  danger,
  warning,
  onConfirm,
  onClose,
}: {
  title: string;
  description?: string;
  label: string;
  confirmLabel: string;
  danger?: boolean;
  warning?: ReactNode;
  onConfirm: (reason: string) => Promise<unknown>;
  onClose: () => void;
}) {
  const [reason, setReason] = useState("");
  const clean = reason.trim();
  const { busy, error, fields, submit } = useAction(
    () => onConfirm(clean),
    onClose,
  );
  return (
    <Modal
      title={title}
      description={description}
      onClose={busy ? () => undefined : onClose}
      footer={
        <DialogFooter
          confirmLabel={confirmLabel}
          busy={busy}
          disabled={clean === "" || clean.length > 500}
          danger={danger}
          onClose={onClose}
          onConfirm={submit}
        />
      }
    >
      <Field
        label={label}
        hint="Queda registrado en el historial."
        error={fields.reason}
      >
        <textarea
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          rows={3}
          maxLength={500}
          autoFocus
        />
      </Field>
      {warning && (
        <div className="modal-warning">
          <Icon name="alert" size={16} /> {warning}
        </div>
      )}
      <ErrorLine message={error} />
    </Modal>
  );
}

/** A plain yes or no. */
export function ConfirmDialog({
  title,
  message,
  confirmLabel,
  danger,
  onConfirm,
  onClose,
}: {
  title: string;
  message: string;
  confirmLabel: string;
  danger?: boolean;
  onConfirm: () => Promise<unknown>;
  onClose: () => void;
}) {
  const { busy, error, submit } = useAction(onConfirm, onClose);
  return (
    <Modal
      title={title}
      onClose={busy ? () => undefined : onClose}
      footer={
        <DialogFooter
          confirmLabel={confirmLabel}
          busy={busy}
          danger={danger}
          onClose={onClose}
          onConfirm={submit}
        />
      }
    >
      <p>{message}</p>
      <ErrorLine message={error} />
    </Modal>
  );
}

export function CancelSaleDialog({
  onConfirm,
  onClose,
}: {
  onConfirm: (category: CancelCategory, text: string) => Promise<unknown>;
  onClose: () => void;
}) {
  const [category, setCategory] = useState<CancelCategory>("customer_withdrew");
  const [text, setText] = useState("");
  const clean = text.trim();
  const { busy, error, fields, submit } = useAction(
    () => onConfirm(category, clean),
    onClose,
  );
  return (
    <Modal
      title="Cancelar venta"
      description="La venta y sus tareas pasan a modo lectura hasta que un responsable la reabra."
      onClose={busy ? () => undefined : onClose}
      footer={
        <DialogFooter
          confirmLabel="Cancelar venta"
          danger
          busy={busy}
          disabled={category === "other" && clean === ""}
          onClose={onClose}
          onConfirm={submit}
        />
      }
    >
      <Field label="Motivo de cancelación" error={fields.category}>
        <select
          value={category}
          onChange={(event) =>
            setCategory(event.target.value as CancelCategory)
          }
        >
          {cancelCategories.map((item) => (
            <option key={item} value={item}>
              {cancelLabels[item]}
            </option>
          ))}
        </select>
      </Field>
      <Field
        label="Detalle del motivo"
        hint={
          category === "other"
            ? "Obligatorio para «Otro motivo»."
            : "Opcional: agrega contexto para el historial."
        }
        error={fields.text}
      >
        <textarea
          value={text}
          onChange={(event) => setText(event.target.value)}
          rows={3}
          maxLength={500}
        />
      </Field>
      <ErrorLine message={error} />
    </Modal>
  );
}

export function ReassignDialog({
  sale,
  advisors,
  loading,
  onConfirm,
  onClose,
}: {
  sale: ApiSale;
  advisors: Advisor[];
  loading: boolean;
  onConfirm: (advisorId: string) => Promise<unknown>;
  onClose: () => void;
}) {
  const options = advisors.filter((advisor) => advisor.id !== sale.advisorId);
  const [advisorId, setAdvisorId] = useState("");
  const { busy, error, fields, submit } = useAction(
    () => onConfirm(advisorId),
    onClose,
  );
  return (
    <Modal
      title="Reasignar asesor"
      description={`Asesor actual: ${sale.advisorName}. El asesor anterior pierde el acceso a la venta de inmediato.`}
      onClose={busy ? () => undefined : onClose}
      footer={
        <DialogFooter
          confirmLabel="Reasignar"
          busy={busy}
          disabled={advisorId === ""}
          onClose={onClose}
          onConfirm={submit}
        />
      }
    >
      <Field label="Nuevo asesor responsable" error={fields.advisorId}>
        <select
          value={advisorId}
          onChange={(event) => setAdvisorId(event.target.value)}
          disabled={loading}
        >
          <option value="">
            {loading ? "Cargando asesores…" : "Selecciona un asesor"}
          </option>
          {options.map((advisor) => (
            <option key={advisor.id} value={advisor.id}>
              {advisor.firstName} {advisor.lastName}
            </option>
          ))}
        </select>
      </Field>
      <ErrorLine message={error} />
    </Modal>
  );
}

export function DeleteSaleDialog({
  sale,
  onConfirm,
  onClose,
}: {
  sale: ApiSale;
  onConfirm: (reason: string, confirmation: string) => Promise<unknown>;
  onClose: () => void;
}) {
  const [reason, setReason] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const { busy, error, fields, submit } = useAction(
    () => onConfirm(reason.trim(), confirmation.trim()),
    onClose,
  );
  return (
    <Modal
      title="Eliminar venta definitivamente"
      description="Solo para ventas de prueba o con datos corruptos que nunca llegaron a la entrega. Esta acción no se puede deshacer."
      onClose={busy ? () => undefined : onClose}
      footer={
        <DialogFooter
          confirmLabel="Eliminar definitivamente"
          danger
          busy={busy}
          disabled={reason.trim() === "" || confirmation.trim() === ""}
          onClose={onClose}
          onConfirm={submit}
        />
      }
    >
      <div className="modal-warning">
        <Icon name="alert" size={16} /> Se eliminan las 13 tareas, los datos
        financieros, los pagos, las facturas y los documentos. El historial de
        auditoría se conserva.
      </div>
      <Field label="Motivo de la eliminación" error={fields.reason}>
        <textarea
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          rows={2}
          maxLength={500}
        />
      </Field>
      <Field
        label={`Para confirmar, escribe el documento del cliente (${sale.customerDocumentNumber})`}
        error={fields.confirmation}
      >
        <input
          value={confirmation}
          onChange={(event) => setConfirmation(event.target.value)}
          autoComplete="off"
        />
      </Field>
      <ErrorLine message={error} />
    </Modal>
  );
}
