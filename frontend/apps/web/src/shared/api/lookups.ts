import { queryString, type Schemas } from "@endosoflow/api";
import { api } from "./client";

export type IdentityResult = Schemas["IdentityResult"];

export const lookupsApi = {
  /**
   * Fills a name from a DNI or RUC (Codart, through our server). A customer we already have is answered from
   * our own database; otherwise the call counts against a per-minute limit and the month's allowance.
   */
  identity: (documentType: "DNI" | "RUC", documentNumber: string) =>
    api.request<IdentityResult>(
      `/lookups/identity${queryString({ documentType, documentNumber })}`,
    ),
};
