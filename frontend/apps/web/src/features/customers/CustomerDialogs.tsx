import { useState } from "react";
import { ApiError } from "../../shared/api/client";
import { Field } from "../../shared/ui/Field";
import { Modal } from "../../shared/ui/Modal";
import type { Toast } from "../../shared/ui/toast";
import type { ApiCustomer } from "./api";
import { useCustomerStatus, useDeleteCustomer } from "./queries";

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "Inténtalo nuevamente.";
}

/** Confirms deactivating or reactivating a customer. */
export function StatusDialog({
  customer,
  onClose,
  onDone,
  toast,
}: {
  customer: ApiCustomer;
  onClose: () => void;
  onDone: (customer: ApiCustomer) => void;
  toast: (toast: Toast) => void;
}) {
  const mutation = useCustomerStatus(customer.id);
  const reactivating = !customer.isActive;
  const [error, setError] = useState("");

  async function confirm() {
    setError("");
    try {
      const updated = await mutation.mutateAsync(reactivating);
      toast({
        title: reactivating ? "Cliente reactivado" : "Cliente desactivado",
        message: reactivating
          ? "Ya puede recibir nuevas ventas."
          : "No recibirá nuevas ventas ni acceso al portal. Su historial se conserva.",
        tone: "success",
      });
      onDone(updated);
    } catch (e) {
      setError(messageOf(e));
    }
  }

  return (
    <Modal
      title={reactivating ? "Reactivar cliente" : "Desactivar cliente"}
      description={customer.displayName}
      onClose={onClose}
      footer={
        <>
          <button className="button button-secondary" onClick={onClose}>
            Cancelar
          </button>
          <button
            className={`button ${reactivating ? "button-primary" : "button-danger"}`}
            disabled={mutation.isPending}
            onClick={confirm}
          >
            {mutation.isPending
              ? "Guardando…"
              : reactivating
                ? "Reactivar"
                : "Desactivar"}
          </button>
        </>
      }
    >
      <p>
        {reactivating
          ? "El cliente volverá a estar disponible para nuevas ventas."
          : "Un cliente inactivo no puede recibir nuevas ventas ni acceder al portal. Sus ventas e historial no se modifican y puedes reactivarlo cuando quieras."}
      </p>
      {error && (
        <div className="form-alert" role="alert">
          {error}
        </div>
      )}
    </Modal>
  );
}

/** Permanently deletes a customer who has no sales. Admin only; the reason goes to the audit trail. */
export function DeleteDialog({
  customer,
  onClose,
  onDone,
  toast,
}: {
  customer: ApiCustomer;
  onClose: () => void;
  onDone: () => void;
  toast: (toast: Toast) => void;
}) {
  const mutation = useDeleteCustomer(customer.id);
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");

  async function confirm() {
    if (!reason.trim()) {
      setError("Escribe el motivo de la eliminación.");
      return;
    }
    setError("");
    try {
      await mutation.mutateAsync(reason.trim());
      toast({
        title: "Cliente eliminado",
        message: "El registro se eliminó de forma permanente.",
        tone: "success",
      });
      onDone();
    } catch (e) {
      setError(
        e instanceof ApiError && e.code === "CUSTOMER_HAS_SALES"
          ? "Este cliente tiene ventas y no puede eliminarse. Desactívalo en su lugar."
          : messageOf(e),
      );
    }
  }

  return (
    <Modal
      title="Eliminar cliente"
      description={customer.displayName}
      onClose={onClose}
      footer={
        <>
          <button className="button button-secondary" onClick={onClose}>
            Cancelar
          </button>
          <button
            className="button button-danger"
            disabled={mutation.isPending}
            onClick={confirm}
          >
            {mutation.isPending ? "Eliminando…" : "Eliminar definitivamente"}
          </button>
        </>
      }
    >
      <p>
        Esta acción es permanente y solo se permite si el cliente no tiene
        ventas. Queda registrada en la auditoría con el motivo que indiques.
      </p>
      <Field label="Motivo" error={error}>
        <textarea
          rows={3}
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          placeholder="Por ejemplo: registro duplicado creado por error"
        />
      </Field>
    </Modal>
  );
}
