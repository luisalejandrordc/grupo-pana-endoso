import { useState } from "react";
import type { PortalVehicle } from "../../shared/types";
import { labelOf, viewpointLabels } from "./labels";
import { VehicleImage } from "./VehicleImage";

/** The main photo, large, with a thumbnail for each other view the catalog has. */
export function VehicleGallery({ vehicle }: { vehicle: PortalVehicle }) {
  const name = `${vehicle.model} ${vehicle.version}, color ${vehicle.color}`;
  // The main view comes first; the rest follow in the order the catalog gives them.
  const photos = vehicle.images.filter((image) => image.url);
  const [selected, setSelected] = useState(
    () => photos.find((image) => image.url === vehicle.mainImageUrl)?.url ?? "",
  );
  const shown = selected || vehicle.mainImageUrl || photos[0]?.url || "";

  return (
    <div className="gallery">
      <VehicleImage className="gallery-main" src={shown} alt={name} />
      {photos.length > 1 && (
        <ul className="gallery-thumbs" aria-label="Vistas del vehículo">
          {photos.map((image) => (
            <li key={image.viewpoint}>
              <button
                type="button"
                className={`gallery-thumb ${image.url === shown ? "is-active" : ""}`.trim()}
                aria-pressed={image.url === shown}
                aria-label={labelOf(viewpointLabels, image.viewpoint)}
                title={labelOf(viewpointLabels, image.viewpoint)}
                onClick={() => setSelected(image.url)}
              >
                <img src={image.url} alt="" loading="lazy" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
