import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { operacionesApi } from "./api";
import { TripSidebar } from "./TripSidebar";
import type { TripDetail } from "./types";
import type { useTrips } from "./useTrips";

vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return { ...actual, operacionesApi: {
    ...actual.operacionesApi, sellers: vi.fn(), createTrip: vi.fn(), deletePassenger: vi.fn(), updateCustomer: vi.fn(),
  } };
});

afterEach(() => { cleanup(); vi.clearAllMocks(); });
beforeEach(() => {
  vi.mocked(operacionesApi.sellers).mockResolvedValue({ items: [{ id: "u-1", name: "Ana Gil" }] });
});

const active: TripDetail = {
  id: "trip-1", number: 10482, customerId: "customer-1", status: "cotizado", destination: "Iguazú",
  createdAt: "2026-08-20T12:00:00Z", passengerCount: 1,
  customer: { id: "customer-1", name: "Ana Pérez", customerSince: "2026-08-20", contactIds: ["contact-1"] },
  passengers: [{ id: "p-1", tripId: "trip-1", firstName: "Ana", lastName: "Pérez", kind: "adulto", isHolder: true }],
  payment: null, controls: [],
};

function tripsState(overrides: Partial<ReturnType<typeof useTrips>> = {}): ReturnType<typeof useTrips> {
  return {
    byContact: {}, contactTrips: [active], active, loading: false, error: undefined,
    reload: vi.fn().mockResolvedValue(undefined), select: vi.fn(), ...overrides,
  };
}

it("ofrece los vendedores de la agencia en el encabezado", async () => {
  render(<TripSidebar contactId="contact-1" contactName="Ana" trips={tripsState()} onRunControl={vi.fn()} />);

  expect(await screen.findByRole("option", { name: "Ana Gil" })).toBeTruthy();
});

it("un viaje nuevo sobre otro existente se crea aparte y queda elegido", async () => {
  const trips = tripsState();
  vi.mocked(operacionesApi.createTrip).mockResolvedValue({ ...active, id: "trip-2", number: 10483, destination: "" });
  render(<TripSidebar contactId="contact-1" contactName="Ana" trips={trips} onRunControl={vi.fn()} />);

  fireEvent.click(screen.getByRole("button", { name: "Nuevo viaje" }));
  expect(screen.getByText("Operación nueva")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Destino"), { target: { value: "Salta" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar viaje" }));

  await waitFor(() => expect(operacionesApi.createTrip).toHaveBeenCalledWith({ contactId: "contact-1", destination: "Salta" }));
  expect(trips.select).toHaveBeenCalledWith("trip-2");
  await waitFor(() => expect(screen.queryByText("Operación nueva")).toBeNull());
});

it("quitar un pasajero lo borra y recarga la ficha", async () => {
  const trips = tripsState();
  vi.mocked(operacionesApi.deletePassenger).mockResolvedValue(undefined);
  render(<TripSidebar contactId="contact-1" contactName="Ana" trips={trips} onRunControl={vi.fn()} />);

  fireEvent.click(screen.getByRole("tab", { name: "Pasajeros" }));
  fireEvent.click(screen.getByRole("button", { name: "Editar Pérez, Ana" }));
  fireEvent.click(screen.getByRole("button", { name: "Quitar pasajero" }));

  await waitFor(() => expect(operacionesApi.deletePassenger).toHaveBeenCalledWith("p-1"));
  expect(trips.reload).toHaveBeenCalled();
});

it("el cliente se guarda sobre el cliente del viaje", async () => {
  vi.mocked(operacionesApi.updateCustomer).mockResolvedValue(active.customer);
  render(<TripSidebar contactId="contact-1" contactName="Ana" trips={tripsState()} onRunControl={vi.fn()} />);

  fireEvent.change(screen.getByLabelText("Mail"), { target: { value: "ana@example.com" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar cliente" }));

  await waitFor(() => expect(operacionesApi.updateCustomer).toHaveBeenCalledWith("customer-1", { email: "ana@example.com" }));
});
