import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { mirrorApi, type Conversation, type Message } from "./api";
import { Thread } from "./Thread";

vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return {
    ...actual,
    mirrorApi: {
      ...actual.mirrorApi,
      messages: vi.fn(),
      send: vi.fn(),
      media: vi.fn(),
    },
  };
});

const conversation: Conversation = {
  id: "conversation-1",
  instanceId: "line-1",
  instanceLabel: "Ventas",
  contactId: "contact-1",
  contactName: "Sofia",
  phoneNumber: "+5491111111111",
  isGroup: false,
  unread: 0,
};

const original: Message = {
  id: "message-original",
  direction: "inbound",
  origin: "contact",
  type: "text",
  status: "sent",
  body: "Mensaje original",
  timestamp: "2026-08-18T10:00:00Z",
};

const reply: Message = {
  id: "message-reply",
  direction: "outbound",
  origin: "panel",
  sentByName: "Operador",
  type: "text",
  status: "sent",
  body: "Respuesta concreta",
  timestamp: "2026-08-18T10:01:00Z",
  quotedId: "wa-original",
  quote: {
    id: original.id,
    direction: original.direction,
    type: original.type,
    body: original.body,
    senderName: "Sofia",
  },
};

beforeEach(() => {
  vi.mocked(mirrorApi.messages).mockResolvedValue({ items: [reply, original] });
  Object.defineProperty(Element.prototype, "scrollIntoView", {
    configurable: true,
    value: vi.fn(),
  });
  Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
    configurable: true,
    get: () => 1000,
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("Thread quoted replies", () => {
  it("selects a real message as the composer reply target", async () => {
    render(<Thread conversation={conversation} lineStatus="open" />);
    const originalText = (await screen.findAllByText("Mensaje original"))
      .find((element) => element.tagName === "P");
    const originalBubble = originalText?.closest("article") ?? null;
    expect(originalBubble).not.toBeNull();

    fireEvent.click(within(originalBubble!).getByRole("button", { name: "Responder mensaje" }));

    const replyPreview = screen.getByText("Respondiendo a Sofia").parentElement;
    expect(replyPreview).not.toBeNull();
    expect(within(replyPreview!).getByText("Mensaje original")).toBeTruthy();
  });

  it("scrolls to and marks a quoted message that is already loaded", async () => {
    const { container } = render(<Thread conversation={conversation} lineStatus="open" />);
    const replyBubble = (await screen.findByText("Respuesta concreta")).closest("article");
    fireEvent.click(within(replyBubble!).getByRole("button", { name: "Ir al mensaje citado" }));

    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalled());
    expect(container.querySelector(`[data-message-id="${original.id}"]`)?.getAttribute("aria-current")).toBe("true");
    expect(mirrorApi.messages).toHaveBeenCalledTimes(1);
  });

  it("loads an around window before focusing an unloaded quoted message", async () => {
    const missingReply = {
      ...reply,
      quote: { ...reply.quote!, id: "message-outside-window", body: "Original anterior" },
    };
    vi.mocked(mirrorApi.messages)
      .mockResolvedValueOnce({ items: [missingReply] })
      .mockResolvedValueOnce({ items: [{ ...original, id: "message-outside-window" }, missingReply] });

    render(<Thread conversation={conversation} lineStatus="open" />);
    const replyBubble = (await screen.findByText("Respuesta concreta")).closest("article");
    fireEvent.click(within(replyBubble!).getByRole("button", { name: "Ir al mensaje citado" }));

    await waitFor(() => expect(mirrorApi.messages).toHaveBeenLastCalledWith(
      conversation.id,
      { around: "message-outside-window" },
    ));
    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalled());
  });
});

describe("Thread timeline", () => {
  it("separates messages by their real calendar day", async () => {
    vi.mocked(mirrorApi.messages).mockResolvedValue({
      items: [
        { ...reply, id: "day-two", timestamp: "2026-08-19T12:00:00Z", quote: undefined, quotedId: undefined },
        original,
      ],
    });

    render(<Thread conversation={conversation} lineStatus="open" />);
    await screen.findByText("Respuesta concreta");

    expect(screen.getByRole("separator", { name: "martes, 18 de agosto de 2026" })).toBeTruthy();
    expect(screen.getByRole("separator", { name: "miércoles, 19 de agosto de 2026" })).toBeTruthy();
  });
});
