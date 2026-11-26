import { ToastProvider, logoSquare } from "@endosoflow/ui";
import { Route, Routes } from "react-router-dom";
import { ActivateAccountPage } from "../features/auth/ActivateAccountPage";
import { AuthProvider } from "../features/auth/AuthProvider";
import { LoginPage } from "../features/auth/LoginPage";
import { ResetPasswordPage } from "../features/auth/ResetPasswordPage";
import { useAuth } from "../features/auth/useAuth";
import { ErrorBoundary } from "./ErrorBoundary";
import { PortalShell } from "./layout/PortalShell";
import { PortalRoutes } from "./routes";

/**
 * The portal's root: the providers every screen relies on, then either one of the two public pages (the links
 * in the emails, which never wait for a session) or the login gate with the portal behind it.
 */
export default function App() {
  return (
    <ToastProvider>
      <AuthProvider>
        <Routes>
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
    <PortalShell user={user} onSignOut={() => void signOut()}>
      <ErrorBoundary>
        <PortalRoutes />
      </ErrorBoundary>
    </PortalShell>
  );
}
