import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { operacionesApi } from "./api";
import { useTrips } from "./useTrips";

let socketHandler: (() => void) | undefined;
vi.mock("@/kernel/api/useSocket", () => ({ useSocket: (_name: string, handler: () => void) => { socketHandler = handler; } }));
vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return { ...actual, operacionesApi: { ...actual.operacionesApi, inbox: vi.fn(), trip: vi.fn() } };
});

const summary = { id: "trip-1", number: 1, customerId: "customer-1", status: "cotizado" as const, destination: "Salta", departAt: "2026-10-01", returnAt: "2026-10-05", createdAt: "2026-08-20", passengerCount: 0 };
const detail = { ...summary, customer: { id: "customer-1", name: "Ana", customerSince: "2026-08-20", contactIds: ["contact-1"] }, passengers: [], payment: null, controls: [] };

function Harness() {
  const trips = useTrips(["contact-1", "contact-2"], "contact-1");
  return <div>{trips.byContact["contact-1"]?.destination ?? "sin viaje"}|{trips.active?.customer.name ?? "sin detalle"}</div>;
}

beforeEach(() => {
  vi.mocked(operacionesApi.inbox).mockResolvedValue({ items: [{ contactId: "contact-1", trip: summary }, { contactId: "contact-2" }] });
  vi.mocked(operacionesApi.trip).mockResolvedValue(detail);
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

it("enriquece por contacto y recarga el detalle ante un evento", async () => {
  render(<Harness />);
  expect(await screen.findByText("Salta|Ana")).toBeTruthy();
  act(() => socketHandler?.());
  await waitFor(() => expect(operacionesApi.inbox).toHaveBeenCalledTimes(2));
  expect(operacionesApi.trip).toHaveBeenCalledTimes(2);
});
