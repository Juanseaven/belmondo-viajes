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
  it("pide al backend solamente las conversaciones no leídas", async () => {
    conversations.mockResolvedValue({ items: [] });

    renderHook(() => useInbox("", "", true));

    await waitFor(() => expect(conversations).toHaveBeenCalledWith({
      instance: "",
      q: "",
      status: "unread",
    }));
  });

  it("conserva el filtro de no leídos al paginar", async () => {
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
      status: "unread",
    });
  });

  it("saca de No leídos una conversación cuando se marca como leída", async () => {
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

    act(() => result.current.applyRead("conv-1"));

    expect(result.current.items).toHaveLength(0);
  });

  it("actualiza durante una rafaga continua sin esperar a que termine", async () => {
    vi.useFakeTimers();
    try {
      conversations.mockResolvedValue({ items: [] });
      const { result } = renderHook(() => useInbox("", ""));

      await act(async () => {
        await vi.runAllTimersAsync();
      });
      conversations.mockClear();

      act(() => result.current.scheduleReload());
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0);
      });
      expect(conversations).toHaveBeenCalledTimes(1);

      for (let second = 1; second <= 4; second += 1) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(1000);
        });
        act(() => result.current.scheduleReload());
      }
      expect(conversations).toHaveBeenCalledTimes(1);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(conversations).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
