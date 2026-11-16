import { createContext } from "react";
import type { Toast } from "./toast";

/** Shows a notification. Outside a ToastProvider it does nothing, so a component renders anywhere. */
export const ToastContext = createContext<(toast: Toast) => void>(() => {});
