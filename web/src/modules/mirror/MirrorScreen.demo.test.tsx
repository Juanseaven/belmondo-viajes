import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { MirrorScreen } from "./MirrorScreen";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams("demo=1"),
}));

vi.mock("../../kernel/auth/session", () => ({
  useSession: () => ({ user: { role: "admin" } }),
}));

vi.mock("../../kernel/api/useSocket", () => ({
  useSocket: () => undefined,
}));

vi.mock("./useInbox", () => ({
  useInbox: () => ({
    items: [],
    loading: false,
    error: null,
    hasMore: false,
    loadMore: async () => undefined,
    reload: async () => undefined,
    applyNewMessage: () => undefined,
    applyRead: () => undefined,
  }),
}));

vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return {
    ...actual,
    mirrorApi: {
      ...actual.mirrorApi,
      lines: async () => ({ lines: [] }),
    },
  };
});

afterEach(cleanup);

describe("MirrorScreen demo", () => {
  it("shows a populated multi-line inbox without loading backend data", () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );

    // La conversación abierta se ve en la lista, en el hilo y en la ficha.
    expect(screen.getAllByText("Ana Pérez").length).toBeGreaterThan(1);
    expect(screen.getAllByText("Ventas Nacional").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Ventas Exterior").length).toBeGreaterThan(0);

    // Franja de operación: el viaje queda a la vista mientras se conversa.
    expect(screen.getAllByText("Iguazú").length).toBeGreaterThan(0);
    expect(screen.getAllByText("USD 1.840").length).toBeGreaterThan(0);
    expect(screen.getByText("Operación #10482")).toBeTruthy();
    expect(screen.getAllByText("#10482 · Iguazú").length).toBeGreaterThan(0);

    // El chip de la fila sale del control abierto más urgente, no de un texto
    // escrito a mano; el crítico gana aunque venza después.
    expect(screen.getAllByText("Vencimiento de DNI o pasaporte").length).toBeGreaterThan(0);

    // Una conversación sin operación conserva su estado comercial.
    expect(screen.getByText("Sin responder 4 h")).toBeTruthy();

    // Las solapas quedaron en dos y el hilo no lleva acciones sueltas.
    const inboxTabs = within(screen.getByRole("tablist", { name: "Filtrar conversaciones" }));
    expect(inboxTabs.getAllByRole("tab").map((tab) => tab.textContent?.replace(/\s+/g, " ").trim())).toEqual([
      "Todas 10",
      "Sin responder 3",
    ]);
    expect(screen.queryByText("Asignar")).toBeNull();
    expect(screen.queryByText("Resolver")).toBeNull();
    expect(screen.queryByText("Enviar cotización")).toBeNull();

    // Un saliente se atribuye a la línea por la que salió, nunca a una persona.
    const outboundReply = screen
      .getByText("Hola Ana, sí, queda lugar del 12 al 15. ¿Cuántos viajarían?")
      .closest("article");
    expect(outboundReply).not.toBeNull();
    expect(within(outboundReply!).getByText("Ventas Nacional")).toBeTruthy();
    expect(screen.queryByText("¿Y el lado brasilero?")).toBeNull();
    expect(screen.queryByText("Eso va aparte. Sale USD 65 por persona.")).toBeNull();
    expect(screen.queryByText("Dale. ¿Con cuánto reservo?")).toBeNull();

    fireEvent.click(screen.getAllByText("Laura Díaz")[0]);
    const otherReply = screen.getByText("Dale, lo reviso y te digo.").closest("article");
    expect(otherReply).not.toBeNull();
    expect(within(otherReply!).getByText("Ventas Exterior")).toBeTruthy();

    // Nada del contenido de relleno anterior sobrevive.
    expect(screen.queryByText("Cliente 1")).toBeNull();
    expect(screen.queryByText("Ventas 1")).toBeNull();
    expect(screen.queryByText("Sin información adicional.")).toBeNull();
    expect(screen.queryByText(/Julián|Camila|Rocío/)).toBeNull();

    const attach = screen.getByRole("button", { name: "Adjuntar" });
    expect(getComputedStyle(attach).display).toBe("inline-flex");
    expect(getComputedStyle(attach).alignItems).toBe("center");

    expect(screen.queryByRole("heading", { name: "Bandeja de entrada" })).toBeNull();
    expect(screen.queryAllByRole("img")).toHaveLength(0);
    expect(screen.getByLabelText("Filtrar por línea")).toBeTruthy();
    expect(screen.getByRole("tablist", { name: "Filtrar conversaciones" })).toBeTruthy();
  });

  it("cierra un control ejecutando la acción, no tildando", () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );

    // El control está abierto y el aviso todavía no salió por la conversación.
    expect(screen.getAllByText("Vencimiento de DNI o pasaporte").length).toBeGreaterThan(0);
    expect(screen.queryByText(/vence antes del regreso del viaje/)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Avisar al pasajero" }));

    // El mensaje salió, y por eso —no por un tilde— el control quedó cerrado.
    expect(screen.getByText(/vence antes del regreso del viaje/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Avisar al pasajero" })).toBeNull();

    // Por el renglón y no por el texto: Diego también tiene un documento por
    // vencer, así que el mismo título aparece en su fila de la bandeja.
    const casilla = screen.getByRole("checkbox", { name: "Control de reserva: Vencimiento de DNI o pasaporte" });
    expect(casilla.getAttribute("aria-checked")).toBe("true");
    expect(within(casilla.closest("li")!).getByText(/en la conversación/)).toBeTruthy();

    // Sin el crítico abierto, la fila pasa a mostrar el siguiente pendiente.
    expect(screen.getAllByText("El pax aceptó los términos y condiciones").length).toBeGreaterThan(0);
  });

  it("oculta y vuelve a abrir el panel de operación", () => {
    render(
      <TooltipProvider>
        <MirrorScreen />
      </TooltipProvider>,
    );

    const panel = screen.getByRole("complementary", { name: "Operación" });
    const body = panel.closest(".mirror-body");
    fireEvent.click(screen.getByRole("button", { name: "Ocultar operación" }));

    expect(screen.queryByRole("complementary", { name: "Operación" })).toBeNull();
    expect(document.body.contains(panel)).toBe(true);
    expect(panel.getAttribute("aria-hidden")).toBe("true");
    expect(panel.hasAttribute("inert")).toBe(true);
    expect(body?.classList.contains("operation-collapsed")).toBe(true);

    const reopen = screen.getByRole("button", { name: "Ver operación" });
    expect(reopen.closest(".thread-header")).not.toBeNull();
    expect(reopen.getAttribute("data-variant")).toBe("default");

    fireEvent.click(screen.getAllByText("Martín Gómez")[0]);
    expect(screen.queryByRole("complementary", { name: "Operación" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Ver operación" }));

    expect(screen.getByRole("complementary", { name: "Operación" })).toBeTruthy();
    expect(body?.classList.contains("operation-collapsed")).toBe(false);
  });
});
