import { api } from "../../shared/api/client";
import { SetPasswordPage, type SetPasswordFlow } from "./SetPasswordPage";

const flow: SetPasswordFlow = {
  documentTitle: "Restablecer contraseña",
  title: "Crea una nueva contraseña",
  lede: "Elige una contraseña nueva para tu cuenta. Por seguridad, se cerrarán tus sesiones activas.",
  submitLabel: "Guardar contraseña",
  busyLabel: "Guardando…",
  done: {
    title: "Contraseña actualizada",
    lede: "Por seguridad se cerraron todas tus sesiones. Inicia sesión con tu nueva contraseña.",
  },
  invalid: {
    title: "Este enlace ya no es válido",
    lede: "El enlace para restablecer tu contraseña venció o ya se usó. Puedes pedir uno nuevo.",
    missingLede:
      "Falta el código del enlace. Abre el enlace completo que te llegó por correo, o pide uno nuevo.",
  },
  retry: { label: "Pedir un enlace nuevo", forgot: true },
  endsSessions: true,
  submit: (token, password) => api.resetPassword({ token, password }),
};

/** The page the password-reset email links to: `/reset-password?token=…`. */
export function ResetPasswordPage() {
  return <SetPasswordPage flow={flow} />;
}
