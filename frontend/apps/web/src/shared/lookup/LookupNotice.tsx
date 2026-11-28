import { Icon } from "@endosoflow/ui";
import type { LookupState } from "./useIdentityLookup";

/** The result of a lookup, as the small card the customer form already shows under its document. */
export function LookupNotice({ state }: { state: LookupState }) {
  if (state.status === "filled")
    return (
      <div className="check-card check-ok" role="status">
        <Icon name="check-circle" size={15} />
        <span>{state.message}</span>
      </div>
    );
  if (state.status === "failed")
    return (
      <div className="check-card check-warn" role="status">
        <Icon name="alert" size={15} />
        <span>{state.message}</span>
      </div>
    );
  return null;
}

/** "Obtenido de RENIEC. Editar manualmente": the names are locked until the person chooses to type them. */
export function LockedNameHint({
  source,
  onUnlock,
}: {
  source: string;
  /** Absent when the form cannot be edited (the link would do nothing). */
  onUnlock?: () => void;
}) {
  return (
    <small className="field-hint">
      Obtenido de {source}.{" "}
      {onUnlock && (
        <button type="button" className="link-button" onClick={onUnlock}>
          Editar manualmente
        </button>
      )}
    </small>
  );
}
