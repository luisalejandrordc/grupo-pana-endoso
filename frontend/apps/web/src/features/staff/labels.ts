import { type BadgeTone } from "@endosoflow/ui";
import type { Role } from "../../shared/types";

/** The color of each role's badge: the administrator stands out, an advisor is the plain default. */
export const roleTone: Record<Role, BadgeTone> = {
  system_admin: "ink",
  sales_manager: "doing",
  sales_advisor: "neutral",
};
