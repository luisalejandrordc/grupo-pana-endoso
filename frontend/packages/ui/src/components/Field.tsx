import {
  cloneElement,
  isValidElement,
  useId,
  type ReactElement,
  type ReactNode,
} from "react";

const CONTROLS = new Set(["input", "select", "textarea"]);

/**
 * A labelled form control with an optional hint or error underneath.
 *
 * When the child is a single input, select or textarea, the label is attached with htmlFor and the hint or
 * error with aria-describedby, so a screen reader announces the label as the name and the message as the
 * description (and the error is marked with aria-invalid). Any other child falls back to a wrapping label.
 */
export function Field({
  label,
  children,
  hint,
  error,
  className = "",
}: {
  label: string;
  children: ReactNode;
  hint?: string;
  error?: string;
  className?: string;
}) {
  const id = useId();
  const messageId = `${id}-message`;
  const classes = `field ${className} ${error ? "has-error" : ""}`.trim();
  const message = error ? (
    <small className="field-error" id={messageId} role="alert">
      {error}
    </small>
  ) : hint ? (
    <small className="field-hint" id={messageId}>
      {hint}
    </small>
  ) : null;

  if (
    isValidElement(children) &&
    typeof children.type === "string" &&
    CONTROLS.has(children.type)
  ) {
    const control = children as ReactElement<Record<string, unknown>>;
    return (
      <div className={classes}>
        <label className="field-label" htmlFor={id}>
          {label}
        </label>
        {cloneElement(control, {
          id,
          "aria-invalid": error ? true : undefined,
          "aria-describedby": message ? messageId : undefined,
        })}
        {message}
      </div>
    );
  }

  return (
    <label className={classes}>
      <span className="field-label">{label}</span>
      {children}
      {message}
    </label>
  );
}
