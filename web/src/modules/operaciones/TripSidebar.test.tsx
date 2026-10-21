import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import type { Conversation } from "@/modules/mirror/api";
import { operacionesApi } from "./api";
import { TripSidebar } from "./TripSidebar";
import type { TripDetail } from "./types";
import type { useTrips } from "./useTrips";

vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return { ...actual, operacionesApi: {
    ...actual.operacionesApi, sellers: vi.fn(), countries: vi.fn(), createTrip: vi.fn(), deletePassenger: vi.fn(), updateCustomer: vi.fn(),
  } };
});

afterEach(() => { cleanup(); vi.clearAllMocks(); vi.unstubAllGlobals(); });
beforeEach(() => {
  class LoadedImage extends EventTarget {
    complete = true;
    naturalWidth = 96;
    crossOrigin: string | null = null;
    referrerPolicy = "";
    src = "";
  }
  vi.stubGlobal("Image", LoadedImage);
  vi.mocked(operacionesApi.sellers).mockResolvedValue({ items: [{ id: "u-1", name: "Ana Gil" }] });
  vi.mocked(operacionesApi.countries).mockResolvedValue({ items: [{ code: "AR", name: "Argentina" }, { code: "CL", name: "Chile" }] });
});

const active: TripDetail = {
  id: "trip-1", number: 10482, customerId: "customer-1", status: "cotizado", destination: "Iguazú",
  createdAt: "2026-08-20T12:00:00Z", passengerCount: 1,
  customer: { id: "customer-1", name: "Ana Pérez", customerSince: "2026-08-20", contactIds: ["contact-1"] },
  passengers: [{ id: "p-1", tripId: "trip-1", firstName: "Ana", lastName: "Pérez", kind: "adulto", isHolder: true, residenceCountry: "AR", documents: [], permits: [] }],
  payment: null, controls: [], originCountry: "AR", itinerary: [], requirements: [],
};

function tripsState(overrides: Partial<ReturnType<typeof useTrips>> = {}): ReturnType<typeof useTrips> {
  return {
    byContact: {}, contactTrips: [active], active, loading: false, error: undefined,
    reload: vi.fn().mockResolvedValue(undefined), select: vi.fn(), ...overrides,
  };
}

const conversation: Pick<Conversation, "contactId" | "contactName" | "avatarUrl" | "phoneNumber" | "instanceLabel" | "isGroup"> = {
  contactId: "contact-1", contactName: "Ana", phoneNumber: "+5491126126437", instanceLabel: "Juanse", isGroup: false,
};

it("sin viaje abierto muestra el contacto y ofrece crear uno, con su foto de WhatsApp", () => {
  render(
    <TripSidebar
      conversation={{ ...conversation, avatarUrl: "https://example.test/ana.jpg" }}
      trips={tripsState({ active: undefined, contactTrips: [] })}
      onRunControl={vi.fn()}
    />,
  );

  expect(screen.getByRole("img", { name: "Ana" }).getAttribute("src")).toBe("https://example.test/ana.jpg");
  expect(screen.getByText("+54 9 11 2612-6437")).toBeTruthy();
  expect(screen.getByText("Este contacto no tiene viajes")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Nuevo viaje" })).toBeTruthy();
  expect(operacionesApi.createTrip).not.toHaveBeenCalled();
});

it("con viajes cerrados, los ofrece para abrirlos", () => {
  const trips = tripsState({ active: undefined, contactTrips: [{ ...active, status: "cerrado" }] });
  render(<TripSidebar conversation={conversation} trips={trips} onRunControl={vi.fn()} />);

  expect(screen.getByText("Este contacto no tiene viajes abiertos")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Viaje #10482/ }));
  expect(trips.select).toHaveBeenCalledWith("trip-1");
});

it("un viaje nuevo sobre otro existente se crea aparte y queda elegido", async () => {
  const trips = tripsState();
  vi.mocked(operacionesApi.createTrip).mockResolvedValue({ ...active, id: "trip-2", number: 10483, destination: "" });
  render(<TripSidebar conversation={conversation} trips={trips} onRunControl={vi.fn()} />);

  fireEvent.click(screen.getByRole("button", { name: "Nuevo viaje" }));
  expect(screen.getByRole("form", { name: "Nuevo viaje" })).toBeTruthy();
  await screen.findAllByRole("option", { name: "Chile" });
  fireEvent.change(screen.getByLabelText("Destino"), { target: { value: "CL" } });
  fireEvent.click(screen.getByRole("button", { name: "Crear viaje" }));

  await waitFor(() => expect(operacionesApi.createTrip).toHaveBeenCalledWith({
    contactId: "contact-1", originCountry: "AR", itinerary: [{ country: "CL", role: "destino" }],
  }));
  expect(trips.select).toHaveBeenCalledWith("trip-2");
  await waitFor(() => expect(screen.queryByRole("form", { name: "Nuevo viaje" })).toBeNull());
});

it("los vendedores de la agencia se eligen al editar el viaje", async () => {
  render(<TripSidebar conversation={conversation} trips={tripsState()} onRunControl={vi.fn()} />);

  fireEvent.click(screen.getByRole("button", { name: "Editar viaje y cliente" }));
  expect(await screen.findByRole("option", { name: "Ana Gil" })).toBeTruthy();
});

it("quitar un pasajero lo borra y recarga la ficha", async () => {
  const trips = tripsState();
  vi.mocked(operacionesApi.deletePassenger).mockResolvedValue(undefined);
  render(<TripSidebar conversation={conversation} trips={trips} onRunControl={vi.fn()} />);

  fireEvent.click(screen.getByRole("tab", { name: /^Pasajeros/ }));
  fireEvent.click(screen.getByRole("button", { name: "Abrir Pérez, Ana" }));
  fireEvent.click(screen.getByRole("button", { name: "Quitar pasajero" }));

  await waitFor(() => expect(operacionesApi.deletePassenger).toHaveBeenCalledWith("p-1"));
  expect(trips.reload).toHaveBeenCalled();
});

it("el cliente se guarda sobre el cliente del viaje", async () => {
  vi.mocked(operacionesApi.updateCustomer).mockResolvedValue(active.customer);
  render(<TripSidebar conversation={conversation} trips={tripsState()} onRunControl={vi.fn()} />);

  fireEvent.click(screen.getByRole("button", { name: "Editar viaje y cliente" }));
  fireEvent.change(screen.getByLabelText("Mail"), { target: { value: "ana@example.com" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

  await waitFor(() => expect(operacionesApi.updateCustomer).toHaveBeenCalledWith("customer-1", { email: "ana@example.com" }));
});
