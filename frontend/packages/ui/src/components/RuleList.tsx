import { Icon } from "./Icon";

export interface Rule {
  label: string;
  met: boolean;
}

/** A live checklist of what a value still needs (a password's length, a match). Met and pending are said in words too. */
export function RuleList({
  rules,
  label,
  className = "",
}: {
  rules: Rule[];
  /** Names the list for assistive technology: "Requisitos de la contraseña". */
  label?: string;
  className?: string;
}) {
  return (
    <ul className={`rule-list ${className}`.trim()} aria-label={label}>
      {rules.map((rule) => (
        <li key={rule.label} className={rule.met ? "is-met" : ""}>
          <Icon name={rule.met ? "check-circle" : "circle"} size={14} />
          {rule.label}
          <span className="sr-only">
            {rule.met ? " (cumplido)" : " (pendiente)"}
          </span>
        </li>
      ))}
    </ul>
  );
}
