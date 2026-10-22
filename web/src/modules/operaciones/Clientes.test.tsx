import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { operacionesApi } from "./api";
import { BringPassengersDialog } from "./BringPassengersDialog";
import { ClienteScreen } from "./ClienteScreen";
import { ClientesScreen } from "./ClientesScreen";
import { ControlRow } from "./ControlsTab";
import { CustomerDialog } from "./CustomerDialog";
import type { Companion, Control, CustomerDetail, CustomerRow } from "./types";

const replace = vi.fn();
const push = vi.fn();
let search = "";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push }),
  useSearchParams: () => new URLSearchParams(search),
  useParams: () => ({ id: "cliente-1" }),
}));
vi.mock("@/kernel/api/useSocket", () => ({ useSocket: () => undefined }));
vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return {
    ...actual,
    operacionesApi: {
      ...actual.operacionesApi, customers: vi.fn(), customer: vi.fn(), similarCustomers: vi.fn(), trip: vi.fn(),
      sellers: vi.fn(), countries: vi.fn(), tripCompanions: vi.fn(),
    },
  };
});

const marta: CustomerRow = {
  id: "cliente-1", name: "Marta Pérez", customerSince: "2024-03-10", phoneNumber: "11 4788-2210", email: "marta@mail.com",
  chatCount: 0, tripCount: 3, lastDestination: "Japón", lastActivity: "2026-10-22T10:00:00Z",
  openTrip: { id: "t-12", number: 12, status: "cotizado", destination: "Japón", departAt: "2026-11-02" },
};
const lucia: CustomerRow = {
  id: "cliente-2", name: "Lucía Méndez", customerSince: "2023-08-01", contactPhone: "+5491160127788", conversationId: "conv-2",
  chatCount: 2, tripCount: 1, lastActivity: "2026-10-21T10:00:00Z",
};

const detail = (overrides: Partial<CustomerDetail> = {}): CustomerDetail => ({
  id: "cliente-1", name: "Marta Pérez", customerSince: "2024-03-10", phoneNumber: "11 4788-2210", contactIds: [],
  hasChat: false, contacts: [], companions: [],
  trips: [
    { id: "t-12", number: 12, customerId: "cliente-1", status: "cotizado", destination: "Japón", customerName: "Marta Pérez", passengerCount: 3 },
    { id: "t-6", number: 6, customerId: "cliente-1", status: "cerrado", destination: "Perú", customerName: "Marta Pérez", passengerCount: 2, conversationId: "conv-9" },
  ],
  ...overrides,
});

beforeEach(() => {
  search = "";
  vi.mocked(operacionesApi.customers).mockResolvedValue({ items: [marta, lucia], total: 2, byChip: { todos: 2, "con-viaje": 1, "sin-viajes": 0, "sin-whatsapp": 1 } });
  vi.mocked(operacionesApi.customer).mockResolvedValue(detail());
  vi.mocked(operacionesApi.similarCustomers).mockResolvedValue({ items: [] });
  vi.mocked(operacionesApi.sellers).mockResolvedValue({ items: [] });
  vi.mocked(operacionesApi.countries).mockResolvedValue({ items: [] });
  vi.mocked(operacionesApi.tripCompanions).mockResolvedValue({ items: [] });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

it("la lista consulta con la URL, muestra el contacto y los chips cambian la URL", async () => {
  search = "q=perez&filtro=sin-whatsapp";
  render(<ClientesScreen />);

  await waitFor(() => expect(operacionesApi.customers).toHaveBeenCalledWith(expect.objectContaining({ q: "perez", filtro: "sin-whatsapp", orden: "movimiento" })));
  const row = (await screen.findByText("Marta Pérez")).closest("tr")!;
  expect(within(row).getByText("11 4788-2210")).toBeTruthy();
  expect(within(row).getByText("marta@mail.com · sin WhatsApp")).toBeTruthy();
  expect(within(row).getByText("#12 Japón · sale lun 2 nov")).toBeTruthy();
  const luciaRow = screen.getByText("Lucía Méndez").closest("tr")!;
  expect(within(luciaRow).getByText("+54 9 11 6012-7788")).toBeTruthy();
  expect(within(luciaRow).getByText("2 chats")).toBeTruthy();

  expect(screen.getByRole("button", { name: "Sin WhatsApp 1" }).getAttribute("aria-pressed")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Con viaje abierto 1" }));
  expect(replace).toHaveBeenLastCalledWith("/clientes?q=perez&filtro=con-viaje", { scroll: false });

  fireEvent.click(row);
  expect(push).toHaveBeenCalledWith("/clientes/cliente-1");
});

it("un viaje sin WhatsApp se abre en el panel y uno con chat va al chat", async () => {
  render(<ClienteScreen />);

  const japon = (await screen.findByText("Japón")).closest("tr")!;
  fireEvent.click(japon);
  expect(replace).toHaveBeenCalledWith("/clientes/cliente-1?viaje=t-12", { scroll: false });

  fireEvent.click(screen.getByText("Perú").closest("tr")!);
  expect(push).toHaveBeenCalledWith("/mirror?c=conv-9&viaje=t-6");

  expect(screen.getByText("No tiene chats")).toBeTruthy();
  expect(screen.getByText(/Si escribe desde 11 4788-2210/)).toBeTruthy();
});

it("con ?viaje= el panel del viaje se abre sobre la página", async () => {
  search = "viaje=t-12";
  vi.mocked(operacionesApi.trip).mockResolvedValue({
    id: "t-12", number: 12, customerId: "cliente-1", status: "cotizado", destination: "Japón", createdAt: "2026-10-01",
    passengerCount: 0, customer: { ...detail(), hasChat: false }, originCountry: "AR", itinerary: [], requirements: [],
    passengers: [], payment: null, controls: [],
  });
  render(<ClienteScreen />);
  expect(await screen.findByRole("heading", { name: "Viaje #12" })).toBeTruthy();
});

it("«Marcar hecho» reemplaza la acción solo cuando el cliente no tiene chat", () => {
  const control: Control = {
    id: "c-1", itemId: "carga-vouchers", title: "Vouchers enviados", critical: false, closing: "accion",
    stage: "carga", actionLabel: "Enviar vouchers", status: "abierto", evidence: [],
  };
  const onRun = vi.fn();
  const onClose = vi.fn();
  const { rerender } = render(<ul><ControlRow control={control} onRun={onRun} onClose={onClose} onReopen={vi.fn()} /></ul>);
  expect(screen.getByRole("button", { name: "Enviar vouchers" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Marcar hecho" })).toBeNull();

  rerender(<ul><ControlRow control={control} onRun={onRun} onClose={onClose} onReopen={vi.fn()} markByHand /></ul>);
  fireEvent.click(screen.getByRole("button", { name: "Marcar hecho" }));
  expect(onClose).toHaveBeenCalledWith(control);
  expect(onRun).not.toHaveBeenCalled();
});

it("el alta avisa si el cliente parece repetido y lleva a su ficha", async () => {
  vi.mocked(operacionesApi.similarCustomers).mockResolvedValue({ items: [{ id: "cliente-2", name: "Lucía Méndez", tripCount: 3, conversationId: "conv-2" }] });
  const onOpenCustomer = vi.fn();
  render(<CustomerDialog onClose={vi.fn()} onOpenCustomer={onOpenCustomer} />);

  fireEvent.change(screen.getByLabelText("Nombre y apellido"), { target: { value: "Lucia Mendez" } });
  const go = await screen.findByRole("button", { name: "Ir a su ficha" }, { timeout: 2000 });
  expect(operacionesApi.similarCustomers).toHaveBeenLastCalledWith(expect.objectContaining({ nombre: "Lucia Mendez" }));
  expect(screen.getByText("3 viajes · con WhatsApp")).toBeTruthy();
  fireEvent.click(go);
  expect(onOpenCustomer).toHaveBeenCalledWith("cliente-2");
});

it("traer pasajeros marca los del último viaje y deshabilita los que ya están", async () => {
  const base = { kind: "adulto" as const, tripCount: 1, tripId: "t-12", tripNumber: 12, destination: "Japón", permitCount: 0 };
  const companions: Companion[] = [
    { ...base, passengerId: "p-1", firstName: "Marta", lastName: "Pérez", selected: true, permitCount: 1,
      document: { id: "d-1", passengerId: "p-1", type: "Pasaporte", country: "AR", expiresAt: "2031-08-01" } },
    { ...base, passengerId: "p-2", firstName: "Tomás", lastName: "Gómez", inTrip: true },
    { ...base, passengerId: "p-3", firstName: "Elena", lastName: "Pérez", tripId: "t-6", tripNumber: 6, destination: "Perú" },
  ];
  const onBring = vi.fn().mockResolvedValue(undefined);
  render(<BringPassengersDialog companions={companions} onClose={vi.fn()} onBring={onBring} />);

  expect((screen.getByLabelText("Pérez, Marta") as HTMLInputElement).checked).toBe(true);
  expect((screen.getByLabelText("Gómez, Tomás") as HTMLInputElement).disabled).toBe(true);
  expect(screen.getByText("Ya está en este viaje")).toBeTruthy();
  expect((screen.getByLabelText("Pérez, Elena") as HTMLInputElement).checked).toBe(false);
  expect(screen.getByText(/Pasaporte AR vence 08\/2031/)).toBeTruthy();

  fireEvent.click(screen.getByLabelText("Pérez, Elena"));
  fireEvent.click(screen.getByRole("button", { name: "Traer 2 pasajeros" }));
  await waitFor(() => expect(onBring).toHaveBeenCalledWith(["p-1", "p-3"]));
});
