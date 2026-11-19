import { ApiError } from "../../shared/api/client";
import type { CreateModelData } from "./api";
import type { VehicleCategory } from "./labels";

const pricePattern = /^[0-9]{1,10}(\.[0-9]{1,2})?$/;

/** A price as typed. Accepts "35000", "35000.5" and "35000.50"; rejects zero, signs, commas and exponents. */
export function priceProblem(raw: string): string | null {
  const text = raw.trim();
  if (!text) return "El precio de referencia es obligatorio.";
  if (!pricePattern.test(text))
    return "Usa solo números, con hasta dos decimales (por ejemplo 35000.50).";
  if (Number(text) <= 0) return "El precio debe ser mayor que cero.";
  return null;
}

/** Normalizes the typed price to two decimals, as the API returns it. */
export function normalizePrice(raw: string) {
  const [whole, fraction = ""] = raw.trim().split(".");
  return `${whole}.${fraction.padEnd(2, "0")}`;
}

export interface CreateModelForm {
  name: string;
  description: string;
  categories: VehicleCategory[];
  versionName: string;
  referencePrice: string;
  colorName: string;
}

export const emptyModelForm = (): CreateModelForm => ({
  name: "",
  description: "",
  categories: [],
  versionName: "",
  referencePrice: "",
  colorName: "",
});

export type CreateModelErrors = Partial<
  Record<keyof CreateModelForm | "image", string>
>;

export function validateCreateModel(
  form: CreateModelForm,
  hasImage: boolean,
): CreateModelErrors {
  const errors: CreateModelErrors = {};
  if (!form.name.trim()) errors.name = "El nombre del modelo es obligatorio.";
  if (form.categories.length === 0)
    errors.categories = "Elige al menos una categoría.";
  if (!form.versionName.trim())
    errors.versionName = "El nombre de la versión es obligatorio.";
  const price = priceProblem(form.referencePrice);
  if (price) errors.referencePrice = price;
  if (!form.colorName.trim())
    errors.colorName = "El nombre del color es obligatorio.";
  if (!hasImage)
    errors.image = "La imagen de la vista principal es obligatoria.";
  return errors;
}

export function toCreateModelData(form: CreateModelForm): CreateModelData {
  return {
    name: form.name.trim(),
    ...(form.description.trim()
      ? { description: form.description.trim() }
      : {}),
    categories: form.categories,
    version: {
      name: form.versionName.trim(),
      referencePrice: normalizePrice(form.referencePrice),
    },
    color: { name: form.colorName.trim() },
  };
}

/** Maps the API's field names ("version.name", "color.name", ...) onto the create form's keys. */
const createFieldMap: Record<string, keyof CreateModelForm | "image"> = {
  name: "name",
  description: "description",
  categories: "categories",
  "version.name": "versionName",
  "version.referencePrice": "referencePrice",
  "color.name": "colorName",
  image: "image",
};

export function createErrorsFrom(error: unknown): CreateModelErrors {
  const errors: CreateModelErrors = {};
  if (error instanceof ApiError && error.fields) {
    for (const item of error.fields) {
      const key = createFieldMap[item.field];
      if (key) errors[key] ??= item.message;
    }
  }
  return errors;
}

/** The field errors of a simple form whose field names equal the API's. */
export function simpleErrorsFrom<K extends string>(
  error: unknown,
  keys: readonly K[],
): Partial<Record<K, string>> {
  const errors: Partial<Record<K, string>> = {};
  if (error instanceof ApiError && error.fields) {
    for (const item of error.fields) {
      if ((keys as readonly string[]).includes(item.field))
        errors[item.field as K] ??= item.message;
    }
  }
  return errors;
}
