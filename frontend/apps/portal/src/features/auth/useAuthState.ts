import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useState } from "react";
import { api } from "../../shared/api/client";
import type {
  CustomerDocumentType,
  CustomerProfile,
  CustomerSession,
} from "../../shared/types";

export interface LoginInput {
  documentType: CustomerDocumentType;
  documentNumber: string;
  password: string;
  rememberMe: boolean;
}

/** The session itself: who is signed in, and the calls that change that. AuthProvider owns the one instance. */
export function useAuthState() {
  const queryClient = useQueryClient();
  const [user, setUser] = useState<CustomerProfile | null>(null);
  const [authLoading, setAuthLoading] = useState(true);
  const [authError, setAuthError] = useState("");

  // What the cache holds belongs to one customer: it never outlives their session on a shared computer.
  const endSession = useCallback(() => {
    setUser(null);
    queryClient.clear();
  }, [queryClient]);

  useEffect(() => {
    api.setUnauthorizedHandler(endSession);
    let alive = true;
    api
      .refreshSession<CustomerSession>()
      .then((session) => {
        if (alive && session) setUser(session.customer);
      })
      .catch(() => undefined)
      .finally(() => {
        if (alive) setAuthLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [endSession]);

  async function signIn(input: LoginInput) {
    setAuthError("");
    try {
      const session = await api.login(input);
      setUser(session.customer);
    } catch (error) {
      setAuthError(
        error instanceof Error
          ? error.message
          : "No fue posible iniciar sesión.",
      );
      throw error;
    }
  }

  async function signOut() {
    try {
      await api.logout();
    } catch {
      /* A local sign out must still complete. */
    }
    api.setToken(null);
    endSession();
  }

  return { user, authLoading, authError, signIn, signOut };
}
