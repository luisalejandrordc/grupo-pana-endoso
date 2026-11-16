import type { ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

const defaultIcon: Record<string, IconName> = {
  error: "alert",
  warn: "info",
  done: "circle-check",
  quiet: "lock",
  info: "info",
};

/** A short message set inside a page: an error, a warning, a confirmation or a note. */
export function Callout({
  tone = "info",
  icon,
  role,
  children,
}: {
  tone?: "info" | "error" | "warn" | "done" | "quiet";
  icon?: IconName;
  role?: "alert" | "status";
  children: ReactNode;
}) {
  return (
    <div
      className={`callout callout-${tone}`}
      role={role ?? (tone === "error" ? "alert" : undefined)}
    >
      <Icon name={icon ?? defaultIcon[tone]} size={17} />
      <div>{children}</div>
    </div>
  );
}
