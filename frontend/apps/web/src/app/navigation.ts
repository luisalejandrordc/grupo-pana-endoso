import { type IconName } from "@endosoflow/ui";
import type { Role } from "../shared/types";

export interface NavItem {
  to: string;
  label: string;
  icon: IconName;
  /** Roles that see the item; everyone when omitted. */
  roles?: Role[];
}

export const navItems: NavItem[] = [
  { to: "/sales", label: "Ventas", icon: "sales" },
  { to: "/customers", label: "Clientes", icon: "users" },
  { to: "/catalog", label: "Catálogo", icon: "car" },
  {
    to: "/staff",
    label: "Colaboradores",
    icon: "shield",
    roles: ["system_admin"],
  },
];

const sectionTitles: Record<string, string> = {
  "/sales": "Ventas",
  "/customers": "Clientes",
  "/catalog": "Catálogo",
  "/staff": "Colaboradores",
  "/profile": "Mi perfil",
};

/** The name of the browser tab for an address. */
export function titleFor(pathname: string): string | undefined {
  if (pathname === "/sales/new") return "Nueva venta";
  return sectionTitles["/" + pathname.split("/")[1]];
}
