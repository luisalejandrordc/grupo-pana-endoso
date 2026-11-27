import { useDocumentTitle } from "@endosoflow/ui";
import { useState } from "react";
import { useLocation } from "react-router-dom";
import { AuthLayout } from "./AuthLayout";
import { ForgotPasswordForm } from "./ForgotPasswordForm";
import { LoginForm } from "./LoginForm";
import type { LoginInput } from "./useAuthState";

export function LoginPage({
  onLogin,
  error,
}: {
  onLogin: (input: LoginInput) => Promise<void>;
  error: string;
}) {
  // A dead reset link sends people here already on the "forgot your password" step.
  const location = useLocation();
  const [forgot, setForgot] = useState(
    (location.state as { forgot?: boolean } | null)?.forgot === true,
  );
  useDocumentTitle(forgot ? "Recuperar acceso" : "Ingresar");

  return (
    <AuthLayout
      title={forgot ? "Recupere su acceso" : "Bienvenido a su portal"}
      lede={
        forgot
          ? "Ingrese su documento y le enviaremos las instrucciones si existe una cuenta activa asociada."
          : "Ingrese con las credenciales con las que fue registrado."
      }
    >
      {forgot ? (
        <ForgotPasswordForm onBack={() => setForgot(false)} />
      ) : (
        <LoginForm
          onLogin={onLogin}
          onForgot={() => setForgot(true)}
          error={error}
        />
      )}
    </AuthLayout>
  );
}
