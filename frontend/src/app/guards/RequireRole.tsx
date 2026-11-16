import type { ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { useCurrentUser } from "../../features/auth/useAuth";
import type { Role } from "../../shared/types";

/** Renders its children only for the listed roles; anyone else is sent to the sales list. */
export function RequireRole({
  roles,
  children,
}: {
  roles: Role[];
  children: ReactNode;
}) {
  const user = useCurrentUser();
  if (!roles.includes(user.role)) return <Navigate to="/sales" replace />;
  return <>{children}</>;
}
