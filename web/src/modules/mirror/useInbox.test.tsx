import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { mirrorApi } from "./api";
import { useInbox } from "./useInbox";

vi.mock("./api", () => ({
  mirrorApi: {
    conversations: vi.fn(),
  },
}));

const conversations = vi.mocked(mirrorApi.conversations);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useInbox", () => {
  it("pide al backend solamente las conversaciones sin responder", async () => {
    conversations.mockResolvedValue({ items: [] });

    renderHook(() => useInbox("", "", true));

    await waitFor(() => expect(conversations).toHaveBeenCalledWith({
      instance: "",
      q: "",
      status: "unanswered",
    }));
  });

  it("conserva el filtro sin responder al paginar", async () => {
    conversations
      .mockResolvedValueOnce({ items: [], nextCursor: "pagina-2" })
      .mockResolvedValueOnce({ items: [] });

    const { result } = renderHook(() => useInbox("linea-1", "ana", true));
    await waitFor(() => expect(result.current.hasMore).toBe(true));

    await act(async () => {
      await result.current.loadMore();
    });

    expect(conversations).toHaveBeenLastCalledWith({
      instance: "linea-1",
      q: "ana",
      cursor: "pagina-2",
      status: "unanswered",
    });
  });

  it("saca de Sin responder una conversación cuando llega un mensaje saliente", async () => {
    conversations.mockResolvedValue({
      items: [{
        id: "conv-1",
        instanceId: "linea-1",
        instanceLabel: "Ventas",
        contactId: "contacto-1",
        contactName: "Cliente real",
        isGroup: false,
        preview: "¿Me respondés?",
        lastType: "text",
        lastMessageAt: "2026-08-18T15:00:00Z",
        unread: 1,
        needsReply: true,
      }],
    });

    const { result } = renderHook(() => useInbox("", "", true));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    act(() => {
      result.current.applyNewMessage(
        "conv-1",
        "Respuesta enviada",
        "text",
        "2026-08-18T15:01:00Z",
        "outbound",
      );
    });

    expect(result.current.items).toHaveLength(0);
  });
});
