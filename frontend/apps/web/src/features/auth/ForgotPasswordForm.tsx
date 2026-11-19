import { useState, type FormEvent } from "react";
import { api } from "../../shared/api/client";
import type { DocumentType } from "../../shared/types";
import { Field, Icon } from "@endosoflow/ui";
import { documentTypeOptions } from "./documentTypes";

export function ForgotPasswordForm({ onBack }: { onBack: () => void }) {
  const [documentType, setDocumentType] = useState<DocumentType>("DNI");
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
      setError(
        caught instanceof Error
          ? caught.message
          : "No fue posible procesar la solicitud.",
      );
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
            setDocumentType(event.target.value as DocumentType)
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
          placeholder="Ingresa tu número"
          autoComplete="username"
        />
      </Field>
      {sent && (
        <div className="inline-success" role="status">
          <Icon name="check-circle" size={18} />
          <span>
            Si el identificador corresponde a una cuenta activa, recibirás un
            enlace en tu correo corporativo.
          </span>
        </div>
      )}
      {error && (
        <div className="inline-error" role="alert">
          <Icon name="alert" size={18} />
          <span>{error}</span>
        </div>
      )}
      <button className="button button-primary button-wide" disabled={busy}>
        {busy ? "Enviando solicitud…" : "Enviar instrucciones"}
        {!busy && <Icon name="arrow-right" size={17} />}
      </button>
      <button type="button" className="text-button centered" onClick={onBack}>
        Volver a iniciar sesión
      </button>
    </form>
  );
}
