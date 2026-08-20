import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Message } from "./api";
import { MessageBubble } from "./MessageBubble";

const message: Message = {
  id: "message-1",
  direction: "inbound",
  origin: "contact",
  type: "text",
  status: "sent",
  body: "Hola grupo",
  timestamp: "2026-08-05T17:00:01Z",
  senderName: "Sofia",
  senderPhone: "+5491126374402",
};

afterEach(cleanup);

describe("MessageBubble group attribution", () => {
  it("shows who sent an inbound group message", () => {
    render(<MessageBubble message={message} isGroup />);

    expect(screen.getByText("Sofia")).toBeTruthy();
    expect(screen.getByText("Hola grupo")).toBeTruthy();
  });

  it("does not repeat the contact name in a private conversation", () => {
    render(<MessageBubble message={message} />);

    expect(screen.queryByText("Sofia")).toBeNull();
  });

  it("falls back to the sender JID when WhatsApp has no name or phone", () => {
    render(
      <MessageBubble
        message={{
          ...message,
          senderName: undefined,
          senderPhone: undefined,
          senderJid: "94132898906287@lid",
        }}
        isGroup
      />,
    );

    expect(screen.getByText("94132898906287@lid")).toBeTruthy();
  });
});

describe("MessageBubble quoted replies", () => {
  it("offers a reply action and returns the selected real message", () => {
    const onReply = vi.fn();
    render(<MessageBubble message={message} onReply={onReply} />);

    fireEvent.click(screen.getByRole("button", { name: "Responder mensaje" }));

    expect(onReply).toHaveBeenCalledWith(message);
  });

  it("shows the quoted author and opens the original message", () => {
    const onOpenQuote = vi.fn();
    render(
      <MessageBubble
        message={{
          ...message,
          id: "reply-1",
          body: "Esta es mi respuesta",
          quotedId: "wa-original",
          quote: {
            id: "message-original",
            direction: "inbound",
            type: "text",
            body: "Este es el mensaje original",
            senderName: "Sofia",
          },
        }}
        onOpenQuote={onOpenQuote}
      />,
    );

    expect(screen.getByText("Sofia")).toBeTruthy();
    expect(screen.getByText("Este es el mensaje original")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Ir al mensaje citado" }));
    expect(onOpenQuote).toHaveBeenCalledWith("message-original");
  });

  it("keeps an unavailable historical quote visible as context", () => {
    render(<MessageBubble message={{ ...message, quotedId: "wa-no-sincronizado" }} />);

    expect(screen.getByText("Mensaje citado no disponible")).toBeTruthy();
  });
});
