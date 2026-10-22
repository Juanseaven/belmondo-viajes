import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { mirrorApi, type Message } from "./api";
import { useThread } from "./useThread";

vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return {
    ...actual,
    mirrorApi: {
      ...actual.mirrorApi,
      messages: vi.fn(),
    },
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useThread optimistic reconciliation", () => {
  it("removes the optimistic bubble when the websocket echo arrives before the send response", async () => {
    vi.mocked(mirrorApi.messages).mockResolvedValue({ items: [] });
    const definitive: Message = {
      id: "message-definitive",
      direction: "outbound",
      origin: "panel",
      type: "text",
      status: "sent",
      body: "Mensaje sin duplicar",
      timestamp: "2026-08-21T12:00:00Z",
    };

    const { result } = renderHook(() => useThread("conversation-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => {
      result.current.addOptimistic("request-1", "Mensaje sin duplicar");
      result.current.append(definitive, "wa-1");
    });
    expect(result.current.messages).toHaveLength(2);

    act(() => {
      result.current.settleOptimistic("request-1", "wa-1");
    });

    expect(result.current.messages).toEqual([definitive]);
  });
});

describe("useThread media", () => {
  it("stops showing «Descargando adjunto…» when the media is stored", async () => {
    const photo: Message = {
      id: "message-photo",
      direction: "inbound",
      origin: "contact",
      type: "image",
      status: "sent",
      timestamp: "2026-10-22T12:00:00Z",
      media: { id: "media-1", mimeType: "image/jpeg", status: "pending" },
    };
    vi.mocked(mirrorApi.messages).mockResolvedValue({ items: [photo] });

    const { result } = renderHook(() => useThread("conversation-1"));
    await waitFor(() => expect(result.current.messages).toHaveLength(1));

    act(() => result.current.mediaStored("message-photo"));

    expect(result.current.messages[0].media?.status).toBe("stored");
  });
});
