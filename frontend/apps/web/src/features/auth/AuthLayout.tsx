import { AuthFormPanel, Icon } from "@endosoflow/ui";
import type { ReactNode } from "react";

const steps = [
  { title: "Gestiona la venta", text: "Del primer contacto al cierre" },
  { title: "Coordina cada tarea", text: "Documentos, validaciones y pagos" },
  {
    title: "Entrega con confianza",
    text: "Un historial claro de principio a fin",
  },
];

/**
 * The page every signed-out screen shares: the brand panel on one side and, on the other, a card with the logo,
 * a heading and whatever the screen needs (the login, the forgotten password, setting a password from a link).
 */
export function AuthLayout({
  title,
  lede,
  children,
}: {
  title: string;
  lede?: ReactNode;
  children: ReactNode;
}) {
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

      <AuthFormPanel
        kicker="PORTAL DE COLABORADORES"
        title={title}
        lede={lede}
        security="Acceso protegido para personal autorizado de Grupo Pana"
        helpLead="¿Necesitas ayuda?"
        help="Contacta al administrador de tu cuenta."
      >
        {children}
      </AuthFormPanel>
    </div>
  );
}
