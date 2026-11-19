import { useState, type FormEvent } from "react";
import { ApiError } from "../../shared/api/client";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";
import { Modal } from "../../shared/ui/Modal";
import type { Toast } from "../../shared/ui/toast";
import type { Role } from "../../shared/types";
import { customersApi, type ApiCustomer } from "./api";
import { DeleteDialog, StatusDialog } from "./CustomerDialogs";
import {
  emptyForm,
  fieldErrorsFrom,
  formFromCustomer,
  toCreateBody,
  toUpdateBody,
  validateForm,
  withCustomerType,
  type CustomerForm,
  type FormErrors,
} from "./form";
import {
  canLookUp,
  customerTypeLabels,
  customerTypes,
  documentTypeLabels,
  documentTypesFor,
  isPerson,
  isValidDocumentNumber,
  normalizeDocumentNumber,
  type CustomerType,
} from "./labels";
import { customerKeys, useCreateCustomer, useUpdateCustomer } from "./queries";
import { useQueryClient } from "@tanstack/react-query";

export type FormMode =
  { kind: "create" } | { kind: "edit"; customer: ApiCustomer };

/** What the document check found, shown right under the document fields. */
type Check =
  | { state: "idle" }
  | { state: "checking" }
  | { state: "exists"; customer: ApiCustomer }
  | { state: "restricted"; name: string }
  | { state: "free"; message: string }
  | { state: "filled"; message: string }
  | { state: "failed"; message: string };

const lookupSource: Record<string, string> = { DNI: "RENIEC", RUC: "SUNAT" };

export function CustomerFormModal({
  mode,
  role,
  onClose,
  onSaved,
  onOpenExisting,
  toast,
}: {
  mode: FormMode;
  role: Role;
  onClose: () => void;
  /** Called after a create or an edit, with the saved customer. */
  onSaved: (customer: ApiCustomer, created: boolean) => void;
  onOpenExisting: (customer: ApiCustomer) => void;
  toast: (toast: Toast) => void;
}) {
  const editing = mode.kind === "edit";
  const [current, setCurrent] = useState<ApiCustomer | null>(
    mode.kind === "edit" ? mode.customer : null,
  );
  const [form, setForm] = useState<CustomerForm>(
    mode.kind === "edit" ? formFromCustomer(mode.customer) : emptyForm(),
  );
  const [errors, setErrors] = useState<FormErrors>({});
  const [banner, setBanner] = useState("");
  const [stale, setStale] = useState(false);
  const [check, setCheck] = useState<Check>({ state: "idle" });
  const [dialog, setDialog] = useState<"status" | "delete" | null>(null);

  const client = useQueryClient();
  const create = useCreateCustomer();
  const update = useUpdateCustomer(current?.id ?? "");
  const saving = create.isPending || update.isPending;

  const person = isPerson(form.customerType);
  const canManage = role === "system_admin" || role === "sales_manager";
  const lockedNames = !editing && form.nameSource === "looked_up";
  const changed = current ? toUpdateBody(current, form).changed : true;
  // A customer that already exists must not be created again.
  const duplicate =
    !editing && (check.state === "exists" || check.state === "restricted");

  function set<K extends keyof CustomerForm>(key: K, value: CustomerForm[K]) {
    setForm((old) => ({ ...old, [key]: value }));
    setErrors((old) => (old[key] ? { ...old, [key]: undefined } : old));
  }

  /** A different document makes any looked-up name wrong, so it is cleared and the check starts over. */
  function setIdentity(next: CustomerForm) {
    const identityChanged =
      next.customerType !== form.customerType ||
      next.documentType !== form.documentType ||
      normalizeDocumentNumber(next.documentNumber) !==
        normalizeDocumentNumber(form.documentNumber);
    if (identityChanged && next.nameSource === "looked_up") {
      next = {
        ...next,
        nameSource: "manual",
        firstName: "",
        lastName: "",
        legalName: "",
      };
    }
    if (identityChanged) setCheck({ state: "idle" });
    setForm(next);
    setErrors((old) => ({
      ...old,
      documentNumber: undefined,
      documentType: undefined,
    }));
  }

  async function runCheck() {
    const number = normalizeDocumentNumber(form.documentNumber);
    if (!isValidDocumentNumber(form.documentType, number)) {
      setErrors((old) => ({
        ...old,
        documentNumber:
          "Ingresa un número de documento válido antes de verificar.",
      }));
      return;
    }
    setCheck({ state: "checking" });
    try {
      const found = await customersApi.lookup(form.documentType, number);
      if (found.exists) {
        if (found.access === "full" && found.customer) {
          setCheck({ state: "exists", customer: found.customer });
        } else {
          setCheck({
            state: "restricted",
            name: found.brief?.displayName ?? "este cliente",
          });
        }
        return;
      }
    } catch (error) {
      setCheck({
        state: "failed",
        message:
          error instanceof Error
            ? error.message
            : "No se pudo verificar el documento.",
      });
      return;
    }
    if (!canLookUp(form.documentType)) {
      setCheck({
        state: "free",
        message:
          "No hay un cliente con este documento. Completa los datos manualmente.",
      });
      return;
    }
    try {
      const identity = await customersApi.lookupIdentity(
        form.documentType,
        number,
      );
      const source = lookupSource[form.documentType];
      setForm((old) => ({
        ...old,
        nameSource: "looked_up",
        firstName: identity.firstName ?? old.firstName,
        lastName: identity.lastName ?? old.lastName,
        legalName: identity.legalName ?? old.legalName,
      }));
      setErrors((old) => ({
        ...old,
        firstName: undefined,
        lastName: undefined,
        legalName: undefined,
      }));
      setCheck({
        state: "filled",
        message: `Nombre obtenido de ${source}. Completa el resto de los datos.`,
      });
    } catch (error) {
      // Any failure (not found, quota, rate limit, outage) leaves manual entry open.
      setCheck({
        state: "failed",
        message:
          (error instanceof Error
            ? error.message
            : "La consulta automática falló.") +
          " Puedes ingresar los datos manualmente.",
      });
    }
  }

  function unlockNames() {
    setForm((old) => ({ ...old, nameSource: "manual" }));
    setCheck({ state: "idle" });
  }

  async function reload() {
    if (!current) return;
    try {
      const fresh = await customersApi.get(current.id);
      setCurrent(fresh);
      setForm(formFromCustomer(fresh));
      setErrors({});
      setBanner("");
      setStale(false);
      client.invalidateQueries({ queryKey: customerKeys.all });
    } catch (error) {
      setBanner(
        error instanceof Error
          ? error.message
          : "No se pudo recargar la ficha.",
      );
    }
  }

  function showApiError(error: unknown) {
    setErrors(fieldErrorsFrom(error));
    if (error instanceof ApiError && error.code === "VERSION_CONFLICT") {
      setStale(true);
      setBanner("Otra persona modificó esta ficha mientras la editabas.");
      return;
    }
    setBanner(
      error instanceof Error
        ? error.message
        : "No se pudo guardar. Inténtalo nuevamente.",
    );
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBanner("");
    const problems = validateForm(form);
    if (Object.keys(problems).length > 0) {
      setErrors(problems);
      setBanner("Revisa los campos marcados.");
      return;
    }
    try {
      if (current) {
        const { body, changed: anyChange } = toUpdateBody(current, form);
        if (!anyChange) return;
        const saved = await update.mutateAsync(body);
        toast({
          title: "Cliente actualizado",
          message: saved.displayName,
          tone: "success",
        });
        onSaved(saved, false);
      } else {
        const saved = await create.mutateAsync(toCreateBody(form));
        toast({
          title: "Cliente creado",
          message: `${saved.displayName} ya está disponible para nuevas ventas.`,
          tone: "success",
        });
        onSaved(saved, true);
      }
    } catch (error) {
      showApiError(error);
    }
  }

  const title = editing ? "Ficha del cliente" : "Nuevo cliente";

  return (
    <>
      <Modal
        title={title}
        description={
          editing
            ? `${current?.displayName ?? ""} · ${current?.documentType} ${current?.documentNumber}`
            : "Verifica el documento primero: si el cliente ya existe, no se duplica."
        }
        onClose={onClose}
        footer={
          <>
            <button
              type="button"
              className="button button-secondary"
              onClick={onClose}
            >
              Cancelar
            </button>
            <button
              type="submit"
              form="customer-form"
              className="button button-primary"
              disabled={saving || (editing && !changed) || stale || duplicate}
            >
              {saving
                ? "Guardando…"
                : editing
                  ? "Guardar cambios"
                  : "Crear cliente"}
            </button>
          </>
        }
      >
        <form
          id="customer-form"
          className="form-section"
          onSubmit={submit}
          noValidate
        >
          {banner && (
            <div className="form-alert" role="alert">
              <Icon name="alert" size={15} />
              <span>{banner}</span>
              {stale && (
                <button
                  type="button"
                  className="button button-secondary button-sm"
                  onClick={reload}
                >
                  Cargar los cambios
                </button>
              )}
            </div>
          )}

          <p className="form-section-title">Identificación</p>
          <Field label="Tipo de cliente">
            <select
              value={form.customerType}
              onChange={(event) =>
                setIdentity(
                  withCustomerType(form, event.target.value as CustomerType),
                )
              }
            >
              {customerTypes.map((type) => (
                <option key={type} value={type}>
                  {customerTypeLabels[type]}
                </option>
              ))}
            </select>
          </Field>
          <div className="document-row">
            <Field label="Tipo de documento" error={errors.documentType}>
              <select
                value={form.documentType}
                onChange={(event) =>
                  setIdentity({
                    ...form,
                    documentType: event.target
                      .value as CustomerForm["documentType"],
                  })
                }
              >
                {documentTypesFor(form.customerType).map((type) => (
                  <option key={type} value={type}>
                    {documentTypeLabels[type]}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Número de documento" error={errors.documentNumber}>
              <input
                inputMode="text"
                autoComplete="off"
                value={form.documentNumber}
                onChange={(event) =>
                  setIdentity({ ...form, documentNumber: event.target.value })
                }
                placeholder={
                  form.documentType === "RUC" ? "11 dígitos" : "Número"
                }
              />
            </Field>
            {!editing && (
              <button
                type="button"
                className="button button-secondary"
                onClick={runCheck}
                disabled={check.state === "checking"}
              >
                <Icon name="search" size={15} />
                {check.state === "checking"
                  ? "Verificando…"
                  : canLookUp(form.documentType)
                    ? "Verificar y autocompletar"
                    : "Verificar documento"}
              </button>
            )}
          </div>

          <CheckResult check={check} onOpenExisting={onOpenExisting} />

          <p className="form-section-title">
            {person ? "Datos personales" : "Datos de la organización"}
          </p>
          {person ? (
            <>
              <div className="form-grid-two">
                <Field label="Nombres" error={errors.firstName}>
                  <input
                    value={form.firstName}
                    readOnly={lockedNames}
                    onChange={(event) => set("firstName", event.target.value)}
                  />
                </Field>
                <Field label="Apellidos" error={errors.lastName}>
                  <input
                    value={form.lastName}
                    readOnly={lockedNames}
                    onChange={(event) => set("lastName", event.target.value)}
                  />
                </Field>
              </div>
              <div className="form-grid-two">
                <Field label="Género" error={errors.gender}>
                  <select
                    value={form.gender}
                    onChange={(event) =>
                      set(
                        "gender",
                        event.target.value as CustomerForm["gender"],
                      )
                    }
                  >
                    <option value="">Selecciona</option>
                    <option value="F">Femenino</option>
                    <option value="M">Masculino</option>
                  </select>
                </Field>
                <Field label="Nacionalidad" error={errors.nationality}>
                  <input
                    value={form.nationality}
                    onChange={(event) => set("nationality", event.target.value)}
                  />
                </Field>
              </div>
            </>
          ) : (
            <>
              <Field label="Razón social" error={errors.legalName}>
                <input
                  value={form.legalName}
                  readOnly={lockedNames}
                  onChange={(event) => set("legalName", event.target.value)}
                />
              </Field>
              <Field label="País o nacionalidad" error={errors.nationality}>
                <input
                  value={form.nationality}
                  onChange={(event) => set("nationality", event.target.value)}
                />
              </Field>
            </>
          )}
          {lockedNames && (
            <small className="field-hint">
              Obtenido de {lookupSource[form.documentType]}.{" "}
              <button
                type="button"
                className="link-button"
                onClick={unlockNames}
              >
                Editar manualmente
              </button>
            </small>
          )}
          {editing && current?.nameSource === "looked_up" && (
            <small className="field-hint">
              El nombre vino de{" "}
              {lookupSource[current.documentType] ?? "la consulta automática"}.
              Si lo editas, quedará registrado como ingresado manualmente.
            </small>
          )}

          {!person && (
            <>
              <p className="form-section-title">Persona de contacto</p>
              <div className="form-grid-two">
                <Field label="Nombres" error={errors.contactFirstName}>
                  <input
                    value={form.contactFirstName}
                    onChange={(event) =>
                      set("contactFirstName", event.target.value)
                    }
                  />
                </Field>
                <Field label="Apellidos" error={errors.contactLastName}>
                  <input
                    value={form.contactLastName}
                    onChange={(event) =>
                      set("contactLastName", event.target.value)
                    }
                  />
                </Field>
              </div>
              <Field label="Cargo" error={errors.contactJobTitle}>
                <input
                  value={form.contactJobTitle}
                  onChange={(event) =>
                    set("contactJobTitle", event.target.value)
                  }
                />
              </Field>
            </>
          )}

          <p className="form-section-title">Contacto</p>
          <div className="form-grid-two">
            <Field label="Teléfono" error={errors.phone}>
              <input
                type="tel"
                value={form.phone}
                onChange={(event) => set("phone", event.target.value)}
              />
            </Field>
            <Field label="Correo electrónico" error={errors.email}>
              <input
                type="email"
                value={form.email}
                onChange={(event) => set("email", event.target.value)}
              />
            </Field>
          </div>
        </form>

        {editing && current && canManage && (
          <div className="admin-zone">
            <div className="admin-zone-row">
              <div>
                <strong>
                  {current.isActive
                    ? "Desactivar cliente"
                    : "Reactivar cliente"}
                </strong>
                {current.isActive
                  ? "Deja de recibir nuevas ventas y acceso al portal. El historial se conserva."
                  : "Vuelve a estar disponible para nuevas ventas."}
              </div>
              <button
                type="button"
                className={`button button-sm ${current.isActive ? "button-secondary" : "button-primary"}`}
                onClick={() => setDialog("status")}
              >
                {current.isActive ? "Desactivar" : "Reactivar"}
              </button>
            </div>
            {role === "system_admin" && (
              <div className="admin-zone-row">
                <div>
                  <strong>Eliminar cliente</strong>
                  Permanente. Solo si no tiene ventas.
                </div>
                <button
                  type="button"
                  className="button button-danger button-sm"
                  onClick={() => setDialog("delete")}
                >
                  Eliminar…
                </button>
              </div>
            )}
          </div>
        )}
      </Modal>

      {dialog === "status" && current && (
        <StatusDialog
          customer={current}
          toast={toast}
          onClose={() => setDialog(null)}
          onDone={(updated) => {
            setDialog(null);
            setCurrent(updated);
            setForm(formFromCustomer(updated));
            onSaved(updated, false);
          }}
        />
      )}
      {dialog === "delete" && current && (
        <DeleteDialog
          customer={current}
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

function CheckResult({
  check,
  onOpenExisting,
}: {
  check: Check;
  onOpenExisting: (customer: ApiCustomer) => void;
}) {
  switch (check.state) {
    case "exists":
      return (
        <div className="check-card check-warn" role="status">
          <Icon name="info" size={15} />
          <span>
            <strong>Este cliente ya está registrado:</strong>{" "}
            {check.customer.displayName}.
          </span>
          <button
            type="button"
            className="button button-secondary button-sm"
            onClick={() => onOpenExisting(check.customer)}
          >
            Abrir ficha
          </button>
        </div>
      );
    case "restricted":
      return (
        <div className="check-card check-warn" role="status">
          <Icon name="info" size={15} />
          <span>
            <strong>Este cliente ya está registrado como {check.name}.</strong>{" "}
            No forma parte de tu cartera: se vinculará a ti cuando crees una
            venta para él, así que no hace falta volver a crearlo.
          </span>
        </div>
      );
    case "free":
      return (
        <div className="check-card" role="status">
          <Icon name="check-circle" size={15} />
          <span>{check.message}</span>
        </div>
      );
    case "filled":
      return (
        <div className="check-card check-ok" role="status">
          <Icon name="check-circle" size={15} />
          <span>{check.message}</span>
        </div>
      );
    case "failed":
      return (
        <div className="check-card check-warn" role="status">
          <Icon name="alert" size={15} />
          <span>{check.message}</span>
        </div>
      );
    default:
      return null;
  }
}
