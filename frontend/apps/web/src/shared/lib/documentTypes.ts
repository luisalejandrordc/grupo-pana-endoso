import type { DocumentType } from "../../shared/types";

export const documentTypeOptions: { value: DocumentType; label: string }[] = [
  { value: "DNI", label: "DNI" },
  { value: "CE", label: "Carné de extranjería" },
  { value: "PAS", label: "Pasaporte" },
];
