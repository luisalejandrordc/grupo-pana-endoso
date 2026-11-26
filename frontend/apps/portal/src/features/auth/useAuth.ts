import { useContext } from "react";
import type { CustomerProfile } from "../../shared/types";
import { AuthContext } from "./authContext";

export function useAuth() {
  const auth = useContext(AuthContext);
  if (!auth) throw new Error("useAuth must be used inside an AuthProvider.");
  return auth;
}

/** The signed-in customer, for the part of the portal that only exists behind the login. */
export function useCurrentCustomer(): CustomerProfile {
  const { user } = useAuth();
  if (!user) throw new Error("useCurrentCustomer was used without a session.");
  return user;
}
