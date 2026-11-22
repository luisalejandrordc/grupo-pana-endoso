import { ApiError, errorText, fieldErrors } from "@endosoflow/api";
import { Badge, Callout, Field, Modal } from "@endosoflow/ui";
import { useState, type FormEvent } from "react";
import { documentTypeOptions } from "../../shared/lib/documentTypes";
import { roleLabel, roleOptions } from "../../shared/lib/roles";
import type { DocumentType, Role } from "../../shared/types";
import type { Toast } from "@endosoflow/ui";
import type { Employee } from "./api";
import {
  emptyEmployeeForm,
  formFromEmployee,
  toCreateBody,
  toUpdateBody,
  type EmployeeForm,
} from "./employeeForm";
import { DeleteDialog, StatusDialog } from "./EmployeeDialogs";
import {
  useCreateEmployee,
  useFetchEmployee,
  useResendActivation,
  useUpdateEmployee,
} from "./queries";

export type EmployeeFormMode =
  { kind: "create" } | { kind: "edit"; employee: Employee };

const FORM_ID = "employee-form";

/**
 * Creates an employee, or edits one. An edit sends only what changed and the version it started from; if
 * someone else saved in the meantime the form says so and offers to load the current data.
 */
export function EmployeeFormModal({
  mode,
  currentUserId,
  toast,
  onClose,
  onSaved,
}: {
  mode: EmployeeFormMode;
  /** The signed-in administrator: they cannot change their own role from here. */
  currentUserId: string;
  toast: (toast: Toast) => void;
  onClose: () => void;
  onSaved?: (employee: Employee, created: boolean) => void;
}) {
  const [current, setCurrent] = useState<Employee | null>(
    mode.kind === "edit" ? mode.employee : null,
  );
  const [form, setForm] = useState<EmployeeForm>(
    mode.kind === "edit" ? formFromEmployee(mode.employee) : emptyEmployeeForm,
  );
  const [banner, setBanner] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [stale, setStale] = useState(false);
  const [dialog, setDialog] = useState<"status" | "delete" | null>(null);

  const create = useCreateEmployee();
  const update = useUpdateEmployee();
  const fetchLatest = useFetchEmployee();
  const resend = useResendActivation();

  const editing = current !== null;
  const isSelf = current?.id === currentUserId;
  /** An active account that has not set its password: its activation link can be sent again. */
  const pendingActivation =
    !!current && current.isActive && !current.activatedAt;
  const dirty = current ? toUpdateBody(current, form) !== null : true;
  const busy = create.isPending || update.isPending;

  const text = (key: keyof EmployeeForm) => ({
    value: form[key],
    onChange: (event: { target: { value: string } }) =>
      setForm((old) => ({ ...old, [key]: event.target.value })),
  });

  async function resendActivation() {
    if (!current) return;
    try {
      await resend.mutateAsync(current.id);
      toast({
        title: "Enlace de activación enviado",
        message: `Se envió a ${current.businessEmail}.`,
        tone: "success",
      });
    } catch (error) {
      toast({
        title: "No se pudo reenviar la activación",
        message: errorText(error),
        tone: "error",
      });
    }
  }

  async function loadLatest() {
    if (!current) return;
    try {
      const fresh = await fetchLatest.mutateAsync(current.id);
      setCurrent(fresh);
      setForm(formFromEmployee(fresh));
      setErrors({});
      setBanner(null);
      setStale(false);
    } catch (error) {
      setBanner(errorText(error));
    }
  }

  function showError(error: unknown) {
    setErrors(fieldErrors(error));
    if (error instanceof ApiError && error.code === "VERSION_CONFLICT") {
      setStale(true);
      setBanner(
        "Otra persona modificó a este colaborador mientras lo editabas.",
      );
      return;
    }
    setBanner(errorText(error));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBanner(null);
    setErrors({});
    try {
      if (current) {
        const body = toUpdateBody(current, form);
        if (!body) return;
        const saved = await update.mutateAsync({ id: current.id, body });
        toast({
          title: "Colaborador actualizado",
          message: `${saved.firstName} ${saved.lastName}`,
          tone: "success",
        });
        onSaved?.(saved, false);
      } else {
        const result = await create.mutateAsync(toCreateBody(form));
        toast({
          title: "Colaborador creado",
          message: result.activationLinkIssued
            ? "Se emitió el enlace de activación."
            : "No se pudo emitir el enlace; puedes reenviarlo desde la lista.",
          tone: "success",
        });
        onSaved?.(result.employee, true);
      }
      onClose();
    } catch (error) {
      showError(error);
    }
  }

  return (
    <>
      <Modal
        title={editing ? "Ficha del colaborador" : "Nuevo colaborador"}
        description={
          current
            ? `${current.firstName} ${current.lastName} · ${current.documentType} ${current.documentNumber}`
            : "Se creará la cuenta y se enviará un enlace de activación a su correo corporativo."
        }
        onClose={onClose}
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
              form={FORM_ID}
              className="button button-primary"
              disabled={busy || stale || !dirty}
            >
              {busy
                ? editing
                  ? "Guardando…"
                  : "Creando…"
                : editing
                  ? "Guardar cambios"
                  : "Crear colaborador"}
            </button>
          </>
        }
      >
        <form id={FORM_ID} onSubmit={submit} className="form-stack">
          {current && (
            <div className="cluster">
              <Badge tone={current.isActive ? "done" : "neutral"}>
                {current.isActive ? "Activo" : "Inactivo"}
              </Badge>
              <Badge plain tone={current.activatedAt ? "done" : "warn"}>
                {current.activatedAt
                  ? "Cuenta activada"
                  : "Activación pendiente"}
              </Badge>
            </div>
          )}
          {current && !current.isActive && (
            <Callout tone="quiet">
              Este colaborador está inactivo y no puede iniciar sesión. Se
              reactiva desde la lista.
            </Callout>
          )}

          <div className="form-section">
            <p className="form-section-title">Identificación</p>
            <div className="form-grid-two">
              <Field label="Tipo de documento">
                <select
                  value={form.documentType}
                  onChange={(event) =>
                    setForm((old) => ({
                      ...old,
                      documentType: event.target.value as DocumentType,
                    }))
                  }
                >
                  {documentTypeOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Número de documento" error={errors.documentNumber}>
                <input required {...text("documentNumber")} />
              </Field>
            </div>
          </div>

          <div className="form-section">
            <p className="form-section-title">Datos personales</p>
            <div className="form-grid-two">
              <Field label="Nombres" error={errors.firstName}>
                <input required {...text("firstName")} />
              </Field>
              <Field label="Apellidos" error={errors.lastName}>
                <input required {...text("lastName")} />
              </Field>
              <Field label="Fecha de nacimiento" error={errors.birthdate}>
                <input required type="date" {...text("birthdate")} />
              </Field>
              <Field label="Género">
                <select
                  value={form.gender}
                  onChange={(event) =>
                    setForm((old) => ({
                      ...old,
                      gender: event.target.value as "M" | "F",
                    }))
                  }
                >
                  <option value="F">Femenino</option>
                  <option value="M">Masculino</option>
                </select>
              </Field>
              <Field
                label="Nacionalidad"
                className="span-2"
                error={errors.nationality}
              >
                <input required {...text("nationality")} />
              </Field>
            </div>
          </div>

          <div className="form-section">
            <p className="form-section-title">Contacto</p>
            <div className="form-grid-two">
              <Field label="Teléfono corporativo" error={errors.businessPhone}>
                <input required type="tel" {...text("businessPhone")} />
              </Field>
              <Field label="Correo corporativo" error={errors.businessEmail}>
                <input required type="email" {...text("businessEmail")} />
              </Field>
              <Field
                label="Teléfono personal (opcional)"
                error={errors.personalPhone}
              >
                <input type="tel" {...text("personalPhone")} />
              </Field>
              <Field
                label="Correo personal (opcional)"
                error={errors.personalEmail}
              >
                <input type="email" {...text("personalEmail")} />
              </Field>
            </div>
          </div>

          <div className="form-section">
            <p className="form-section-title">Puesto</p>
            <div className="form-grid-two">
              <Field label="Fecha de contratación" error={errors.hireDate}>
                <input required type="date" {...text("hireDate")} />
              </Field>
              <Field
                label="Rol asignado"
                error={errors.role}
                hint={isSelf ? "No puedes cambiar tu propio rol." : undefined}
              >
                <select
                  value={form.role}
                  disabled={isSelf}
                  onChange={(event) =>
                    setForm((old) => ({
                      ...old,
                      role: event.target.value as Role,
                    }))
                  }
                >
                  {roleOptions.map((role) => (
                    <option key={role} value={role}>
                      {roleLabel[role]}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
          </div>

          {banner && (
            <Callout tone="error">
              {banner}
              {stale && (
                <>
                  {" "}
                  <button
                    type="button"
                    className="link-button"
                    onClick={loadLatest}
                    disabled={fetchLatest.isPending}
                  >
                    {fetchLatest.isPending
                      ? "Cargando…"
                      : "Cargar datos actuales"}
                  </button>
                </>
              )}
            </Callout>
          )}
        </form>

        {current && (pendingActivation || !isSelf) && (
          <div className="admin-zone">
            {pendingActivation && (
              <div className="admin-zone-row">
                <div>
                  <strong>Reenviar activación</strong>
                  Envía un enlace nuevo al correo corporativo para que cree su
                  contraseña.
                </div>
                <button
                  type="button"
                  className="button button-secondary button-sm"
                  onClick={resendActivation}
                  disabled={resend.isPending}
                >
                  {resend.isPending ? "Enviando…" : "Reenviar enlace"}
                </button>
              </div>
            )}
            {!isSelf && (
              <>
                <div className="admin-zone-row">
                  <div>
                    <strong>
                      {current.isActive
                        ? "Desactivar colaborador"
                        : "Reactivar colaborador"}
                    </strong>
                    {current.isActive
                      ? "No podrá iniciar sesión y se cerrarán sus sesiones. El historial se conserva."
                      : "Vuelve a poder iniciar sesión."}
                  </div>
                  <button
                    type="button"
                    className={`button button-sm ${current.isActive ? "button-secondary" : "button-primary"}`}
                    onClick={() => setDialog("status")}
                  >
                    {current.isActive ? "Desactivar" : "Reactivar"}
                  </button>
                </div>
                <div className="admin-zone-row">
                  <div>
                    <strong>Eliminar colaborador</strong>
                    Permanente. Solo si nunca inició sesión ni realizó acciones.
                  </div>
                  <button
                    type="button"
                    className="button button-danger button-sm"
                    onClick={() => setDialog("delete")}
                  >
                    Eliminar…
                  </button>
                </div>
              </>
            )}
          </div>
        )}
      </Modal>

      {dialog === "status" && current && (
        <StatusDialog
          employee={current}
          toast={toast}
          onClose={() => setDialog(null)}
          onDone={(updated) => {
            setDialog(null);
            // Only the server's copy changes; what was typed in the form stays, and saves against the new version.
            setCurrent(updated);
          }}
        />
      )}
      {dialog === "delete" && current && (
        <DeleteDialog
          employee={current}
          toast={toast}
          onClose={() => setDialog(null)}
          onDone={() => {
            setDialog(null);
            onClose();
          }}
        />
      )}
    </>
  );
}
