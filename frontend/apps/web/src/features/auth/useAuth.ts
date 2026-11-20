import { useContext } from "react";
import type { EmployeeProfile } from "../../shared/types";
import { AuthContext } from "./authContext";

export function useAuth() {
  const auth = useContext(AuthContext);
  if (!auth) throw new Error("useAuth must be used inside an AuthProvider.");
  return auth;
}

/** The signed-in employee, for the part of the app that only exists behind the login. */
export function useCurrentUser(): EmployeeProfile {
  const { user } = useAuth();
  if (!user) throw new Error("useCurrentUser was used without a session.");
  return user;
}
