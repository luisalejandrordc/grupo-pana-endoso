import { useState } from "react";
import { initials } from "../lib/format";
import { usePhoto } from "../photos/usePhoto";

/**
 * A person's picture, or their initials when they have none (or it cannot be loaded). Pass `hasPhoto` from the
 * record you hold: without it the picture is never requested.
 */
export function Avatar({
  id,
  name,
  hasPhoto = false,
  size = 32,
  className = "",
}: {
  id?: string;
  name: string;
  hasPhoto?: boolean;
  size?: number;
  className?: string;
}) {
  const photo = usePhoto(id, hasPhoto);
  const [broken, setBroken] = useState<string | null>(null);
  const showImage = Boolean(photo.data) && broken !== photo.data;
  return (
    <span
      className={`avatar ${className}`.trim()}
      style={{ width: size, height: size, fontSize: Math.max(11, size * 0.38) }}
      aria-hidden={showImage ? undefined : true}
    >
      {showImage ? (
        <img
          src={photo.data}
          alt={name}
          width={size}
          height={size}
          onError={() => setBroken(photo.data ?? null)}
        />
      ) : (
        initials(name)
      )}
    </span>
  );
}
