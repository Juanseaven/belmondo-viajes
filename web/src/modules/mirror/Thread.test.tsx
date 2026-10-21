import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

let intersectionCallback: IntersectionObserverCallback | undefined;

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
  vi.stubGlobal("IntersectionObserver", class {
    constructor(callback: IntersectionObserverCallback) {
      intersectionCallback = callback;
    }
    observe() {}
    disconnect() {}
    unobserve() {}
    takeRecords() { return []; }
    root = null;
    rootMargin = "0px";
    thresholds = [0];
  });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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

    expect(screen.getByRole("separator", { name: /^Martes 18 de agosto/ })).toBeTruthy();
    expect(screen.getByRole("separator", { name: /^Miércoles 19 de agosto/ })).toBeTruthy();
  });

  it("marks where the unread messages start, counting back the received ones", async () => {
    vi.mocked(mirrorApi.messages).mockResolvedValue({
      items: [
        original,
        { ...original, id: "out-1", direction: "outbound", origin: "device", body: "Te paso el presupuesto", timestamp: "2026-08-18T11:00:00Z" },
        { ...original, id: "in-1", body: "Dale", timestamp: "2026-08-18T12:00:00Z" },
        { ...original, id: "in-2", body: "¿Cuándo?", timestamp: "2026-08-18T12:01:00Z" },
      ],
    });

    render(<Thread conversation={{ ...conversation, unread: 2 }} lineStatus="open" />);
    await screen.findByText("¿Cuándo?");

    const divider = (await screen.findByText("2 mensajes sin leer")).parentElement!;
    expect(divider.nextElementSibling?.textContent).toContain("Dale");
    expect(screen.getByLabelText("Enviado desde el teléfono")).toBeTruthy();
    expect(screen.queryByText(/desde el teléfono/i)).toBeNull();
  });

  it("stays anchored at the bottom when sending while older messages are loading", async () => {
    let resolveOlder!: (value: { items: Message[] }) => void;
    vi.mocked(mirrorApi.messages)
      .mockResolvedValueOnce({ items: [original], nextCursor: "older-page" })
      .mockReturnValueOnce(new Promise((resolve) => { resolveOlder = resolve; }));
    vi.mocked(mirrorApi.send).mockReturnValue(new Promise(() => undefined));

    const { container } = render(<Thread conversation={conversation} lineStatus="open" />);
    await screen.findByText("Mensaje original");
    const messages = container.querySelector<HTMLElement>(".messages")!;
    expect(messages.scrollTop).toBe(1000);

    act(() => {
      intersectionCallback?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Mensaje" }), {
      target: { value: "Nuevo mensaje" },
    });
    fireEvent.submit(screen.getByRole("textbox", { name: "Mensaje" }).closest("form")!);

    await screen.findByText("Nuevo mensaje");
    expect(messages.scrollTop).toBe(1000);

    act(() => resolveOlder({ items: [] }));
  });
});
