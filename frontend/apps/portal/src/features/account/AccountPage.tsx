import { ApiError, errorText, fieldErrors } from "@endosoflow/api";
import {
  BackLink,
  Callout,
  Icon,
  InitialsTile,
  PageHeader,
  PasswordField,
  RuleList,
  useDocumentTitle,
  useToast,
} from "@endosoflow/ui";
import { useState, type FormEvent } from "react";
import { api } from "../../shared/api/client";
import { documentTypeOptions } from "../../shared/lib/documentTypes";
import {
  isPasswordAcceptable,
  passwordRules,
} from "../../shared/lib/passwordRules";
import { useAuth, useCurrentCustomer } from "../auth/useAuth";

function ChangePasswordForm() {
  const { notify } = useToast();
  const { signOut } = useAuth();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [banner, setBanner] = useState<string | null>(null);
  const [errors, setErrors] = useState<{ current?: string; password?: string }>(
    {},
  );

  const acceptable =
    current !== "" && isPasswordAcceptable(password, confirmation);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!acceptable || busy) return;
    setBusy(true);
    setBanner(null);
    setErrors({});
    try {
      await api.changePassword({
        currentPassword: current,
        newPassword: password,
      });
      // The server ended every session, this one included: this browser follows and asks to sign in again.
      await signOut();
      notify({
        tone: "success",
        title: "Contraseña actualizada",
        message: "Por seguridad, ingrese nuevamente con su nueva contraseña.",
      });
    } catch (error) {
      const fields = fieldErrors(error);
      if (
        error instanceof ApiError &&
        error.code === "INVALID_CURRENT_PASSWORD"
      )
        setErrors({ current: error.message });
      else {
        setErrors({
          current: fields.currentPassword,
          password: fields.newPassword,
        });
        setBanner(errorText(error));
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="form-stack" noValidate>
      <PasswordField
        label="Contraseña actual"
        value={current}
        onChange={setCurrent}
        autoComplete="current-password"
        error={errors.current}
      />
      <PasswordField
        label="Nueva contraseña"
        value={password}
        onChange={setPassword}
        autoComplete="new-password"
        error={errors.password}
      />
      <PasswordField
        label="Confirmar nueva contraseña"
        value={confirmation}
        onChange={setConfirmation}
        autoComplete="new-password"
      />
      <RuleList
        label="Requisitos de la contraseña"
        rules={passwordRules(password, confirmation)}
      />
      <Callout tone="info">
        Al cambiar su contraseña se cerrarán todas sus sesiones, también en
        otros dispositivos.
      </Callout>
      {banner && <Callout tone="error">{banner}</Callout>}
      <div>
        <button
          className="button button-primary"
          disabled={busy || !acceptable}
        >
          {busy ? "Guardando…" : "Cambiar contraseña"}
        </button>
      </div>
    </form>
  );
}

/** The customer's own data (read-only: staff keep it) and the one thing they can change, their password. */
export function AccountPage() {
  useDocumentTitle("Mi cuenta");
  const customer = useCurrentCustomer();
  const documentLabel =
    documentTypeOptions.find((option) => option.value === customer.documentType)
      ?.label ?? customer.documentType;

  return (
    <div className="stack account">
      <BackLink to="/">Mis compras</BackLink>
      <PageHeader
        title="Mi cuenta"
        description="Sus datos de registro y la seguridad de su acceso."
      />

      <section className="panel account-card" aria-labelledby="profile-title">
        <div className="account-identity">
          <InitialsTile name={customer.displayName} />
          <h2 id="profile-title">{customer.displayName}</h2>
        </div>
        <dl className="account-data">
          <div>
            <dt>{documentLabel}</dt>
            <dd>{customer.documentNumber}</dd>
          </div>
          <div>
            <dt>Correo electrónico</dt>
            <dd>{customer.email}</dd>
          </div>
          <div>
            <dt>Teléfono</dt>
            <dd>{customer.phone || "—"}</dd>
          </div>
        </dl>
        <p className="account-note">
          <Icon name="info" size={15} /> Para actualizar estos datos,
          comuníquese con su asesor de Grupo Pana.
        </p>
      </section>

      <section className="panel account-card" aria-labelledby="password-title">
        <h2 id="password-title">Cambiar contraseña</h2>
        <ChangePasswordForm />
      </section>
    </div>
  );
}
