import type { ReactNode } from "react";

export type BadgeTone =
  "neutral" | "done" | "doing" | "late" | "warn" | "brand" | "ink";

/** A small status marker: a colored dot and a word, never color alone. */
export function Badge({
  tone = "neutral",
  plain = false,
  children,
}: {
  tone?: BadgeTone;
  /** Without the leading dot, for labels that are not a status (a role, a category). */
  plain?: boolean;
  children: ReactNode;
}) {
  return (
    <span className={`badge badge-${tone}${plain ? " badge-plain" : ""}`}>
      {children}
    </span>
  );
}
