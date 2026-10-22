import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { mirrorApi, type Conversation, type Line } from "./api";
import { MirrorScreen } from "./MirrorScreen";
import { operacionesApi } from "../operaciones/api";

const replace = vi.fn();
let socketHandler: ((event: { type: string; payload: unknown }) => void) | undefined;
let userRole = "admin";
const tripHookState = vi.hoisted(() => ({ active: undefined as unknown, reload: vi.fn() }));

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

const activeTrip = {
  id: "trip-real", number: 10482, customerId: "customer-real", status: "cotizado" as const,
  destination: "Iguazú", departAt: "2026-09-12", returnAt: "2026-09-15",
  createdAt: "2026-08-20", passengerCount: 0,
  customer: { id: "customer-real", name: "Contacto Real", customerSince: "2026-08-20", contactIds: ["real-contact"] },
  passengers: [], payment: null, originCountry: "AR", itinerary: [], requirements: [],
  controls: [{ id: "control-real", itemId: "visa", stage: "control" as const, closing: "accion" as const, title: "Avisar por visa", actionLabel: "Avisar al pasajero", message: "Revisemos la visa.", critical: true, status: "abierto" as const, dueAt: "2026-08-19", evidence: [] }],
};

// Archivos también escucha al espejo; el doble de useSocket guarda un solo
// handler, que tiene que ser el de la pantalla.
vi.mock("../operaciones/CustomerFiles", () => ({ CustomerFiles: () => null, useFileCount: () => undefined }));
vi.mock("../operaciones/useTrips", () => ({
  useTrips: () => ({ byContact: tripHookState.active ? { "real-contact": tripHookState.active } : {}, contactTrips: tripHookState.active ? [tripHookState.active] : [], active: tripHookState.active, loading: false, error: undefined, reload: tripHookState.reload, select: vi.fn() }),
}));

vi.mock("../operaciones/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../operaciones/api")>();
  return { ...actual, operacionesApi: { ...actual.operacionesApi, closeControl: vi.fn(), reopenControl: vi.fn(), addPassenger: vi.fn(), putPayment: vi.fn(), createTrip: vi.fn(), sellers: vi.fn().mockResolvedValue({ items: [] }), countries: vi.fn().mockResolvedValue({ items: [] }) } };
});

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
  tripHookState.active = activeTrip;
  vi.mocked(mirrorApi.lines).mockResolvedValue({ lines: [line] });
  vi.mocked(mirrorApi.conversations).mockResolvedValue({ items: [conversation] });
  vi.mocked(mirrorApi.conversation).mockResolvedValue(conversation);
  vi.mocked(mirrorApi.messages).mockResolvedValue({ items: [] });
  vi.mocked(mirrorApi.markRead).mockResolvedValue(undefined);
  vi.mocked(mirrorApi.send).mockResolvedValue({ id: "outbound-action", idempotencyKey: "action-key", waMessageId: "wa-action", status: "sent" });
  vi.mocked(operacionesApi.closeControl).mockResolvedValue(undefined);
  tripHookState.reload.mockResolvedValue(undefined);
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
  it("sin viaje ofrece crear uno y lo crea con lo cargado", async () => {
    tripHookState.active = undefined;
    vi.mocked(operacionesApi.createTrip).mockResolvedValue({
      id: "trip-draft", number: 10483, customerId: "customer-real", status: "consulta",
      destination: "", createdAt: "2026-08-21", passengerCount: 0,
    });

    render(<TooltipProvider><MirrorScreen /></TooltipProvider>);
    await screen.findAllByText("Contacto Real");
    expect(screen.getByText("Este contacto no tiene viajes")).toBeTruthy();
    // Abrir el chat no crea nada.
    expect(operacionesApi.createTrip).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Nuevo viaje" }));
    fireEvent.change(screen.getByLabelText("Salida"), { target: { value: "2026-12-10" } });
    fireEvent.click(screen.getByRole("button", { name: "Crear viaje" }));

    await waitFor(() => expect(operacionesApi.createTrip).toHaveBeenCalledWith({
      contactId: "real-contact", originCountry: "AR", departAt: "2026-12-10",
    }));
  });

  it("usa la composicion nueva con datos reales y sin contenido demo", async () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );

    expect((await screen.findAllByText("Contacto Real")).length).toBeGreaterThan(0);
    // La ficha se lee: el cliente es un dato, no un campo abierto.
    expect(screen.queryAllByDisplayValue("Contacto Real")).toHaveLength(0);
    expect(screen.getByRole("heading", { name: "Chats" })).toBeTruthy();
    expect(screen.getByLabelText("Filtrar por línea")).toBeTruthy();

    const tabs = screen.getByRole("tablist", { name: "Filtrar conversaciones" });
    expect(within(tabs).getByRole("tab", { name: /^Todos$/ })).toBeTruthy();
    expect(within(tabs).getByRole("tab", { name: /No leídos 1/ })).toBeTruthy();

    expect(screen.queryByRole("heading", { name: "Bandeja de entrada" })).toBeNull();
    expect(screen.queryByText("Ana Pérez")).toBeNull();
    expect(screen.getByText("Viaje #10482")).toBeTruthy();
    expect(screen.getByText("Viaje #10482 · Iguazú")).toBeTruthy();
    expect(screen.getByText("#10482 Iguazú")).toBeTruthy();
    expect(screen.getByText("Iguazú · sin vendedor")).toBeTruthy();
    expect(screen.getAllByText("Avisar por visa").length).toBeGreaterThan(0);
    expect(screen.queryByText("Nota de demostración")).toBeNull();
  });

  it("consulta el filtro real al elegir No leídos", async () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );
    await screen.findAllByText("Contacto Real");

    fireEvent.click(screen.getByRole("tab", { name: /No leídos/ }));

    await waitFor(() => expect(mirrorApi.conversations).toHaveBeenCalledWith({
      instance: "",
      q: "",
      status: "unread",
    }));
  });

  it("permite ocultar y volver a abrir los detalles reales", async () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );
    await screen.findAllByText("Contacto Real");

    const close = within(screen.getByRole("complementary", { name: "Viaje" })).getByRole("button", { name: "Ocultar viaje" });
    fireEvent.click(close);
    expect(screen.queryByRole("complementary", { name: "Viaje" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Mostrar viaje" }));
    expect(screen.getByRole("complementary", { name: "Viaje" })).toBeTruthy();
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

  it("cierra una acción con el mensaje definitivo del webhook y no con el outbound provisional", async () => {
    render(<TooltipProvider><MirrorScreen /></TooltipProvider>);
    await screen.findAllByText("Contacto Real");

    fireEvent.click(screen.getByRole("button", { name: "Avisar al pasajero" }));
    const composer = screen.getByRole("textbox", { name: "Mensaje" });
    expect(composer).toHaveProperty("value", "Revisemos la visa.");
    fireEvent.submit(composer.closest("form")!);
    await waitFor(() => expect(mirrorApi.send).toHaveBeenCalled());
    expect(operacionesApi.closeControl).not.toHaveBeenCalled();

    act(() => socketHandler?.({
      type: "message.new",
      payload: {
        messageId: "message-definitive", conversationId: conversation.id, instanceId: line.id,
        waMessageId: "wa-action", direction: "outbound", origin: "panel", type: "text",
        body: "Revisemos la visa.", timestamp: "2026-08-20T15:00:00Z",
      },
    }));

    await waitFor(() => expect(operacionesApi.closeControl).toHaveBeenCalledWith("control-real", "message-definitive"));
  });

  it("no pierde la evidencia si el webhook gana la carrera al POST de envío", async () => {
    let resolveSend!: (value: { id: string; idempotencyKey: string; waMessageId: string; status: "sent" }) => void;
    vi.mocked(mirrorApi.send).mockReturnValue(new Promise((resolve) => { resolveSend = resolve; }));
    render(<TooltipProvider><MirrorScreen /></TooltipProvider>);
    await screen.findAllByText("Contacto Real");

    fireEvent.click(screen.getByRole("button", { name: "Avisar al pasajero" }));
    fireEvent.submit(screen.getByRole("textbox", { name: "Mensaje" }).closest("form")!);
    await waitFor(() => expect(mirrorApi.send).toHaveBeenCalled());

    act(() => socketHandler?.({
      type: "message.new",
      payload: {
        messageId: "message-early", conversationId: conversation.id, instanceId: line.id,
        waMessageId: "wa-early", direction: "outbound", origin: "panel", type: "text",
        body: "Revisemos la visa.", timestamp: "2026-08-20T15:00:00Z",
      },
    }));
    expect(operacionesApi.closeControl).not.toHaveBeenCalled();

    act(() => resolveSend({ id: "outbound-early", idempotencyKey: "early", waMessageId: "wa-early", status: "sent" }));
    await waitFor(() => expect(operacionesApi.closeControl).toHaveBeenCalledWith("control-real", "message-early"));
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
