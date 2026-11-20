import { ApiError } from "./client";

/** The message to show for any failure: the server's own Spanish text when there is one. */
export function errorText(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  if (error instanceof Error && error.message) return error.message;
  return "Ocurrió un error. Inténtalo nuevamente.";
}

/** Field errors from a refused request, by field name. */
export function fieldErrors(error: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  if (error instanceof ApiError && error.fields) {
    for (const f of error.fields) if (!out[f.field]) out[f.field] = f.message;
  }
  return out;
}
