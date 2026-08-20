import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ContactAvatar } from "./ContactAvatar";

beforeEach(() => {
  class LoadedImage extends EventTarget {
    complete = true;
    naturalWidth = 96;
    crossOrigin: string | null = null;
    referrerPolicy = "";
    src = "";
  }
  vi.stubGlobal("Image", LoadedImage);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ContactAvatar", () => {
  it("uses two initials when the contact has no photo", () => {
    render(<ContactAvatar name="Carla Mendoza" />);

    expect(screen.getByText("CM")).toBeTruthy();
  });

  it("renders the contact photo with an accessible name", () => {
    render(<ContactAvatar name="Carla Mendoza" src="https://example.test/carla.jpg" />);

    const image = screen.getByRole("img", { name: "Carla Mendoza" });
    expect(image.getAttribute("src")).toBe("https://example.test/carla.jpg");
  });
});
