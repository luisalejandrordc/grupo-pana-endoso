import type { IconName } from "../shared/ui/Icon";
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
