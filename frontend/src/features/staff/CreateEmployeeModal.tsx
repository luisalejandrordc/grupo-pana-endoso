import { useState, type FormEvent } from "react";
import { errorText, fieldErrors } from "../../shared/api/errors";
import { roleLabel, roleOptions } from "../../shared/lib/roles";
import type { DocumentType, Role } from "../../shared/types";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";
import { Modal } from "../../shared/ui/Modal";
import type { Toast } from "../../shared/ui/toast";
import { documentTypeOptions } from "../auth/documentTypes";
import type { CreateEmployeeInput } from "./api";
import { useCreateEmployee } from "./queries";

type Form = {
  documentType: DocumentType;
  documentNumber: string;
  firstName: string;
  lastName: string;
  nationality: string;
  gender: "M" | "F";
  birthdate: string;
  hireDate: string;
  businessPhone: string;
  businessEmail: string;
  personalPhone: string;
  personalEmail: string;
  role: Role;
};

const empty: Form = {
  documentType: "DNI",
  documentNumber: "",
  firstName: "",
  lastName: "",
  nationality: "Peruana",
  gender: "F",
  birthdate: "",
  hireDate: "",
  businessPhone: "",
  businessEmail: "",
  personalPhone: "",
  personalEmail: "",
  role: "sales_advisor",
};

export function CreateEmployeeModal({
  onClose,
  toast,
}: {
  onClose: () => void;
  toast: (toast: Toast) => void;
}) {
  const [form, setForm] = useState<Form>(empty);
  const [error, setError] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const create = useCreateEmployee();

  const set =
    <K extends keyof Form>(key: K) =>
    (value: Form[K]) =>
      setForm((old) => ({ ...old, [key]: value }));
  const text = (key: keyof Form) => ({
    value: form[key],
    onChange: (event: { target: { value: string } }) =>
      setForm((old) => ({ ...old, [key]: event.target.value })),
  });

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError(null);
    setErrors({});
    const body: CreateEmployeeInput = {
      ...form,
      personalPhone: form.personalPhone || undefined,
      personalEmail: form.personalEmail || undefined,
    };
    try {
      const result = await create.mutateAsync(body);
      toast({
        title: "Colaborador creado",
        message: result.activationLinkIssued
          ? "Se emitió el enlace de activación."
          : "No se pudo emitir el enlace; puedes reenviarlo más adelante.",
        tone: "success",
      });
      onClose();
    } catch (caught) {
      setError(errorText(caught));
      setErrors(fieldErrors(caught));
    }
  }

  return (
    <Modal
      title="Nuevo colaborador"
      description="Se creará la cuenta y se enviará un enlace de activación a su correo corporativo."
      wide
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="button button-secondary"
            onClick={onClose}
            disabled={create.isPending}
          >
            Cancelar
          </button>
          <button
            form="employee-form"
            className="button button-primary"
            disabled={create.isPending}
          >
            {create.isPending ? "Creando…" : "Crear colaborador"}
          </button>
        </>
      }
    >
      <form id="employee-form" onSubmit={submit} className="form-stack">
        <div className="form-grid">
          <Field label="Tipo de documento">
            <select
              value={form.documentType}
              onChange={(event) =>
                set("documentType")(event.target.value as DocumentType)
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
          <Field label="Nombres" error={errors.firstName}>
            <input required {...text("firstName")} />
          </Field>
          <Field label="Apellidos" error={errors.lastName}>
            <input required {...text("lastName")} />
          </Field>
          <Field label="Nacionalidad" error={errors.nationality}>
            <input required {...text("nationality")} />
          </Field>
          <Field label="Género">
            <select
              value={form.gender}
              onChange={(event) =>
                set("gender")(event.target.value as "M" | "F")
              }
            >
              <option value="F">Femenino</option>
              <option value="M">Masculino</option>
            </select>
          </Field>
          <Field label="Fecha de nacimiento" error={errors.birthdate}>
            <input required type="date" {...text("birthdate")} />
          </Field>
          <Field label="Fecha de contratación" error={errors.hireDate}>
            <input required type="date" {...text("hireDate")} />
          </Field>
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
          <Field label="Rol asignado" className="span-2">
            <select
              value={form.role}
              onChange={(event) => set("role")(event.target.value as Role)}
            >
              {roleOptions.map((role) => (
                <option key={role} value={role}>
                  {roleLabel[role]}
                </option>
              ))}
            </select>
          </Field>
        </div>
        {error && (
          <div className="form-alert" role="alert">
            <Icon name="alert" size={17} />
            <span>{error}</span>
          </div>
        )}
      </form>
    </Modal>
  );
}
