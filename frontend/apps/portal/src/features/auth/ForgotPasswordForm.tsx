import { errorText } from "@endosoflow/api";
import { Callout, Field } from "@endosoflow/ui";
import { Icon } from "@endosoflow/ui";
import { useState, type FormEvent } from "react";
import { api } from "../../shared/api/client";
import { documentTypeOptions } from "../../shared/lib/documentTypes";
import type { CustomerDocumentType } from "../../shared/types";

export function ForgotPasswordForm({ onBack }: { onBack: () => void }) {
  const [documentType, setDocumentType] = useState<CustomerDocumentType>("DNI");
  const [documentNumber, setDocumentNumber] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setSent(false);
    setError("");
    try {
      await api.forgotPassword({
        documentType,
        documentNumber: documentNumber.trim(),
      });
      setSent(true);
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="form-stack">
      <Field label="Tipo de documento">
        <select
          value={documentType}
          onChange={(event) =>
            setDocumentType(event.target.value as CustomerDocumentType)
          }
        >
          {documentTypeOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Número de documento">
        <input
          required
          autoFocus
          value={documentNumber}
          onChange={(event) => setDocumentNumber(event.target.value)}
          placeholder="Ingrese su número"
          autoComplete="username"
        />
      </Field>
      {sent && (
        <Callout tone="done" role="status">
          Si el documento corresponde a una cuenta activa, enviaremos un enlace
          al correo registrado. Revise también su carpeta de correo no deseado.
        </Callout>
      )}
      {error && <Callout tone="error">{error}</Callout>}
      <button className="button button-primary button-wide" disabled={busy}>
        {busy ? "Enviando…" : "Enviar instrucciones"}
        {!busy && <Icon name="arrow-right" size={17} />}
      </button>
      <button type="button" className="text-button centered" onClick={onBack}>
        Volver a ingresar
      </button>
    </form>
  );
}
