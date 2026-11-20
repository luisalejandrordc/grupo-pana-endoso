import { useId, useState } from "react";
import { Icon } from "./Icon";

/**
 * A password input with a show/hide button, wired for screen readers like Field: the label names the input and a
 * hint or error is its description. The button is named after the field ("Mostrar nueva contraseña"), so a page
 * with several password fields has several distinct buttons.
 */
export function PasswordField({
  label,
  value,
  onChange,
  hint,
  error,
  autoComplete,
  placeholder,
  autoFocus,
  required = true,
  className = "",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  hint?: string;
  error?: string;
  /** "current-password" to sign in, "new-password" when choosing one (so a password manager offers to generate it). */
  autoComplete: "current-password" | "new-password";
  placeholder?: string;
  autoFocus?: boolean;
  required?: boolean;
  className?: string;
}) {
  const id = useId();
  const messageId = `${id}-message`;
  const [shown, setShown] = useState(false);
  const message = error ?? hint;
  return (
    <div className={`field ${className} ${error ? "has-error" : ""}`.trim()}>
      <label className="field-label" htmlFor={id}>
        {label}
      </label>
      <div className="password-input">
        <input
          id={id}
          type={shown ? "text" : "password"}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          required={required}
          autoFocus={autoFocus}
          autoComplete={autoComplete}
          placeholder={placeholder}
          aria-invalid={error ? true : undefined}
          aria-describedby={message ? messageId : undefined}
        />
        <button
          type="button"
          className="icon-button"
          aria-label={`${shown ? "Ocultar" : "Mostrar"} ${label.toLowerCase()}`}
          aria-pressed={shown}
          onClick={() => setShown((value) => !value)}
        >
          <Icon name={shown ? "eye-off" : "eye"} size={18} />
        </button>
      </div>
      {error ? (
        <small className="field-error" id={messageId} role="alert">
          {error}
        </small>
      ) : hint ? (
        <small className="field-hint" id={messageId}>
          {hint}
        </small>
      ) : null}
    </div>
  );
}
