import type { DocumentType, Role } from "../../shared/types";
import type { CreateEmployeeInput, Employee, UpdateEmployeeInput } from "./api";

/** What the form holds: text for every field, so an input is never "undefined". */
export interface EmployeeForm {
  documentType: DocumentType;
  documentNumber: string;
  firstName: string;
  lastName: string;
  /** Whether the name was filled from a lookup (and is locked) or typed. Sent when the employee is created. */
  nameSource: "manual" | "looked_up";
  nationality: string;
  gender: "M" | "F";
  birthdate: string;
  hireDate: string;
  businessPhone: string;
  businessEmail: string;
  personalPhone: string;
  personalEmail: string;
  role: Role;
}

export const emptyEmployeeForm: EmployeeForm = {
  documentType: "DNI",
  documentNumber: "",
  firstName: "",
  lastName: "",
  nameSource: "manual",
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

export function formFromEmployee(employee: Employee): EmployeeForm {
  return {
    documentType: employee.documentType,
    documentNumber: employee.documentNumber,
    firstName: employee.firstName,
    lastName: employee.lastName,
    nameSource: employee.nameSource,
    nationality: employee.nationality,
    gender: employee.gender,
    birthdate: employee.birthdate,
    hireDate: employee.hireDate,
    businessPhone: employee.businessPhone,
    businessEmail: employee.businessEmail,
    personalPhone: employee.personalPhone ?? "",
    personalEmail: employee.personalEmail ?? "",
    role: employee.role,
  };
}

const trimmed = (form: EmployeeForm): EmployeeForm => ({
  ...form,
  documentNumber: form.documentNumber.trim(),
  firstName: form.firstName.trim(),
  lastName: form.lastName.trim(),
  nationality: form.nationality.trim(),
  businessPhone: form.businessPhone.trim(),
  businessEmail: form.businessEmail.trim(),
  personalPhone: form.personalPhone.trim(),
  personalEmail: form.personalEmail.trim(),
});

/** The body of a new employee. The two personal contacts are optional: empty means "not given". */
export function toCreateBody(form: EmployeeForm): CreateEmployeeInput {
  const clean = trimmed(form);
  return {
    ...clean,
    personalPhone: clean.personalPhone || undefined,
    personalEmail: clean.personalEmail || undefined,
  };
}

const plainFields = [
  "documentType",
  "documentNumber",
  "firstName",
  "lastName",
  "nationality",
  "gender",
  "birthdate",
  "hireDate",
  "businessPhone",
  "businessEmail",
  "role",
] as const;

/**
 * The PATCH for an edit: `version` plus only the fields that differ from what the server holds, or null when
 * nothing changed. Clearing a personal contact sends an empty string, which is how the API clears it.
 * (The name's source is left out on purpose: the backend marks an edited name as manual by itself.)
 */
export function toUpdateBody(
  employee: Employee,
  form: EmployeeForm,
): UpdateEmployeeInput | null {
  const clean = trimmed(form);
  const body: UpdateEmployeeInput = { version: employee.version };
  let changed = false;

  for (const key of plainFields) {
    if (clean[key] !== employee[key]) {
      (body as Record<string, unknown>)[key] = clean[key];
      changed = true;
    }
  }
  for (const key of ["personalPhone", "personalEmail"] as const) {
    if (clean[key] !== (employee[key] ?? "")) {
      body[key] = clean[key];
      changed = true;
    }
  }
  return changed ? body : null;
}
