import { api } from "../../shared/api/client";
import { SetPasswordPage, type SetPasswordFlow } from "./SetPasswordPage";

const flow: SetPasswordFlow = {
  documentTitle: "Activar cuenta",
  title: "Activa tu cuenta",
  lede: "Crea una contraseña para empezar a usar EndosoFlow.",
  submitLabel: "Activar cuenta",
  busyLabel: "Activando…",
  done: {
    title: "Tu cuenta está activa",
    lede: "Ya puedes iniciar sesión con tu documento y la contraseña que acabas de crear.",
  },
  invalid: {
    title: "Este enlace ya no es válido",
    lede: "El enlace de activación venció o ya se usó. Pide a un administrador que te envíe uno nuevo.",
    missingLede:
      "Falta el código del enlace. Abre el enlace completo que te llegó por correo, o pide a un administrador que te envíe uno nuevo.",
  },
  retry: { label: "Ir a iniciar sesión", forgot: false },
  endsSessions: false,
  submit: (token, password) => api.activateAccount({ token, password }),
};

/** The page the activation email links to: `/activate-account?token=…`. */
export function ActivateAccountPage() {
  return <SetPasswordPage flow={flow} />;
}
