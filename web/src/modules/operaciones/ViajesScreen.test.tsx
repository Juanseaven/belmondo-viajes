import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { operacionesApi } from "./api";
import type { PendingRow, TripRow } from "./types";
import { ViajesScreen } from "./ViajesScreen";

const replace = vi.fn();
const push = vi.fn();
let search = "";
let socketHandler: (() => void) | undefined;

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push }),
  useSearchParams: () => new URLSearchParams(search),
}));
vi.mock("@/kernel/api/useSocket", () => ({ useSocket: (_name: string, handler: () => void) => { socketHandler = handler; } }));
vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return { ...actual, operacionesApi: { ...actual.operacionesApi, pending: vi.fn(), listTrips: vi.fn(), sellers: vi.fn() } };
});

const trip: TripRow = {
  id: "t-1", number: 10482, status: "confirmado", destination: "Cancún", customerName: "Agus Alonso",
  passengerCount: 2, conversationId: "conv-1",
};
const pending: PendingRow[] = [
  { ...trip, group: "atrasado", more: 0, task: { kind: "sena", task: "Cobrar la seña", dueAt: "2020-01-01", critical: true } },
  { ...trip, id: "t-2", number: 10483, group: "semana", more: 0, task: { kind: "control", task: "Enviar la Flight Plus", dueAt: "2099-01-01", critical: false } },
  { ...trip, id: "t-3", number: 10484, group: "salen", more: 0 },
];

beforeEach(() => {
  search = "";
  vi.mocked(operacionesApi.pending).mockResolvedValue({ items: pending });
  vi.mocked(operacionesApi.listTrips).mockResolvedValue({ items: [trip], total: 1 });
  vi.mocked(operacionesApi.sellers).mockResolvedValue({ items: [] });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

it("abre en Pendientes y cuenta solo lo que pide algo", async () => {
  render(<ViajesScreen />);

  expect(await screen.findByText("Cobrar la seña")).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Pendientes 2" }).getAttribute("aria-selected")).toBe("true");
  expect(operacionesApi.listTrips).not.toHaveBeenCalled();
});

it("cambiar de pestaña queda en la URL", async () => {
  render(<ViajesScreen />);
  fireEvent.click(screen.getByRole("tab", { name: "Todos los viajes" }));
  expect(replace).toHaveBeenCalledWith("/viajes?tab=todos", { scroll: false });
});

it("Todos los viajes consulta con los filtros de la URL y los cambia ahí", async () => {
  search = "tab=todos&estado=confirmado";
  render(<ViajesScreen />);

  await waitFor(() => expect(operacionesApi.listTrips).toHaveBeenCalledWith(expect.objectContaining({ estado: "confirmado", orden: "salida" })));
  expect(await screen.findByText("Agus Alonso")).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "Cotizados" }));
  expect(replace).toHaveBeenLastCalledWith("/viajes?tab=todos&estado=cotizado", { scroll: false });
});

it("abrir el chat lleva a la conversación con ese viaje elegido", async () => {
  render(<ViajesScreen />);
  fireEvent.click((await screen.findAllByRole("button", { name: "Abrir chat" }))[0]);
  expect(push).toHaveBeenCalledWith("/mirror?c=conv-1&viaje=t-1");
});

it("se recarga ante un evento del módulo y al volver a la pestaña", async () => {
  render(<ViajesScreen />);
  await screen.findByText("Cobrar la seña");

  act(() => socketHandler?.());
  await waitFor(() => expect(operacionesApi.pending).toHaveBeenCalledTimes(2));

  act(() => { document.dispatchEvent(new Event("visibilitychange")); });
  await waitFor(() => expect(operacionesApi.pending).toHaveBeenCalledTimes(3));
});
