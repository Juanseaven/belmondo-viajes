import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError, apiBlob, token } from "./client";

beforeEach(() => {
  window.localStorage.clear();
  vi.unstubAllGlobals();
});

describe("apiBlob", () => {
  it("sends the JWT and returns the response blob", async () => {
    token.set("jwt-avatar");
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("image-bytes", {
        status: 200,
        headers: { "Content-Type": "image/png" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await apiBlob("/api/mirror/conversations/c1/avatar");

    expect(result.type).toBe("image/png");
    expect(await result.text()).toBe("image-bytes");
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/mirror/conversations/c1/avatar"),
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer jwt-avatar" }),
      }),
    );
  });

  it("keeps the common ApiError contract", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: { code: "UNAUTHORIZED", message: "sesión vencida" } }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await expect(apiBlob("/api/mirror/conversations/c1/avatar")).rejects.toMatchObject({
      status: 401,
      code: "UNAUTHORIZED",
      isAuthError: true,
    } satisfies Partial<ApiError>);
  });
});
