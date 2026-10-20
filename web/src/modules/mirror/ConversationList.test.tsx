import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { Conversation } from "./api";
import { ConversationList } from "./ConversationList";
import type { useInbox } from "./useInbox";

afterEach(cleanup);

function inboxWith(items: Conversation[]): ReturnType<typeof useInbox> {
  return {
    items,
    loading: false,
    error: null,
    hasMore: false,
    loadMore: async () => undefined,
    reload: async () => undefined,
    scheduleReload: () => undefined,
    applyNewMessage: () => undefined,
    applyRead: () => undefined,
  };
}

describe("ConversationList", () => {
  it("turns an empty inbox into a clear next step", () => {
    render(
      <ConversationList
        inbox={inboxWith([])}
        selectedId=""
        onSelect={() => undefined}
      />,
    );

    expect(screen.getByText("No hay conversaciones todavía.")).toBeTruthy();
    expect(screen.getByText(/conectá una línea/i)).toBeTruthy();
  });

  it("keeps the unread count in the row without repeating the line origin", () => {
    render(
      <ConversationList
        inbox={inboxWith([{
          id: "conversation-1",
          instanceId: "line-1",
          instanceLabel: "Línea Norte",
          contactId: "contact-1",
          contactName: "Carla Mendoza",
          phoneNumber: "+54 9 11 3456-7890",
          isGroup: false,
          preview: "Quería consultar por un viaje",
          lastType: "text",
          lastMessageAt: "2026-08-10T14:45:00Z",
          unread: 3,
        }])}
        selectedId=""
        onSelect={() => undefined}
      />,
    );

    expect(screen.getByText("CM")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.queryByText("Línea Norte")).toBeNull();
  });

  it("marks a conversation with pending messages as unread", () => {
    render(
      <ConversationList
        inbox={inboxWith([{
          id: "conversation-1",
          instanceId: "line-1",
          instanceLabel: "Línea Norte",
          contactId: "contact-1",
          contactName: "Carla Mendoza",
          isGroup: false,
          preview: "¿Me confirmás?",
          unread: 2,
          needsReply: false,
        }])}
        selectedId=""
        onSelect={() => undefined}
      />,
    );

    expect(screen.getByText("Sin leer")).toBeTruthy();
  });
});
