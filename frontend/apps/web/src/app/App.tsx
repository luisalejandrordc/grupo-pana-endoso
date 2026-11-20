import { ActivateAccountPage } from "../features/auth/ActivateAccountPage";
import { AuthProvider } from "../features/auth/AuthProvider";
import { LoginPage } from "../features/auth/LoginPage";
import { ResetPasswordPage } from "../features/auth/ResetPasswordPage";
import { useAuth } from "../features/auth/useAuth";
import { Route, Routes } from "react-router-dom";
import { ToastProvider, logoSquare } from "@endosoflow/ui";
import { AppShell } from "./layout/AppShell";
import { AppRoutes } from "./routes";

/**
 * The application root: the providers every screen relies on, then either one of the few public pages (the links in
 * the emails) or the login gate with the app behind it.
 */
export default function App() {
  return (
    <ToastProvider>
      <AuthProvider>
        <Routes>
          {/* Public: they must open without waiting for, or depending on, a session. */}
          <Route path="/activate-account" element={<ActivateAccountPage />} />
          <Route path="/reset-password" element={<ResetPasswordPage />} />
          <Route path="*" element={<Gate />} />
        </Routes>
      </AuthProvider>
    </ToastProvider>
  );
}

function Gate() {
  const { user, authLoading, authError, signIn, signOut } = useAuth();

  if (authLoading)
    return (
      <div className="boot" role="status" aria-label="Cargando">
        <img src={logoSquare} alt="" width={72} height={72} />
      </div>
    );

  if (!user) return <LoginPage onLogin={signIn} error={authError} />;

  return (
    <AppShell user={user} onSignOut={signOut}>
      <AppRoutes />
    </AppShell>
  );
}
