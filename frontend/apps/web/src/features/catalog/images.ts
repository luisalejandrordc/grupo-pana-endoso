export const MAX_IMAGE_BYTES = 5 * 1024 * 1024;
export const ACCEPTED_IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"];

/** Checks a chosen file before it is uploaded. The server checks again from the file's real bytes. */
export function imageProblem(file: File): string | null {
  if (!ACCEPTED_IMAGE_TYPES.includes(file.type))
    return "La imagen debe ser JPEG, PNG o WebP.";
  if (file.size > MAX_IMAGE_BYTES) return "La imagen supera el máximo de 5 MB.";
  if (file.size === 0) return "El archivo está vacío.";
  return null;
}
