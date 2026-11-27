import { describe, expect, it } from "vitest";
import type { PortalVehicle } from "../../shared/types";
import { photosOf } from "./photos";

const vehicle = (
  images: PortalVehicle["images"],
  mainImageUrl = "",
): PortalVehicle => ({
  model: "Corolla Cross",
  version: "XEI",
  color: "Blanco",
  referencePrice: "1.00",
  categories: [],
  mainImageUrl,
  images,
});

describe("photosOf", () => {
  it("puts the main view first and walks around the car after it", () => {
    const photos = photosOf(
      vehicle([
        { viewpoint: "interior", url: "i" },
        { viewpoint: "exterior_rear", url: "r" },
        { viewpoint: "main_view", url: "m" },
        { viewpoint: "exterior_left", url: "l" },
        { viewpoint: "exterior_front", url: "f" },
      ]),
    );
    expect(photos.map((p) => p.viewpoint)).toEqual([
      "main_view",
      "exterior_front",
      "exterior_left",
      "exterior_rear",
      "interior",
    ]);
    expect(photos[0].label).toBe("Vista principal");
  });

  it("leaves out a view that has no file, and keeps an unknown view last", () => {
    const photos = photosOf(
      vehicle([
        { viewpoint: "roof", url: "x" },
        { viewpoint: "interior", url: "" },
        { viewpoint: "main_view", url: "m" },
      ]),
    );
    expect(photos.map((p) => p.viewpoint)).toEqual(["main_view", "roof"]);
  });

  it("still has the main photo when the list of views is empty", () => {
    expect(photosOf(vehicle([], "main.jpg"))).toEqual([
      { viewpoint: "main_view", url: "main.jpg", label: "Vista principal" },
    ]);
    expect(photosOf(vehicle([], ""))).toEqual([]);
  });
});
