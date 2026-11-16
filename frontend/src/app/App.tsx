import { AuthProvider } from "../features/auth/AuthProvider";
import { LoginPage } from "../features/auth/LoginPage";
import { useAuth } from "../features/auth/useAuth";
import { ToastProvider } from "../shared/ui/ToastProvider";
import { AppShell } from "./layout/AppShell";
import { AppRoutes } from "./routes";

/** The application root: the providers every screen relies on, then the login or the app behind it. */
export default function App() {
  return (
    <ToastProvider>
      <AuthProvider>
        <Gate />
      </AuthProvider>
    </ToastProvider>
  );
}

function Gate() {
  const { user, authLoading, authError, signIn, signOut } = useAuth();

  if (authLoading)
    return (
      <div className="boot" role="status" aria-label="Cargando">
        <img src="/endosoflow-logo-square.png" alt="" width={72} height={72} />
      </div>
    );

  if (!user) return <LoginPage onLogin={signIn} error={authError} />;

  return (
    <AppShell user={user} onSignOut={signOut}>
      <AppRoutes />
    </AppShell>
  );
}
