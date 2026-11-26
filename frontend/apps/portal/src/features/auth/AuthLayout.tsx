import { Icon, logoLandscape, type IconName } from "@endosoflow/ui";
import type { ReactNode } from "react";

const promises: { icon: IconName; title: string; text: string }[] = [
  {
    icon: "clock",
    title: "Siga cada etapa",
    text: "Sepa en qué punto está su compra, sin llamadas ni esperas.",
  },
  {
    icon: "file-text",
    title: "Sus documentos a mano",
    text: "Descargue su cotización, facturas y demás documentos cuando los necesite.",
  },
  {
    icon: "user",
    title: "Su asesor, siempre cerca",
    text: "Tenga a la mano los datos de contacto de quien lo acompaña.",
  },
];

/**
 * The page every signed-out screen shares: the brand panel on one side and, on the other, a card with the logo,
 * a heading and whatever the screen needs (the sign-in, the forgotten password, setting a password from a link).
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
    <div className="auth-layout">
      <section className="auth-brand" aria-hidden="true">
        <div className="auth-brand-body">
          <div className="auth-eyebrow">
            <span className="auth-eyebrow-line" /> PORTAL DE CLIENTES
          </div>
          <p className="auth-headline">
            Su compra,
            <br />
            <em>paso a paso.</em>
          </p>
          <p className="auth-brand-lede">
            Siga el avance del trámite de su vehículo con Grupo Pana, con total
            claridad y desde donde esté.
          </p>
          <ul className="auth-promises">
            {promises.map((item) => (
              <li key={item.title}>
                <span className="auth-promise-icon">
                  <Icon name={item.icon} size={18} />
                </span>
                <div>
                  <strong>{item.title}</strong>
                  <small>{item.text}</small>
                </div>
              </li>
            ))}
          </ul>
        </div>
        <div className="auth-brand-footer">© 2026 Grupo Pana</div>
        <div className="auth-ring auth-ring-one" />
        <div className="auth-ring auth-ring-two" />
      </section>

      <main className="auth-panel">
        <div className="auth-card">
          <img className="auth-logo" src={logoLandscape} alt="EndosoFlow" />
          <div className="auth-heading">
            <p className="auth-kicker">ACCESO DE CLIENTES</p>
            <h1>{title}</h1>
            {lede && <p className="auth-lede">{lede}</p>}
          </div>
          {children}
          <div className="auth-security">
            <Icon name="shield-check" size={17} />
            <span>Su información está protegida y solo usted puede verla</span>
          </div>
        </div>
        <p className="auth-help">
          ¿Necesita ayuda? Comuníquese con su asesor de Grupo Pana.
        </p>
      </main>
    </div>
  );
}
