import { useCallback, useState } from "react";
import { lookupsApi, type IdentityResult } from "../api/lookups";

/** Where a looked-up name comes from, as people know it. */
export const lookupSource = { DNI: "RENIEC", RUC: "SUNAT" } as const;

/** What the last lookup did, shown right under the document. */
export type LookupState =
  | { status: "idle" }
  | { status: "checking" }
  | { status: "filled"; message: string }
  | { status: "failed"; message: string };

export const normalizeDocument = (value: string) => value.replace(/\s+/g, "");
export const isDni = (value: string) =>
  /^[0-9]{8}$/.test(normalizeDocument(value));
export const isRUC = (value: string) =>
  /^[0-9]{11}$/.test(normalizeDocument(value));

/**
 * Looks a DNI up (RENIEC, through Codart) to fill a name, for the forms that take one: the new employee and the
 * signer of task 2. The same rules as the customer form: the names come back to be filled and locked, any
 * failure (unknown document, rate limit, spent allowance, outage) only says so and leaves the names to be typed,
 * and the caller decides what a changed document does to a name that was looked up.
 */
export function useIdentityLookup() {
  const [state, setState] = useState<LookupState>({ status: "idle" });

  const reset = useCallback(() => setState({ status: "idle" }), []);

  /** Returns the names found, or null when there are none (the state says why). */
  const run = useCallback(
    async (documentNumber: string): Promise<IdentityResult | null> => {
      const number = normalizeDocument(documentNumber);
      if (!isDni(number)) {
        setState({
          status: "failed",
          message: "Ingresa un DNI de 8 dígitos para consultarlo.",
        });
        return null;
      }
      setState({ status: "checking" });
      try {
        const found = await lookupsApi.identity("DNI", number);
        setState({
          status: "filled",
          message: `Nombre obtenido de ${lookupSource.DNI}. Completa el resto de los datos.`,
        });
        return found;
      } catch (error) {
        setState({
          status: "failed",
          message: `${
            error instanceof Error
              ? error.message
              : "La consulta automática falló."
          } Puedes ingresar los datos manualmente.`,
        });
        return null;
      }
    },
    [],
  );

  return { state, run, reset };
}
