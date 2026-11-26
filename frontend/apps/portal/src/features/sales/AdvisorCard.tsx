import { Icon, InitialsTile } from "@endosoflow/ui";
import { whatsappLink } from "../../shared/lib/format";
import type { PortalAdvisor } from "../../shared/types";

/** The business contact of the person in charge of the purchase: the way to ask anything. */
export function AdvisorCard({ advisor }: { advisor: PortalAdvisor }) {
  const chat = whatsappLink(advisor.phone);
  const role = advisor.gender === "F" ? "Su asesora" : "Su asesor";
  return (
    <section className="panel advisor-card" aria-labelledby="advisor-title">
      <div className="advisor-head">
        <InitialsTile name={advisor.name} />
        <div>
          <p className="advisor-role" id="advisor-title">
            {role} de Grupo Pana
          </p>
          <p className="advisor-name">{advisor.name}</p>
        </div>
      </div>
      <ul className="advisor-contacts">
        <li>
          <Icon name="mail" size={17} />
          <a href={`mailto:${advisor.email}`}>{advisor.email}</a>
        </li>
        <li>
          <Icon name="phone" size={17} />
          <a href={`tel:${advisor.phone.replace(/[^\d+]/g, "")}`}>
            {advisor.phone}
          </a>
        </li>
      </ul>
      {chat && (
        <a
          className="button button-secondary button-wide"
          href={chat}
          target="_blank"
          rel="noopener noreferrer"
        >
          <Icon name="message" size={16} /> Escribir por WhatsApp
        </a>
      )}
    </section>
  );
}
