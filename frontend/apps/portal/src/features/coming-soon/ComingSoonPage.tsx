import {
  Badge,
  Icon,
  logoLandscape,
  useDocumentTitle,
  type IconName,
} from "@endosoflow/ui";

const upcoming: { icon: IconName; text: string }[] = [
  { icon: "sales", text: "Consulta tus operaciones en un solo lugar" },
  { icon: "clock", text: "Sigue cada etapa de tu trámite" },
  { icon: "file-text", text: "Accede a tus documentos cuando los necesites" },
];

/** A holding page for the customer portal: says what is coming and who to ask in the meantime. */
export function ComingSoonPage() {
  useDocumentTitle("Portal de clientes");
  return (
    <div className="coming-soon">
      <main className="coming-soon-card">
        <img
          className="coming-soon-logo"
          src={logoLandscape}
          alt="EndosoFlow"
        />
        <Badge tone="brand">Próximamente</Badge>
        <h1>Estamos preparando tu portal</h1>
        <p className="coming-soon-lede">
          Muy pronto podrás iniciar sesión para ver tus operaciones y seguir su
          avance, paso a paso.
        </p>
        <ul className="coming-soon-list">
          {upcoming.map((item) => (
            <li key={item.text}>
              <span className="coming-soon-icon">
                <Icon name={item.icon} size={18} />
              </span>
              {item.text}
            </li>
          ))}
        </ul>
        <p className="coming-soon-help">
          ¿Necesitas algo mientras tanto? Contacta a tu asesor de Grupo Pana.
        </p>
      </main>
      <footer className="coming-soon-footer">© 2026 Grupo Pana</footer>
    </div>
  );
}
