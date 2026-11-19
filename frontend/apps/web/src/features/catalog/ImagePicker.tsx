import { useEffect, useId, useMemo, useRef } from "react";
import { Icon } from "../../shared/ui/Icon";
import { ACCEPTED_IMAGE_TYPES } from "./images";

/** Picks one image file and previews it. The caller keeps the file and decides when to upload it. */
export function ImagePicker({
  label,
  file,
  onPick,
  error,
  disabled = false,
  hint = "JPEG, PNG o WebP, hasta 5 MB.",
}: {
  label: string;
  file: File | null;
  onPick: (file: File | null) => void;
  error?: string;
  disabled?: boolean;
  hint?: string;
}) {
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const preview = useMemo(
    () =>
      file && typeof URL.createObjectURL === "function"
        ? URL.createObjectURL(file)
        : null,
    [file],
  );
  useEffect(
    () => () => {
      if (preview && typeof URL.revokeObjectURL === "function")
        URL.revokeObjectURL(preview);
    },
    [preview],
  );

  return (
    <div className={`image-picker ${error ? "has-error" : ""}`.trim()}>
      <span className="field-label" id={`${id}-label`}>
        {label}
      </span>
      <div className="image-picker-box">
        {preview ? (
          <img src={preview} alt={`Vista previa: ${label}`} />
        ) : (
          <div className="image-picker-empty">
            <Icon name="car" size={22} />
            <span>Sin imagen</span>
          </div>
        )}
        <div className="image-picker-actions">
          <button
            type="button"
            className="button button-secondary button-sm"
            disabled={disabled}
            onClick={() => input.current?.click()}
          >
            <Icon name="upload" size={14} />{" "}
            {file ? "Cambiar imagen" : "Elegir imagen"}
          </button>
          {file && (
            <button
              type="button"
              className="link-button"
              onClick={() => onPick(null)}
            >
              Quitar
            </button>
          )}
        </div>
        <input
          ref={input}
          type="file"
          hidden
          aria-labelledby={`${id}-label`}
          accept={ACCEPTED_IMAGE_TYPES.join(",")}
          disabled={disabled}
          onChange={(event) => {
            onPick(event.target.files?.[0] ?? null);
            event.target.value = "";
          }}
        />
      </div>
      {error ? (
        <small className="field-error" role="alert">
          {error}
        </small>
      ) : (
        <small className="field-hint">{hint}</small>
      )}
    </div>
  );
}
