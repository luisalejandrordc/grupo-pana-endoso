import { useCallback, useEffect, useState } from "react";
import { api } from "../../shared/api/client";
import { type DocumentType, type EmployeeProfile } from "../../shared/types";

/** The session itself: who is signed in, and the calls that change that. AuthProvider owns the one instance. */
export function useAuthState() {
  const [user, setUser] = useState<EmployeeProfile | null>(null);
  const [authLoading, setAuthLoading] = useState(true);
  const [authError, setAuthError] = useState("");

  useEffect(() => {
    api.setUnauthorizedHandler(() => setUser(null));
    let alive = true;
    api
      .refreshSession()
      .then((session) => {
        if (alive && session) setUser(session.employee);
      })
      .catch(() => undefined)
      .finally(() => {
        if (alive) setAuthLoading(false);
      });
    return () => {
      alive = false;
    };
  }, []);

  async function signIn(input: {
    documentType: DocumentType;
    documentNumber: string;
    password: string;
    rememberMe: boolean;
  }) {
    setAuthError("");
    try {
      const session = await api.login(input);
      setUser(session.employee);
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
    setUser(null);
  }

  /** Re-reads the signed-in employee, for after something about them changed (their picture). */
  const reload = useCallback(async () => {
    try {
      setUser(await api.me());
    } catch {
      /* The next request will say if the session is gone. */
    }
  }, []);

  return { user, authLoading, authError, signIn, signOut, reload };
}
