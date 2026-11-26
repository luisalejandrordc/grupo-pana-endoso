import { api } from "../../shared/api/client";
import { SetPasswordPage, type SetPasswordFlow } from "./SetPasswordPage";

const flow: SetPasswordFlow = {
  documentTitle: "Restablecer contraseña",
  title: "Cree una nueva contraseña",
  lede: "Elija una contraseña nueva para su cuenta. Por seguridad, se cerrarán sus sesiones activas.",
  submitLabel: "Guardar contraseña",
  busyLabel: "Guardando…",
  done: {
    title: "Contraseña actualizada",
    lede: "Por seguridad se cerraron todas sus sesiones. Ingrese con su nueva contraseña.",
  },
  invalid: {
    title: "Este enlace ya no es válido",
    lede: "El enlace para restablecer su contraseña venció o ya se usó. Puede pedir uno nuevo.",
    missingLede:
      "Falta el código del enlace. Abra el enlace completo que le llegó por correo, o pida uno nuevo.",
  },
  retry: { label: "Pedir un enlace nuevo", forgot: true },
  endsSessions: true,
  submit: (token, password) => api.resetPassword({ token, password }),
};

/** The page the password-reset email links to: `/reset-password?token=…`. */
export function ResetPasswordPage() {
  return <SetPasswordPage flow={flow} />;
}
