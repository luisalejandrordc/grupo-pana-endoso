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

/** Where the search box makes sense; elsewhere it is hidden. */
export const searchPlaceholders: Record<string, string> = {
  "/sales": "Buscar por cliente, código o vehículo",
  "/customers": "Buscar por nombre o documento",
  "/catalog": "Buscar modelo o versión",
};

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

/** The search belongs to lists, not to a sale or a form. */
export function searchPlaceholderFor(pathname: string): string | undefined {
  if (pathname.startsWith("/sales/")) return undefined;
  return searchPlaceholders["/" + pathname.split("/")[1]];
}
