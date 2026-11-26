import { createContext } from "react";
import type { useAuthState } from "./useAuthState";

export type AuthValue = ReturnType<typeof useAuthState>;

export const AuthContext = createContext<AuthValue | null>(null);
