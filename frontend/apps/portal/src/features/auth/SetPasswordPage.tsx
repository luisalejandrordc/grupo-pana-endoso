import { ApiError, errorText, fieldErrors } from "@endosoflow/api";
import {
  Callout,
  Icon,
  PasswordField,
  RuleList,
  useDocumentTitle,
} from "@endosoflow/ui";
import { useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  isPasswordAcceptable,
  passwordRules,
} from "../../shared/lib/passwordRules";
import { AuthLayout } from "./AuthLayout";
import { useAuth } from "./useAuth";

/** What differs between "activate my account" and "reset my password": the words and the call. */
export interface SetPasswordFlow {
  documentTitle: string;
  title: string;
  lede: string;
  submitLabel: string;
  busyLabel: string;
  /** What to show once the password is set. */
  done: { title: string; lede: string };
  /** What to show when the link is missing, expired or already used. */
  invalid: { title: string; lede: string; missingLede: string };
  /** The way out of a dead link. */
  retry: { label: string; forgot: boolean };
  /** True when the server ends every session of the account on success (a reset does). */
  endsSessions: boolean;
  submit: (token: string, password: string) => Promise<void>;
}

type Phase = "form" | "done" | "invalid";

/**
 * Sets a password from the one-use link in an email. The token is read from the address once, kept in memory and
 * sent with the new password. The server says nothing about a token until it is used, so a dead link is only
 * discovered on submit (a 400), and the page then says so and shows the way out.
 */
export function SetPasswordPage({ flow }: { flow: SetPasswordFlow }) {
  useDocumentTitle(flow.documentTitle);
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [token] = useState(() => params.get("token")?.trim() ?? "");
  const { user, signOut } = useAuth();

  const [phase, setPhase] = useState<Phase>(token ? "form" : "invalid");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [banner, setBanner] = useState<string | null>(null);
  const [fieldError, setFieldError] = useState<string | undefined>();

  const acceptable = isPasswordAcceptable(password, confirmation);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!acceptable || busy) return;
    setBusy(true);
    setBanner(null);
    setFieldError(undefined);
    try {
      await flow.submit(token, password);
      // The used token must not stay in the address bar or the history.
      setParams({}, { replace: true });
      // The server has ended the account's sessions; if someone is signed in here, this browser follows.
      if (flow.endsSessions && user) await signOut().catch(() => undefined);
      setPassword("");
      setConfirmation("");
      setPhase("done");
    } catch (error) {
      if (error instanceof ApiError && error.status === 400)
        setPhase("invalid");
      else {
        setFieldError(fieldErrors(error).password);
        setBanner(errorText(error));
      }
    } finally {
      setBusy(false);
    }
  }

  if (phase === "done")
    return (
      <AuthLayout title={flow.done.title} lede={flow.done.lede}>
        <div role="status" className="form-stack">
          <Callout tone="done">Su contraseña quedó guardada.</Callout>
          <button
            autoFocus
            className="button button-primary button-wide"
            onClick={() => navigate("/", { replace: true })}
          >
            Ir a ingresar <Icon name="arrow-right" size={17} />
          </button>
        </div>
      </AuthLayout>
    );

  if (phase === "invalid")
    return (
      <AuthLayout
        title={flow.invalid.title}
        lede={token ? flow.invalid.lede : flow.invalid.missingLede}
      >
        <div className="form-stack">
          <button
            autoFocus
            className="button button-primary button-wide"
            onClick={() =>
              navigate("/", {
                replace: true,
                state: flow.retry.forgot ? { forgot: true } : undefined,
              })
            }
          >
            {flow.retry.label} <Icon name="arrow-right" size={17} />
          </button>
        </div>
      </AuthLayout>
    );

  return (
    <AuthLayout title={flow.title} lede={flow.lede}>
      <form onSubmit={submit} className="form-stack" noValidate>
        <PasswordField
          label="Nueva contraseña"
          value={password}
          onChange={setPassword}
          autoComplete="new-password"
          autoFocus
          error={fieldError}
        />
        <PasswordField
          label="Confirmar contraseña"
          value={confirmation}
          onChange={setConfirmation}
          autoComplete="new-password"
        />
        <RuleList
          label="Requisitos de la contraseña"
          rules={passwordRules(password, confirmation)}
        />
        {banner && <Callout tone="error">{banner}</Callout>}
        <button
          className="button button-primary button-wide"
          disabled={busy || !acceptable}
        >
          {busy ? flow.busyLabel : flow.submitLabel}
          {!busy && <Icon name="arrow-right" size={17} />}
        </button>
      </form>
    </AuthLayout>
  );
}
