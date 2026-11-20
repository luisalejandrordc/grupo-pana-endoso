import { useState } from "react";
import { useLocation } from "react-router-dom";
import { AuthLayout } from "./AuthLayout";
import { ForgotPasswordForm } from "./ForgotPasswordForm";
import { LoginForm, type LoginInput } from "./LoginForm";

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

  return (
    <AuthLayout
      title={forgot ? "Recupera tu acceso" : "Qué bueno tenerte de vuelta"}
      lede={
        forgot
          ? "Ingresa tu documento y te enviaremos las instrucciones si existe una cuenta asociada."
          : "Ingresa con tus credenciales corporativas para continuar."
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
