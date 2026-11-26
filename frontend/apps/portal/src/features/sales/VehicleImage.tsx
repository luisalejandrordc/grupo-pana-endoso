import { Icon } from "@endosoflow/ui";
import { useState } from "react";

/** A vehicle photo, or a neutral stand-in when the color has none or the photo does not load. */
export function VehicleImage({
  src,
  alt,
  className = "",
}: {
  src: string;
  alt: string;
  className?: string;
}) {
  const [failed, setFailed] = useState(false);
  if (!src || failed)
    return (
      <div
        className={`vehicle-image vehicle-image-empty ${className}`.trim()}
        role="img"
        aria-label={`${alt} (sin foto disponible)`}
      >
        <Icon name="car" size={40} strokeWidth={1.4} />
      </div>
    );
  return (
    <img
      className={`vehicle-image ${className}`.trim()}
      src={src}
      alt={alt}
      loading="lazy"
      onError={() => setFailed(true)}
    />
  );
}
