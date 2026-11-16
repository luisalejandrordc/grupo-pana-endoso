import { useContext } from "react";
import { ToastContext } from "./toastContext";

/** `const { notify } = useToast();` — notifications stack, close themselves and can be closed by hand. */
export function useToast() {
  return { notify: useContext(ToastContext) };
}
