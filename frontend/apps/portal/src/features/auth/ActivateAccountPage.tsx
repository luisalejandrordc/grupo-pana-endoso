import { api } from "../../shared/api/client";
import { SetPasswordPage, type SetPasswordFlow } from "./SetPasswordPage";

const flow: SetPasswordFlow = {
  documentTitle: "Activar cuenta",
  title: "Active su cuenta",
  lede: "Cree una contraseña para ingresar a su portal y seguir el avance de su compra.",
  submitLabel: "Activar mi cuenta",
  busyLabel: "Activando…",
  done: {
    title: "Su cuenta está activa",
    lede: "Ya puede ingresar con su documento y la contraseña que acaba de crear.",
  },
  invalid: {
    title: "Este enlace ya no es válido",
    lede: "El enlace de activación venció o ya se usó. Si aún no puede ingresar, pida a su asesor de Grupo Pana que le envíe uno nuevo.",
    missingLede:
      "Falta el código del enlace. Abra el enlace completo que le llegó por correo, o pida a su asesor de Grupo Pana que le envíe uno nuevo.",
  },
  retry: { label: "Ir a ingresar", forgot: false },
  endsSessions: false,
  submit: async (token, password) => {
    await api.activateAccount({ token, password });
  },
};

/** The page the activation email links to: `/activate-account?token=…`. */
export function ActivateAccountPage() {
  return <SetPasswordPage flow={flow} />;
}
