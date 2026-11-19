import { ApiError } from "../../shared/api/client";
import type {
  ApiCustomer,
  CreateCustomerBody,
  UpdateCustomerBody,
} from "./api";
import {
  documentTypesFor,
  isPerson,
  isValidDocumentNumber,
  normalizeDocumentNumber,
  type CustomerDocumentType,
  type CustomerType,
} from "./labels";

/** What the form holds while the person types: everything is a string. */
export interface CustomerForm {
  customerType: CustomerType;
  documentType: CustomerDocumentType;
  documentNumber: string;
  firstName: string;
  lastName: string;
  gender: "M" | "F" | "";
  legalName: string;
  nameSource: "manual" | "looked_up";
  nationality: string;
  phone: string;
  email: string;
  contactFirstName: string;
  contactLastName: string;
  contactJobTitle: string;
}

export type FormErrors = Partial<Record<keyof CustomerForm, string>>;

export function emptyForm(): CustomerForm {
  return {
    customerType: "diplomatico",
    documentType: "DNI",
    documentNumber: "",
    firstName: "",
    lastName: "",
    gender: "",
    legalName: "",
    nameSource: "manual",
    nationality: "",
    phone: "",
    email: "",
    contactFirstName: "",
    contactLastName: "",
    contactJobTitle: "",
  };
}

export function formFromCustomer(c: ApiCustomer): CustomerForm {
  return {
    customerType: c.customerType,
    documentType: c.documentType,
    documentNumber: c.documentNumber,
    firstName: c.firstName ?? "",
    lastName: c.lastName ?? "",
    gender: c.gender ?? "",
    legalName: c.legalName ?? "",
    nameSource: c.nameSource,
    nationality: c.nationality,
    phone: c.phone,
    email: c.email,
    contactFirstName: c.contactFirstName ?? "",
    contactLastName: c.contactLastName ?? "",
    contactJobTitle: c.contactJobTitle ?? "",
  };
}

/** Changing the type switches between person and organization, so the document type must follow. */
export function withCustomerType(
  form: CustomerForm,
  type: CustomerType,
): CustomerForm {
  const allowed = documentTypesFor(type);
  return {
    ...form,
    customerType: type,
    documentType: allowed.includes(form.documentType)
      ? form.documentType
      : allowed[0],
  };
}

const personFields = ["firstName", "lastName", "gender"] as const;
const organizationFields = [
  "legalName",
  "contactFirstName",
  "contactLastName",
  "contactJobTitle",
] as const;
const sharedFields = [
  "documentNumber",
  "nationality",
  "phone",
  "email",
] as const;

function clean(form: CustomerForm): CustomerForm {
  const trimmed = { ...form };
  for (const key of Object.keys(trimmed) as (keyof CustomerForm)[]) {
    const value = trimmed[key];
    if (typeof value === "string") (trimmed[key] as string) = value.trim();
  }
  trimmed.documentNumber = normalizeDocumentNumber(trimmed.documentNumber);
  trimmed.email = trimmed.email.toLowerCase();
  return trimmed;
}

/** Fields that exist for the form's customer type. */
function shapeFields(type: CustomerType) {
  return isPerson(type) ? personFields : organizationFields;
}

export function toCreateBody(form: CustomerForm): CreateCustomerBody {
  const f = clean(form);
  const body: CreateCustomerBody = {
    customerType: f.customerType,
    documentType: f.documentType,
    documentNumber: f.documentNumber,
    nationality: f.nationality,
    phone: f.phone,
    email: f.email,
    nameSource: f.nameSource,
  };
  if (isPerson(f.customerType)) {
    body.firstName = f.firstName;
    body.lastName = f.lastName;
    body.gender = f.gender === "" ? undefined : f.gender;
  } else {
    body.legalName = f.legalName;
    body.contactFirstName = f.contactFirstName;
    body.contactLastName = f.contactLastName;
    body.contactJobTitle = f.contactJobTitle;
  }
  return body;
}

/**
 * The PATCH body: the version plus only what changed. Changing the customer type sends the whole new
 * shape, because the other shape's required fields have to come with it.
 */
export function toUpdateBody(
  original: ApiCustomer,
  form: CustomerForm,
): { body: UpdateCustomerBody; changed: boolean } {
  const before = clean(formFromCustomer(original));
  const after = clean(form);
  const body: Record<string, unknown> = { version: original.version };
  let changed = false;
  const typeChanged = before.customerType !== after.customerType;

  if (typeChanged) {
    body.customerType = after.customerType;
    changed = true;
  }
  if (typeChanged || before.documentType !== after.documentType) {
    if (before.documentType !== after.documentType || typeChanged) {
      body.documentType = after.documentType;
      changed = changed || before.documentType !== after.documentType;
    }
  }
  const fields = [...sharedFields, ...shapeFields(after.customerType)];
  for (const key of fields) {
    if (typeChanged || before[key] !== after[key]) {
      body[key] = after[key];
      if (before[key] !== after[key]) changed = true;
    }
  }
  return { body: body as UpdateCustomerBody, changed };
}

const phonePattern = /^[0-9+()\-\s]{6,20}$/;
const emailPattern = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

/** Quick checks before sending. The server stays the authority and its answers replace these. */
export function validateForm(form: CustomerForm): FormErrors {
  const f = clean(form);
  const errors: FormErrors = {};
  const required = (key: keyof CustomerForm, message: string) => {
    if (!f[key]) errors[key] = message;
  };

  if (!f.documentNumber)
    errors.documentNumber = "El número de documento es obligatorio.";
  else if (!isValidDocumentNumber(f.documentType, f.documentNumber))
    errors.documentNumber = "El número de documento no es válido para su tipo.";

  if (isPerson(f.customerType)) {
    required("firstName", "El nombre es obligatorio.");
    required("lastName", "El apellido es obligatorio.");
    required("gender", "Selecciona el género.");
  } else {
    required("legalName", "La razón social es obligatoria.");
    required("contactFirstName", "El nombre del contacto es obligatorio.");
    required("contactLastName", "El apellido del contacto es obligatorio.");
    required("contactJobTitle", "El cargo del contacto es obligatorio.");
  }
  required("nationality", "La nacionalidad es obligatoria.");
  if (!f.phone) errors.phone = "El teléfono es obligatorio.";
  else if (!phonePattern.test(f.phone))
    errors.phone = "El teléfono no es válido.";
  if (!f.email) errors.email = "El correo es obligatorio.";
  else if (!emailPattern.test(f.email))
    errors.email = "El correo no es válido.";
  return errors;
}

/** Maps the API's per-field problems onto the form's fields. */
export function fieldErrorsFrom(error: unknown): FormErrors {
  const errors: FormErrors = {};
  if (error instanceof ApiError && error.fields) {
    for (const item of error.fields) {
      if (!(item.field in emptyForm())) continue;
      const key = item.field as keyof CustomerForm;
      errors[key] ??= item.message;
    }
  }
  return errors;
}
