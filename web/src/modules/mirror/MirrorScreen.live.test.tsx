import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { mirrorApi, type Conversation, type Line } from "./api";
import { MirrorScreen } from "./MirrorScreen";

const replace = vi.fn();
let socketHandler: ((event: { type: string; payload: unknown }) => void) | undefined;
let userRole = "admin";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace }),
  useSearchParams: () => new URLSearchParams("c=real-conversation"),
}));

vi.mock("../../kernel/auth/session", () => ({
  useSession: () => ({ user: { id: "user-1", name: "Operador real", role: userRole } }),
}));

vi.mock("../../kernel/api/useSocket", () => ({
  useSocket: (_module: string, handler: typeof socketHandler) => {
    socketHandler = handler;
  },
}));

vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return {
    ...actual,
    mirrorApi: {
      ...actual.mirrorApi,
      lines: vi.fn(),
      conversations: vi.fn(),
      conversation: vi.fn(),
      messages: vi.fn(),
      markRead: vi.fn(),
      send: vi.fn(),
      lineQR: vi.fn(),
    },
  };
});

const line: Line = {
  id: "real-line",
  name: "linea-real",
  label: "Ventas Reales",
  displayName: "Agencia Central",
  phoneNumber: "+5491100000000",
  status: "open",
  isActive: true,
  unread: 1,
};

const conversation: Conversation = {
  id: "real-conversation",
  instanceId: line.id,
  instanceLabel: line.label,
  contactId: "real-contact",
  contactName: "Contacto Real",
  phoneNumber: "+5491199999999",
  isGroup: false,
  lastMessageAt: "2026-08-18T15:00:00-03:00",
  preview: "Necesito una respuesta real",
  lastType: "text",
  unread: 1,
  needsReply: true,
};

beforeEach(() => {
  userRole = "admin";
  vi.mocked(mirrorApi.lines).mockResolvedValue({ lines: [line] });
  vi.mocked(mirrorApi.conversations).mockResolvedValue({ items: [conversation] });
  vi.mocked(mirrorApi.conversation).mockResolvedValue(conversation);
  vi.mocked(mirrorApi.messages).mockResolvedValue({ items: [] });
  vi.mocked(mirrorApi.markRead).mockResolvedValue(undefined);
  vi.mocked(mirrorApi.lineQR).mockResolvedValue({
    instance: line.name,
    qr: "data:image/png;base64,ZXhpc3Rpbmc=",
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("MirrorScreen real", () => {
  it("usa la composicion nueva con datos reales y sin contenido demo", async () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );

    expect((await screen.findAllByText("Contacto Real")).length).toBeGreaterThan(1);
    expect(screen.getByLabelText("Filtrar por línea")).toBeTruthy();

    const tabs = screen.getByRole("tablist", { name: "Filtrar conversaciones" });
    expect(within(tabs).getByRole("tab", { name: /Todas/ })).toBeTruthy();
    expect(within(tabs).getByRole("tab", { name: /Sin responder/ })).toBeTruthy();

    expect(screen.queryByRole("heading", { name: "Bandeja de entrada" })).toBeNull();
    expect(screen.queryByText("Ana Pérez")).toBeNull();
    expect(screen.queryByText("Operación #10482")).toBeNull();
    expect(screen.queryByText("Nota de demostración")).toBeNull();
    expect(screen.getByRole("heading", { name: "Contacto" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Atención" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Conversación" })).toBeTruthy();
  });

  it("consulta el filtro real al elegir Sin responder", async () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );
    await screen.findAllByText("Contacto Real");

    fireEvent.click(screen.getByRole("tab", { name: /Sin responder/ }));

    await waitFor(() => expect(mirrorApi.conversations).toHaveBeenCalledWith({
      instance: "",
      q: "",
      status: "unanswered",
    }));
  });

  it("permite ocultar y volver a abrir los detalles reales", async () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );
    await screen.findAllByText("Contacto Real");

    fireEvent.click(screen.getByRole("button", { name: "Ocultar detalles" }));
    expect(screen.queryByRole("complementary", { name: "Detalles del contacto" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Ver detalles" }));
    expect(screen.getByRole("complementary", { name: "Detalles del contacto" })).toBeTruthy();
  });

  it("enriches a quoted websocket message before adding it to the open thread", async () => {
    const liveReply = {
      id: "live-reply",
      direction: "inbound" as const,
      origin: "contact" as const,
      type: "text",
      status: "sent",
      body: "Respuesta en vivo",
      timestamp: "2026-08-19T15:00:00Z",
      quotedId: "wa-original",
      quote: {
        id: "original-message",
        direction: "outbound" as const,
        type: "text",
        body: "Mensaje original en vivo",
        senderName: "Operador real",
      },
    };
    vi.mocked(mirrorApi.messages)
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({ items: [liveReply] });

    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );
    await screen.findAllByText("Contacto Real");

    act(() => {
      socketHandler?.({
        type: "message.new",
        payload: {
          messageId: liveReply.id,
          conversationId: conversation.id,
          instanceId: line.id,
          waMessageId: "wa-live-reply",
          direction: "inbound",
          origin: "contact",
          type: "text",
          body: liveReply.body,
          timestamp: liveReply.timestamp,
          quotedId: "wa-original",
        },
      });
    });

    await waitFor(() => expect(mirrorApi.messages).toHaveBeenLastCalledWith(
      conversation.id,
      { around: liveReply.id },
    ));
    expect(await screen.findByText("Mensaje original en vivo")).toBeTruthy();
  });

  it("muestra el QR sobre la misma línea cuando la sesión fue revocada", async () => {
    const reconnecting = { ...line, status: "connecting" as const, wasConnected: true };
    vi.mocked(mirrorApi.lines).mockResolvedValue({ lines: [reconnecting] });

    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );
    await screen.findAllByText("Contacto Real");

    act(() => {
      socketHandler?.({
        type: "line.qr",
        payload: { instance: reconnecting.name, qr: "data:image/png;base64,ZXhpc3Rpbmc=" },
      });
    });

    expect(await screen.findByRole("heading", { name: /Volver a vincular Agencia Central/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Crear y mostrar QR" })).toBeNull();
  });

  it("no expone el QR de una línea a usuarios sin permisos de administración", async () => {
    userRole = "agent";
    const reconnecting = { ...line, status: "connecting" as const };
    vi.mocked(mirrorApi.lines).mockResolvedValue({ lines: [reconnecting] });

    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );
    await screen.findAllByText("Contacto Real");

    act(() => {
      socketHandler?.({
        type: "line.qr",
        payload: { instance: reconnecting.name, qr: "data:image/png;base64,ZXhpc3Rpbmc=" },
      });
    });

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("no abre otro diálogo para una línea nueva que ya está esperando su primer QR", async () => {
    const firstPairing = { ...line, status: "connecting" as const, wasConnected: false };
    vi.mocked(mirrorApi.lines).mockResolvedValue({ lines: [firstPairing] });

    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );
    await screen.findAllByText("Contacto Real");

    act(() => {
      socketHandler?.({
        type: "line.qr",
        payload: { instance: firstPairing.name, qr: "data:image/png;base64,bmV3=" },
      });
    });

    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
