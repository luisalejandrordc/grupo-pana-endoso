import type { ReactNode } from "react";
import { logoLandscape } from "../brand";
import { Icon } from "./Icon";

/**
 * The form side of every signed-out screen of both sites (login, forgotten password, setting a password from
 * a link): the logo, a kicker, the heading, whatever the screen needs, a line about security and a line about
 * where to get help. Each site draws its own brand panel next to it, inside `<div className="login-layout">`.
 */
export function AuthFormPanel({
  kicker,
  title,
  lede,
  security,
  helpLead,
  help,
  children,
}: {
  /** The audience, in capitals ("PORTAL DE COLABORADORES"). */
  kicker: string;
  title: string;
  lede?: ReactNode;
  security: string;
  /** The bold start of the help line ("¿Necesita ayuda?") and the rest of it. */
  helpLead: string;
  help: string;
  children: ReactNode;
}) {
  return (
    <main className="login-form-panel">
      <div className="login-form-card">
        <img className="login-logo" src={logoLandscape} alt="EndosoFlow" />
        <div className="login-form-heading">
          <p className="section-kicker">{kicker}</p>
          <h1>{title}</h1>
          {lede && <p className="login-form-lede">{lede}</p>}
        </div>
        {children}
        <div className="login-security">
          <Icon name="shield-check" size={17} />
          <span>{security}</span>
        </div>
      </div>
      <div className="login-help">
        <span>{helpLead}</span> {help}
      </div>
    </main>
  );
}
