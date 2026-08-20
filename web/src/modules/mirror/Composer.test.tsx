import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Composer } from "./Composer";
import { mirrorApi, type MessageQuote } from "./api";

vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return {
    ...actual,
    mirrorApi: { ...actual.mirrorApi, send: vi.fn() },
  };
});

const quote: MessageQuote = {
  id: "message-original",
  direction: "inbound",
  type: "text",
  body: "Necesito confirmar esta reserva",
  senderName: "Sofia",
};

function renderComposer(overrides: Partial<React.ComponentProps<typeof Composer>> = {}) {
  const props: React.ComponentProps<typeof Composer> = {
    conversationId: "conversation-1",
    onOptimistic: vi.fn(),
    onSettled: vi.fn(),
    ...overrides,
  };
  return { props, ...render(<Composer {...props} />) };
}

beforeEach(() => {
  vi.mocked(mirrorApi.send).mockResolvedValue({
    id: "outbound-1",
    idempotencyKey: "key-1",
    waMessageId: "wa-reply",
    status: "sent",
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("Composer quoted replies", () => {
  it("shows and cancels the selected reply", () => {
    const onCancelReply = vi.fn();
    renderComposer({ replyTo: quote, onCancelReply });

    expect(screen.getByText("Respondiendo a Sofia")).toBeTruthy();
    expect(screen.getByText("Necesito confirmar esta reserva")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancelar respuesta" }));
    expect(onCancelReply).toHaveBeenCalledTimes(1);
  });

  it("sends replyToMessageId and clears the reply only after success", async () => {
    const onSent = vi.fn();
    const { props } = renderComposer({ replyTo: quote, onSent });
    fireEvent.change(screen.getByRole("textbox", { name: "Mensaje" }), {
      target: { value: "Ya la confirmé" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Enviar mensaje" }));

    await waitFor(() => expect(mirrorApi.send).toHaveBeenCalledWith(
      "conversation-1",
      expect.objectContaining({
        body: "Ya la confirmé",
        replyToMessageId: "message-original",
      }),
    ));
    expect(props.onOptimistic).toHaveBeenCalledWith(
      expect.any(String), "Ya la confirmé", undefined, quote,
    );
    expect(onSent).toHaveBeenCalledTimes(1);
  });

  it("keeps the reply selected when sending fails", async () => {
    vi.mocked(mirrorApi.send).mockRejectedValue(new Error("sin conexión"));
    const onSent = vi.fn();
    const { props } = renderComposer({ replyTo: quote, onSent });
    fireEvent.change(screen.getByRole("textbox", { name: "Mensaje" }), {
      target: { value: "Reintentaré" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Enviar mensaje" }));

    await waitFor(() => expect(props.onSettled).toHaveBeenCalledWith(
      expect.any(String), null, "No se pudo enviar el mensaje",
    ));
    expect(onSent).not.toHaveBeenCalled();
  });
});
