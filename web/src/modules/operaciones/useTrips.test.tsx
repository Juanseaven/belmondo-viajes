import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { operacionesApi } from "./api";
import { useTrips } from "./useTrips";

let socketHandler: (() => void) | undefined;
vi.mock("@/kernel/api/useSocket", () => ({ useSocket: (_name: string, handler: () => void) => { socketHandler = handler; } }));
vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return { ...actual, operacionesApi: { ...actual.operacionesApi, inbox: vi.fn(), trip: vi.fn(), trips: vi.fn() } };
});

const summary = { id: "trip-1", number: 1, customerId: "customer-1", status: "cotizado" as const, destination: "Salta", departAt: "2026-10-01", returnAt: "2026-10-05", createdAt: "2026-08-20", passengerCount: 0 };
const detail = { ...summary, customer: { id: "customer-1", name: "Ana", customerSince: "2026-08-20", contactIds: ["contact-1"] }, passengers: [], payment: null, controls: [], itinerary: [], requirements: [] };

function Harness() {
  const trips = useTrips(["contact-1", "contact-2"], "contact-1");
  return <div>{trips.byContact["contact-1"]?.destination ?? "sin viaje"}|{trips.active?.customer.name ?? "sin detalle"}</div>;
}

beforeEach(() => {
  vi.mocked(operacionesApi.inbox).mockResolvedValue({ items: [{ contactId: "contact-1", trip: summary }, { contactId: "contact-2" }] });
  vi.mocked(operacionesApi.trip).mockResolvedValue(detail);
  vi.mocked(operacionesApi.trips).mockResolvedValue({ items: [summary] });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

it("enriquece por contacto y recarga el detalle ante un evento", async () => {
  render(<Harness />);
  expect(await screen.findByText("Salta|Ana")).toBeTruthy();
  act(() => socketHandler?.());
  await waitFor(() => expect(operacionesApi.inbox).toHaveBeenCalledTimes(2));
  expect(operacionesApi.trip).toHaveBeenCalledTimes(2);
});

// Un cliente puede tener dos viajes a la vez. La bandeja elige el más reciente
// sin cerrar, pero quien atiende puede estar hablando del otro: la elección
// tiene que sobrevivir a las recargas que dispara cualquier evento del módulo.
it("permite elegir otro viaje del contacto y lo conserva al recargar", async () => {
  const older = { ...summary, id: "trip-0", number: 0, destination: "Jujuy" };
  vi.mocked(operacionesApi.trips).mockResolvedValue({ items: [summary, older] });
  vi.mocked(operacionesApi.trip).mockImplementation(async (id) => (id === "trip-0" ? { ...detail, ...older } : detail));

  function Picker() {
    const trips = useTrips(["contact-1"], "contact-1");
    return <>
      <div>{trips.contactTrips.length} viajes|{trips.active?.destination ?? "sin detalle"}</div>
      <button onClick={() => trips.select("trip-0")}>elegir</button>
    </>;
  }
  render(<Picker />);
  expect(await screen.findByText("2 viajes|Salta")).toBeTruthy();

  fireEvent.click(screen.getByText("elegir"));
  expect(await screen.findByText("2 viajes|Jujuy")).toBeTruthy();

  act(() => socketHandler?.());
  await waitFor(() => expect(operacionesApi.inbox).toHaveBeenCalledTimes(3));
  expect(screen.getByText("2 viajes|Jujuy")).toBeTruthy();
});

// Desde la pantalla de Viajes se llega al chat con un viaje concreto: la ficha
// tiene que mostrar ese, no el que elegiría la bandeja.
it("respeta el viaje que llega elegido desde la pantalla de Viajes", async () => {
  const older = { ...summary, id: "trip-0", number: 0, destination: "Jujuy" };
  vi.mocked(operacionesApi.trips).mockResolvedValue({ items: [summary, older] });
  vi.mocked(operacionesApi.trip).mockImplementation(async (id) => (id === "trip-0" ? { ...detail, ...older } : detail));

  function Preferred() {
    const trips = useTrips(["contact-1"], "contact-1", "trip-0");
    return <div>{trips.active?.destination ?? "sin detalle"}</div>;
  }
  render(<Preferred />);

  expect(await screen.findByText("Jujuy")).toBeTruthy();
  expect(operacionesApi.trip).not.toHaveBeenCalledWith("trip-1");
});
