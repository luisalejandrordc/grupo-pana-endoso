import type { Schemas } from "../../shared/api/schema";

export type VehicleCategory = Schemas["VehicleCategory"];
export type Viewpoint = Schemas["Viewpoint"];

export const categoryLabels: Record<VehicleCategory, string> = {
  camioneta: "Camioneta",
  comercial: "Comercial",
  familiar: "Familiar",
  hatchback: "Hatchback",
  sedan: "Sedán",
};

export const categories = Object.keys(categoryLabels) as VehicleCategory[];

export const viewpointLabels: Record<Viewpoint, string> = {
  main_view: "Vista principal",
  exterior_front: "Frente",
  exterior_left: "Lateral izquierdo",
  exterior_right: "Lateral derecho",
  exterior_rear: "Posterior",
  interior: "Interior",
};

export const viewpoints = Object.keys(viewpointLabels) as Viewpoint[];

/**
 * Formats a price string from the API for display only ("35000.50" becomes "$35,000.50"). The API sends
 * decimal strings so money is never a float; the app never does arithmetic on these values.
 */
export function formatPrice(price: string) {
  const value = Number(price);
  return new Intl.NumberFormat("es-PE", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: Number.isInteger(value) ? 0 : 2,
    maximumFractionDigits: 2,
  }).format(value);
}
