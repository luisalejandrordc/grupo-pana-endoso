import type { ReactNode } from "react";
import { AuthContext } from "./authContext";
import { useAuthState } from "./useAuthState";

export function AuthProvider({ children }: { children: ReactNode }) {
  const auth = useAuthState();
  return <AuthContext.Provider value={auth}>{children}</AuthContext.Provider>;
}
