import { useState, type FormEvent } from "react";
import { api } from "../../shared/api/client";
import { type DocumentType } from "../../shared/types";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";

export function LoginPage({
  onLogin,
  busy,
  error,
}: {
  onLogin: (input: {
    documentType: DocumentType;
    documentNumber: string;
    password: string;
    rememberMe: boolean;
  }) => Promise<void>;
  busy: boolean;
  error: string;
}) {
  const [documentType, setDocumentType] = useState<DocumentType>("DNI");
  const [documentNumber, setDocumentNumber] = useState("");
  const [password, setPassword] = useState("");
  const [rememberMe, setRememberMe] = useState(true);
  const [showPassword, setShowPassword] = useState(false);
  const [forgot, setForgot] = useState(false);
  const [notice, setNotice] = useState("");
  const [forgotBusy, setForgotBusy] = useState(false);
  const [localError, setLocalError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setLocalError("");
    try {
      await onLogin({
        documentType,
        documentNumber: documentNumber.trim(),
        password,
        rememberMe,
      });
    } catch {
      /* The shared error message is rendered below the form. */
    }
  }

  async function requestReset(event: FormEvent) {
    event.preventDefault();
    setForgotBusy(true);
    setNotice("");
    setLocalError("");
    try {
      await api.forgotPassword({
        documentType,
        documentNumber: documentNumber.trim(),
      });
      setNotice(
        "Si el identificador corresponde a una cuenta activa, recibirás un enlace en tu correo corporativo.",
      );
    } catch (err) {
      setLocalError(
        err instanceof Error
          ? err.message
          : "No fue posible procesar la solicitud.",
      );
    } finally {
      setForgotBusy(false);
    }
  }

  return (
    <div className="login-layout">
      <section className="login-brand-panel">
        {/* <div className="login-brand-top"> */}
        {/*   <Brand /> */}
        {/* </div> */}
        <div className="login-visual">
          <div className="login-eyebrow">
            <span className="eyebrow-line" /> PLATAFORMA DE GESTIÓN COMERCIAL
          </div>
          <h1>
            Todo el proceso de venta.
            <br />
            <em>En un solo lugar.</em>
          </h1>
          <p>
            Conecta clientes, equipos y trámites en una experiencia de trabajo
            más clara, ordenada y trazable.
          </p>
          <div className="login-flow">
            <div className="flow-step">
              <span className="flow-number">01</span>
              <div>
                <strong>Gestiona la venta</strong>
                <small>Del primer contacto al cierre</small>
              </div>
              <Icon name="arrow-up-right" size={17} />
            </div>
            <div className="flow-step">
              <span className="flow-number">02</span>
              <div>
                <strong>Coordina cada tarea</strong>
                <small>Documentos, validaciones y pagos</small>
              </div>
              <Icon name="arrow-up-right" size={17} />
            </div>
            <div className="flow-step">
              <span className="flow-number">03</span>
              <div>
                <strong>Entrega con confianza</strong>
                <small>Un historial claro de principio a fin</small>
              </div>
              <Icon name="arrow-up-right" size={17} />
            </div>
          </div>
        </div>
        <div className="login-brand-footer">
          <span>© 2026 Grupo Pana</span>
          <span>OPERACIÓN · TRAZABILIDAD · CONFIANZA</span>
        </div>
        <div className="login-decoration decoration-one" />
        <div className="login-decoration decoration-two" />
      </section>
      <section className="login-form-panel">
        {/* <div className="login-mobile-brand"> */}
        {/*   <Brand /> */}
        {/* </div> */}
        <div className="login-form-card">
          <div className="login-form-heading">
            <div className="login-brand-top">
              <img
                src="endosoflow-logo-landscape.png"
                className="logo-landscape"
                alt="EndosoFlow Logo"
              />
            </div>
            <span className="section-kicker">PORTAL DE COLABORADORES</span>
            <h2>
              {forgot ? "Recupera tu acceso" : "Qué bueno tenerte de vuelta"}
            </h2>
            <p>
              {forgot
                ? "Ingresa tu documento y te enviaremos las instrucciones si existe una cuenta asociada."
                : "Ingresa con tus credenciales corporativas para continuar."}
            </p>
          </div>
          {forgot ? (
            <form onSubmit={requestReset} className="form-stack">
              <Field label="Tipo de documento">
                <select
                  value={documentType}
                  onChange={(event) =>
                    setDocumentType(event.target.value as DocumentType)
                  }
                >
                  <option>DNI</option>
                  <option>CE</option>
                  <option>PAS</option>
                </select>
              </Field>
              <Field label="Número de documento">
                <input
                  required
                  value={documentNumber}
                  onChange={(event) => setDocumentNumber(event.target.value)}
                  placeholder="Ingresa tu número"
                  autoComplete="username"
                />
              </Field>
              {notice && (
                <div className="inline-success">
                  <Icon name="check-circle" size={18} />
                  <span>{notice}</span>
                </div>
              )}
              {(localError || error) && (
                <div className="inline-error">
                  <Icon name="alert" size={18} />
                  <span>{localError || error}</span>
                </div>
              )}
              <button
                className="button button-primary button-wide"
                disabled={forgotBusy}
              >
                {forgotBusy ? "Enviando solicitud…" : "Enviar instrucciones"}{" "}
                <Icon name="arrow-right" size={17} />
              </button>
              <button
                type="button"
                className="text-button centered"
                onClick={() => {
                  setForgot(false);
                  setNotice("");
                  setLocalError("");
                }}
              >
                Volver a iniciar sesión
              </button>
            </form>
          ) : (
            <form onSubmit={submit} className="form-stack">
              <Field label="Tipo de documento">
                <select
                  value={documentType}
                  onChange={(event) =>
                    setDocumentType(event.target.value as DocumentType)
                  }
                >
                  <option value="DNI">DNI</option>
                  <option value="CE">Carné de extranjería</option>
                  <option value="PAS">Pasaporte</option>
                </select>
              </Field>
              <Field label="Número de documento">
                <input
                  required
                  value={documentNumber}
                  onChange={(event) => setDocumentNumber(event.target.value)}
                  placeholder="Ej. 45871236"
                  autoComplete="username"
                />
              </Field>
              <Field label="Contraseña">
                <div className="password-input">
                  <input
                    required
                    type={showPassword ? "text" : "password"}
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    placeholder="Ingresa tu contraseña"
                    autoComplete="current-password"
                  />
                  <button
                    type="button"
                    aria-label={
                      showPassword ? "Ocultar contraseña" : "Mostrar contraseña"
                    }
                    onClick={() => setShowPassword(!showPassword)}
                  >
                    <Icon name="eye" size={18} />
                  </button>
                </div>
              </Field>
              <div className="login-form-options">
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={rememberMe}
                    onChange={(event) => setRememberMe(event.target.checked)}
                  />
                  <span>Mantener sesión iniciada</span>
                </label>
                <button
                  type="button"
                  className="text-button"
                  onClick={() => {
                    setForgot(true);
                    setNotice("");
                    setLocalError("");
                  }}
                >
                  ¿Olvidaste tu contraseña?
                </button>
              </div>
              {(error || localError) && (
                <div className="inline-error">
                  <Icon name="alert" size={18} />
                  <span>{error || localError}</span>
                </div>
              )}
              <button
                className="button button-primary button-wide"
                disabled={busy}
              >
                {busy ? "Verificando acceso…" : "Iniciar sesión"}{" "}
                <Icon name="arrow-right" size={17} />
              </button>
            </form>
          )}
          <div className="login-security">
            <Icon name="shield-check" size={17} />
            <span>Acceso protegido para personal autorizado de Grupo Pana</span>
          </div>
        </div>
        <div className="login-help">
          <span>¿Necesitas ayuda?</span> Contacta al administrador de tu cuenta.
        </div>
      </section>
    </div>
  );
}
