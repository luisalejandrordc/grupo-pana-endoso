import type { Schemas } from "@endosoflow/api";

/** Everything here derives from the contract; nothing is retyped by hand. */
export type CustomerProfile = Schemas["CustomerProfile"];
export type CustomerSession = Schemas["CustomerSessionResponse"];
export type CustomerDocumentType =
  Schemas["CustomerLoginRequest"]["documentType"];

export type PortalSaleSummary = Schemas["PortalSaleSummary"];
export type PortalSaleDetail = Schemas["PortalSaleDetail"];
export type PortalMilestone = Schemas["PortalMilestone"];
export type PortalSection = Schemas["PortalSection"];
export type PortalField = Schemas["PortalField"];
export type PortalDocument = Schemas["PortalDocument"];
export type PortalAdvisor = Schemas["PortalAdvisor"];
export type DocumentDownload = Schemas["DocumentDownload"];
export type SaleState = PortalSaleSummary["state"];
export type MilestoneState = PortalMilestone["state"];
export type PortalVehicle = Schemas["PortalVehicle"];
