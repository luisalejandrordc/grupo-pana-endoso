import { ApiError, errorText } from "@endosoflow/api";
import { ConfirmDialog, Field, Modal, type Toast } from "@endosoflow/ui";
import { useState } from "react";
import type { Employee } from "./api";
import {
  useDeactivateEmployee,
  useDeleteEmployee,
  useReactivateEmployee,
} from "./queries";

const fullName = (employee: Employee) =>
  `${employee.firstName} ${employee.lastName}`;

/** Confirms deactivating or reactivating an employee. */
export function StatusDialog({
  employee,
  onClose,
  onDone,
  toast,
}: {
  employee: Employee;
  onClose: () => void;
  /** The employee as the server now holds them, so the form can show the new state. */
  onDone: (employee: Employee) => void;
  toast: (toast: Toast) => void;
}) {
  const deactivate = useDeactivateEmployee();
  const reactivate = useReactivateEmployee();
  const reactivating = !employee.isActive;
  const pending = deactivate.isPending || reactivate.isPending;
  const [error, setError] = useState<string | null>(null);

  async function confirm() {
    setError(null);
    try {
      const updated = await (
        reactivating ? reactivate : deactivate
      ).mutateAsync(employee.id);
      toast({
        title: reactivating
          ? "Colaborador reactivado"
          : "Colaborador desactivado",
        message: reactivating
          ? `${fullName(employee)} puede volver a iniciar sesión.`
          : `${fullName(employee)} ya no puede iniciar sesión.`,
        tone: "success",
      });
      onDone(updated);
    } catch (caught) {
      setError(errorText(caught));
    }
  }

  return (
    <ConfirmDialog
      title={reactivating ? "Reactivar colaborador" : "Desactivar colaborador"}
      description={fullName(employee)}
      confirmLabel={reactivating ? "Reactivar" : "Desactivar"}
      busyLabel="Guardando…"
      tone={reactivating ? "default" : "danger"}
      busy={pending}
      error={error}
      onConfirm={confirm}
      onClose={onClose}
    >
      <p>
        {reactivating
          ? "El colaborador volverá a poder iniciar sesión."
          : "Un colaborador inactivo no puede iniciar sesión y se cierran sus sesiones activas. Su historial se conserva y puedes reactivarlo cuando quieras."}
      </p>
    </ConfirmDialog>
  );
}

/** Permanently deletes an employee who never signed in or acted. The reason goes to the audit trail. */
export function DeleteDialog({
  employee,
  onClose,
  onDone,
  toast,
}: {
  employee: Employee;
  onClose: () => void;
  onDone: () => void;
  toast: (toast: Toast) => void;
}) {
  const mutation = useDeleteEmployee();
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");

  async function confirm() {
    if (!reason.trim()) {
      setError("Escribe el motivo de la eliminación.");
      return;
    }
    setError("");
    try {
      await mutation.mutateAsync({ id: employee.id, reason: reason.trim() });
      toast({
        title: "Colaborador eliminado",
        message: "El registro se eliminó de forma permanente.",
        tone: "success",
      });
      onDone();
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? "Este colaborador ya tiene historial y no puede eliminarse. Desactívalo en su lugar."
          : errorText(caught),
      );
    }
  }

  return (
    <Modal
      title="Eliminar colaborador"
      description={fullName(employee)}
      onClose={mutation.isPending ? () => undefined : onClose}
      footer={
        <>
          <button
            className="button button-secondary"
            onClick={onClose}
            disabled={mutation.isPending}
          >
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
        Esta acción es permanente y solo se permite si el colaborador nunca
        inició sesión ni realizó acciones. Queda registrada en la auditoría con
        el motivo que indiques.
      </p>
      <Field label="Motivo" error={error}>
        <textarea
          rows={3}
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          placeholder="Por ejemplo: cuenta creada por error"
        />
      </Field>
    </Modal>
  );
}
