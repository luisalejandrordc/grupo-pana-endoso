import { useEffect } from "react";

const SUFFIX = "EndosoFlow";

/** Puts the page's name in the browser tab, so history and tabs say where each one is. */
export function useDocumentTitle(title: string | undefined) {
  useEffect(() => {
    document.title = title ? `${title} · ${SUFFIX}` : `${SUFFIX} — Grupo Pana`;
  }, [title]);
}
