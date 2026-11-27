import { useRef, type TouchEvent } from "react";
import type { PortalVehicle } from "../../shared/types";
import { labelOf, viewpointLabels } from "./labels";

export interface Photo {
  viewpoint: string;
  url: string;
  label: string;
}

const ORDER = [
  "main_view",
  "exterior_front",
  "exterior_left",
  "exterior_right",
  "exterior_rear",
  "interior",
];

/**
 * Every photo the customer can look at, the main view always first and the rest in a natural walk around the
 * car. A view the catalog has no file for is left out. A vehicle with no list but a main photo still has one.
 */
export function photosOf(vehicle: PortalVehicle): Photo[] {
  const rank = (viewpoint: string) => {
    const at = ORDER.indexOf(viewpoint);
    return at === -1 ? ORDER.length : at;
  };
  const photos = vehicle.images
    .filter((image) => image.url)
    .map((image) => ({
      viewpoint: image.viewpoint,
      url: image.url,
      label: labelOf(viewpointLabels, image.viewpoint),
    }))
    .sort((a, b) => rank(a.viewpoint) - rank(b.viewpoint));
  if (photos.length === 0 && vehicle.mainImageUrl)
    return [
      {
        viewpoint: "main_view",
        url: vehicle.mainImageUrl,
        label: viewpointLabels.main_view,
      },
    ];
  return photos;
}

/** Touch handlers that call `onPrev` or `onNext` after a horizontal swipe. */
export function useSwipe(onPrev: () => void, onNext: () => void) {
  const start = useRef<number | null>(null);
  return {
    onTouchStart: (event: TouchEvent) => {
      start.current = event.touches[0].clientX;
    },
    onTouchEnd: (event: TouchEvent) => {
      if (start.current === null) return;
      const distance = event.changedTouches[0].clientX - start.current;
      start.current = null;
      if (Math.abs(distance) > 50) (distance > 0 ? onPrev : onNext)();
    },
  };
}
