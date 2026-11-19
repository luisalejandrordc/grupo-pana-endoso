import type { Schemas } from "../../shared/api/schema";

export type CustomerType = Schemas["CustomerType"];
export type CustomerDocumentType = Schemas["CustomerDocumentType"];

export const customerTypeLabels: Record<CustomerType, string> = {
  diplomatico: "Diplomático",
  embajada: "Embajada",
  entidad_religiosa_catolica: "Entidad religiosa católica",
  entidad_religiosa_no_catolica: "Entidad religiosa no católica",
  organismo_internacional: "Organismo internacional",
};

export const customerTypes = Object.keys(customerTypeLabels) as CustomerType[];

export const portalStatusLabels = {
  not_invited: "Sin invitar",
  invited: "Invitado",
  active: "Activo",
} as const;

export const documentTypeLabels: Record<CustomerDocumentType, string> = {
  DNI: "DNI",
  CE: "Carné de extranjería",
  PAS: "Pasaporte",
  CDI: "Documento de identidad diplomático",
  RUC: "RUC",
};

/** A Diplomático is a person; every other type is an organization. */
export function isPerson(type: CustomerType | "") {
  return type === "diplomatico";
}

export function documentTypesFor(
  type: CustomerType | "",
): CustomerDocumentType[] {
  return isPerson(type) ? ["DNI", "CE", "PAS", "CDI"] : ["RUC"];
}

/** Codart can fill names only for these documents; the others are always typed by hand. */
export function canLookUp(docType: CustomerDocumentType) {
  return docType === "DNI" || docType === "RUC";
}

const documentPatterns: Record<CustomerDocumentType, RegExp> = {
  DNI: /^[0-9]{8}$/,
  RUC: /^[0-9]{11}$/,
  CE: /^[A-Z0-9]{9}$/,
  PAS: /^[A-Z0-9]{6,12}$/,
  CDI: /^[A-Z0-9]{8,12}$/,
};

export function normalizeDocumentNumber(value: string) {
  return value.replace(/\s+/g, "").toUpperCase();
}

export function isValidDocumentNumber(
  docType: CustomerDocumentType,
  value: string,
) {
  return documentPatterns[docType].test(normalizeDocumentNumber(value));
}
