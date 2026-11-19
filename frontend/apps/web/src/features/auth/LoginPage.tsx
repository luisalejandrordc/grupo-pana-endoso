import { useState } from "react";
import { Icon, logoLandscape } from "@endosoflow/ui";
import { ForgotPasswordForm } from "./ForgotPasswordForm";
import { LoginForm, type LoginInput } from "./LoginForm";

const steps = [
  { title: "Gestiona la venta", text: "Del primer contacto al cierre" },
  { title: "Coordina cada tarea", text: "Documentos, validaciones y pagos" },
  {
    title: "Entrega con confianza",
    text: "Un historial claro de principio a fin",
  },
];

export function LoginPage({
  onLogin,
  error,
}: {
  onLogin: (input: LoginInput) => Promise<void>;
  error: string;
}) {
  const [forgot, setForgot] = useState(false);

  return (
    <div className="login-layout">
      <section className="login-brand-panel" aria-hidden="true">
        <div className="login-visual">
          <div className="login-eyebrow">
            <span className="eyebrow-line" /> PLATAFORMA DE GESTIÓN COMERCIAL
          </div>
          <p className="login-headline">
            Todo el proceso de venta.
            <br />
            <em>En un solo lugar.</em>
          </p>
          <p className="login-lede">
            Conecta clientes, equipos y trámites en una experiencia de trabajo
            más clara, ordenada y trazable.
          </p>
          <ol className="login-flow">
            {steps.map((step, index) => (
              <li className="flow-step" key={step.title}>
                <span className="flow-number">0{index + 1}</span>
                <div>
                  <strong>{step.title}</strong>
                  <small>{step.text}</small>
                </div>
                <Icon name="arrow-up-right" size={17} />
              </li>
            ))}
          </ol>
        </div>
        <div className="login-brand-footer">
          <span>© 2026 Grupo Pana</span>
          <span>OPERACIÓN · TRAZABILIDAD · CONFIANZA</span>
        </div>
        <div className="login-decoration decoration-one" />
        <div className="login-decoration decoration-two" />
      </section>

      <section className="login-form-panel">
        <div className="login-form-card">
          <img className="login-logo" src={logoLandscape} alt="EndosoFlow" />
          <div className="login-form-heading">
            <p className="section-kicker">PORTAL DE COLABORADORES</p>
            <h1>
              {forgot ? "Recupera tu acceso" : "Qué bueno tenerte de vuelta"}
            </h1>
            <p className="login-form-lede">
              {forgot
                ? "Ingresa tu documento y te enviaremos las instrucciones si existe una cuenta asociada."
                : "Ingresa con tus credenciales corporativas para continuar."}
            </p>
          </div>
          {forgot ? (
            <ForgotPasswordForm onBack={() => setForgot(false)} />
          ) : (
            <LoginForm
              onLogin={onLogin}
              onForgot={() => setForgot(true)}
              error={error}
            />
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
