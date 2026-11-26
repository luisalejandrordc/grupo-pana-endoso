import type { CustomerDocumentType } from "../types";

/** Customers sign in with any of the documents they were registered with (companies and embassies included). */
export const documentTypeOptions: {
  value: CustomerDocumentType;
  label: string;
}[] = [
  { value: "DNI", label: "DNI" },
  { value: "RUC", label: "RUC" },
  { value: "CE", label: "Carné de extranjería" },
  { value: "PAS", label: "Pasaporte" },
  { value: "CDI", label: "Carné diplomático (CDI)" },
];
