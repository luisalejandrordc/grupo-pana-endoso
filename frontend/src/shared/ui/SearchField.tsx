import { useEffect, useRef } from "react";
import { Icon } from "./Icon";

function isTyping(target: EventTarget | null) {
  const node = target as HTMLElement | null;
  return (
    !!node &&
    (node.isContentEditable ||
      ["INPUT", "TEXTAREA", "SELECT"].includes(node.tagName))
  );
}

/** A search box with a clear button. With `shortcut`, pressing "/" anywhere on the page focuses it. */
export function SearchField({
  value,
  onChange,
  placeholder,
  label = "Buscar",
  shortcut = false,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  label?: string;
  shortcut?: boolean;
}) {
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!shortcut) return;
    function onKey(event: KeyboardEvent) {
      if (
        event.key !== "/" ||
        event.metaKey ||
        event.ctrlKey ||
        event.altKey ||
        isTyping(event.target)
      )
        return;
      event.preventDefault();
      input.current?.focus();
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [shortcut]);

  return (
    <label className="search">
      <Icon name="search" size={17} />
      <input
        ref={input}
        type="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape" && value) {
            event.stopPropagation();
            onChange("");
          }
        }}
        placeholder={placeholder}
        aria-label={label}
      />
      {value ? (
        <button
          type="button"
          className="icon-button"
          aria-label="Borrar búsqueda"
          onClick={() => {
            onChange("");
            input.current?.focus();
          }}
        >
          <Icon name="x" size={15} />
        </button>
      ) : (
        shortcut && <kbd aria-hidden="true">/</kbd>
      )}
    </label>
  );
}
