import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

/**
 * The text in the top search box, kept in the address as `?q=`. A filtered list can be shared, survives a
 * refresh and answers the browser's back button; moving to another page drops it, as a new page should.
 */
export function useSearchQuery(): [string, (value: string) => void] {
  const [params, setParams] = useSearchParams();
  const value = params.get("q") ?? "";
  const set = useCallback(
    (next: string) =>
      setParams(
        (old) => {
          const copy = new URLSearchParams(old);
          if (next) copy.set("q", next);
          else copy.delete("q");
          copy.delete("page");
          return copy;
        },
        { replace: true },
      ),
    [setParams],
  );
  return [value, set];
}
