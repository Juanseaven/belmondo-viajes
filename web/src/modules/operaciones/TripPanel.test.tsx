import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { TripPanel } from "./TripPanel";
import type { TripDetail } from "./types";

afterEach(cleanup);

const trip: TripDetail = {
  id: "trip-1", number: 10482, customerId: "customer-1", status: "cotizado",
  destination: "Iguazú", departAt: "2026-09-12T00:00:00Z", returnAt: "2026-09-15T00:00:00Z",
  createdAt: "2026-08-20T12:00:00Z", passengerCount: 0,
  customer: { id: "customer-1", name: "Ana Pérez", customerSince: "2026-08-20", contactIds: ["contact-1"] },
  passengers: [], payment: null,
  controls: [
    { id: "normal", itemId: "normal", stage: "carga", closing: "manual", title: "Revisar nombres", critical: false, status: "abierto", dueAt: "2026-08-19", evidence: [] },
    { id: "critical", itemId: "critical", stage: "control", closing: "accion", title: "Avisar por pasaporte", actionLabel: "Avisar al pasajero", message: "Tu pasaporte vence antes del regreso.", critical: true, status: "abierto", dueAt: "2026-08-25", evidence: [] },
  ],
};

it("prioriza la excepción crítica y pliega el resto", () => {
  render(<TripPanel trip={trip} onRunControl={vi.fn()} onCloseControl={vi.fn()} onReopenControl={vi.fn()} onAddPassenger={vi.fn()} onSavePayment={vi.fn()} />);

  expect(screen.getByText("Lo que sigue").nextElementSibling?.textContent).toContain("Avisar por pasaporte");
  expect(screen.getByText("Ver 1 control restante")).toBeTruthy();
  expect(screen.queryByText("Revisar nombres")).toBeNull();
});

it("un viaje vacío enseña qué cargar", () => {
  render(<TripPanel trip={trip} onRunControl={vi.fn()} onCloseControl={vi.fn()} onReopenControl={vi.fn()} onAddPassenger={vi.fn()} onSavePayment={vi.fn()} />);
  fireEvent.click(screen.getByRole("tab", { name: "Pasajeros" }));

  expect(screen.getByText("Cargá los pasajeros para empezar")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Agregar pasajero" })).toBeTruthy();
});

it("la acción precarga el composer y no cierra el control", () => {
  const run = vi.fn();
  const close = vi.fn();
  render(<TripPanel trip={trip} onRunControl={run} onCloseControl={close} onReopenControl={vi.fn()} onAddPassenger={vi.fn()} onSavePayment={vi.fn()} />);

  fireEvent.click(screen.getByRole("button", { name: "Avisar al pasajero" }));

  expect(run).toHaveBeenCalledWith(trip.controls[1]);
  expect(close).not.toHaveBeenCalled();
});

it("permite completar el destino de una operación ya creada", async () => {
  const onUpdateTrip = vi.fn().mockResolvedValue(undefined);
  const props = {
    trip: { ...trip, destination: "", departAt: undefined, returnAt: undefined },
    onUpdateTrip,
    onRunControl: vi.fn(), onCloseControl: vi.fn(), onReopenControl: vi.fn(),
    onAddPassenger: vi.fn(), onSavePayment: vi.fn(),
  } as Parameters<typeof TripPanel>[0];

  render(<TripPanel {...props} />);
  fireEvent.change(screen.getByLabelText("Destino"), { target: { value: "Salta" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar viaje" }));

  await waitFor(() => expect(onUpdateTrip).toHaveBeenCalledWith({ destination: "Salta" }));
  expect(screen.queryByText(/0001/)).toBeNull();
});
