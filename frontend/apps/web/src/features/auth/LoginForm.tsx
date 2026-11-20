import { useState, type FormEvent } from "react";
import type { DocumentType } from "../../shared/types";
import { Field, Icon, PasswordField } from "@endosoflow/ui";
import { documentTypeOptions } from "../../shared/lib/documentTypes";

export interface LoginInput {
  documentType: DocumentType;
  documentNumber: string;
  password: string;
  rememberMe: boolean;
}

export function LoginForm({
  onLogin,
  onForgot,
  error,
}: {
  onLogin: (input: LoginInput) => Promise<void>;
  onForgot: () => void;
  /** The server's answer to the last attempt. */
  error: string;
}) {
  const [documentType, setDocumentType] = useState<DocumentType>("DNI");
  const [documentNumber, setDocumentNumber] = useState("");
  const [password, setPassword] = useState("");
  const [rememberMe, setRememberMe] = useState(true);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await onLogin({
        documentType,
        documentNumber: documentNumber.trim(),
        password,
        rememberMe,
      });
    } catch {
      /* The message arrives through `error` and is shown below. */
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
          placeholder="Ej. 45871236"
          autoComplete="username"
          inputMode="text"
        />
      </Field>
      <PasswordField
        label="Contraseña"
        value={password}
        onChange={setPassword}
        placeholder="Ingresa tu contraseña"
        autoComplete="current-password"
      />
      <div className="login-form-options">
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={rememberMe}
            onChange={(event) => setRememberMe(event.target.checked)}
          />
          <span>Mantener sesión iniciada</span>
        </label>
        <button type="button" className="text-button" onClick={onForgot}>
          ¿Olvidaste tu contraseña?
        </button>
      </div>
      {error && (
        <div className="inline-error" role="alert">
          <Icon name="alert" size={18} />
          <span>{error}</span>
        </div>
      )}
      <button className="button button-primary button-wide" disabled={busy}>
        {busy ? "Verificando acceso…" : "Iniciar sesión"}
        {!busy && <Icon name="arrow-right" size={17} />}
      </button>
    </form>
  );
}
