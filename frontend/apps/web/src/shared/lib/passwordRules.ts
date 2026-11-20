import type { Rule } from "@endosoflow/ui";

/** The shortest password the product asks for. The server has the last word; this is the early, friendly check. */
export const MIN_PASSWORD_LENGTH = 10;

/** What a new password still needs, for the live checklist next to the form. */
export function passwordRules(password: string, confirmation: string): Rule[] {
  return [
    {
      label: `Al menos ${MIN_PASSWORD_LENGTH} caracteres`,
      met: password.length >= MIN_PASSWORD_LENGTH,
    },
    {
      label: "Las contraseñas coinciden",
      met: password !== "" && password === confirmation,
    },
  ];
}

export function isPasswordAcceptable(
  password: string,
  confirmation: string,
): boolean {
  return passwordRules(password, confirmation).every((rule) => rule.met);
}
