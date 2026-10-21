import { cleanup, render, screen, waitFor } from "@testing-library/react";
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
	vi.stubGlobal("fetch", vi.fn());
	vi.stubGlobal("URL", {
		...URL,
		createObjectURL: vi.fn(() => "blob:avatar-1"),
		revokeObjectURL: vi.fn(),
	});
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

	it("loads internal avatar routes with authentication and renders a blob URL", async () => {
		window.localStorage.setItem("viajes_crm_token", "jwt-avatar");
		vi.mocked(fetch).mockImplementation(() =>
			Promise.resolve(
				new Response(new Blob(["avatar"], { type: "image/jpeg" }), { status: 200 }),
			),
		);

		render(
			<ContactAvatar name="Carla Mendoza" src="/api/mirror/conversations/c1/avatar" />,
		);

		const image = await screen.findByRole("img", { name: "Carla Mendoza" });
		expect(image.getAttribute("src")).toBe("blob:avatar-1");
		expect(fetch).toHaveBeenCalledWith(
			expect.stringContaining("/api/mirror/conversations/c1/avatar"),
			expect.objectContaining({
				headers: expect.objectContaining({ Authorization: "Bearer jwt-avatar" }),
			}),
		);
	});

	it("keeps initials when an internal avatar is unavailable", async () => {
		vi.mocked(fetch).mockResolvedValue(
			new Response(JSON.stringify({ error: { code: "NOT_FOUND", message: "no disponible" } }), {
				status: 404,
				headers: { "Content-Type": "application/json" },
			}),
		);

		render(
			<ContactAvatar name="Carla Mendoza" src="/api/mirror/conversations/c1/avatar" />,
		);

		await waitFor(() => expect(fetch).toHaveBeenCalled());
		expect(screen.getByText("CM")).toBeTruthy();
		expect(screen.queryByRole("img", { name: "Carla Mendoza" })).toBeNull();
	});

	it("revokes blob URLs and ignores a late response after unmount", async () => {
		let resolveFetch!: (response: Response) => void;
		vi.mocked(fetch).mockReturnValue(
			new Promise<Response>((resolve) => {
				resolveFetch = resolve;
			}),
		);
		const view = render(
			<ContactAvatar name="Carla Mendoza" src="/api/mirror/conversations/c1/avatar" />,
		);
		await waitFor(() => expect(fetch).toHaveBeenCalled());
		view.unmount();
		resolveFetch(new Response(new Blob(["late"], { type: "image/jpeg" }), { status: 200 }));

		await waitFor(() => expect(URL.createObjectURL).not.toHaveBeenCalled());

		vi.mocked(fetch).mockImplementation(() =>
			Promise.resolve(
				new Response(new Blob(["avatar"], { type: "image/jpeg" }), { status: 200 }),
			),
		);
		const mounted = render(
			<ContactAvatar name="Carla Mendoza" src="/api/mirror/conversations/c2/avatar" />,
		);
		await screen.findByRole("img", { name: "Carla Mendoza" });
		mounted.unmount();
		expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:avatar-1");
	});

	it("does not render the previous conversation photo while src changes", async () => {
		vi.mocked(URL.createObjectURL)
			.mockReturnValueOnce("blob:avatar-c1")
			.mockReturnValueOnce("blob:avatar-c2");
		vi.mocked(fetch).mockImplementation(() =>
			Promise.resolve(
				new Response(new Blob(["avatar"], { type: "image/jpeg" }), { status: 200 }),
			),
		);
		const view = render(
			<ContactAvatar name="Carla Mendoza" src="/api/mirror/conversations/c1/avatar" />,
		);
		expect((await screen.findByRole("img", { name: "Carla Mendoza" })).getAttribute("src")).toBe(
			"blob:avatar-c1",
		);

		view.rerender(
			<ContactAvatar name="Carla Mendoza" src="/api/mirror/conversations/c2/avatar" />,
		);
		await waitFor(() =>
			expect(screen.getByRole("img", { name: "Carla Mendoza" }).getAttribute("src")).toBe(
				"blob:avatar-c2",
			),
		);
		expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:avatar-c1");
	});
});
