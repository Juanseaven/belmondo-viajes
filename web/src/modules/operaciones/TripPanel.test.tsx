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

// --- Ficha completa ---------------------------------------------------------

const noop = {
  onRunControl: vi.fn(), onCloseControl: vi.fn(), onReopenControl: vi.fn(),
  onAddPassenger: vi.fn(), onSavePayment: vi.fn(),
};

const withPassenger: TripDetail = {
  ...trip,
  passengers: [{
    id: "p-1", tripId: "trip-1", firstName: "Ana", lastName: "Pérez", kind: "adulto",
    isHolder: true, documentType: "Pasaporte", documentNumber: "AAA111", documentExpiresAt: "2026-09-13",
  }],
};

it("las fechas se leen en castellano, no en ISO", () => {
  render(<TripPanel trip={trip} {...noop} />);
  fireEvent.click(screen.getByRole("tab", { name: "Controles" }));

  expect(screen.getByText("Venció 19 ago")).toBeTruthy();
  expect(screen.queryByText(/2026-08-19/)).toBeNull();
});

it("un control cerrado dice quién lo cerró y cuándo", () => {
  const closed: TripDetail = { ...trip, controls: [{
    ...trip.controls[0], status: "cerrado",
    evidence: [{ id: "e-1", closedByName: "Juan", closedAt: "2026-10-03T15:00:00Z" }],
  }] };
  render(<TripPanel trip={closed} {...noop} />);
  fireEvent.click(screen.getByRole("tab", { name: "Controles" }));

  expect(screen.getByText("Cerrado por Juan · 3 oct")).toBeTruthy();
});

it("el estado y el vendedor se cambian desde el encabezado", async () => {
  const onUpdateTrip = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={trip} sellers={[{ id: "u-1", name: "Ana Gil" }]} onUpdateTrip={onUpdateTrip} {...noop} />);

  fireEvent.change(screen.getByLabelText("Estado del viaje"), { target: { value: "confirmado" } });
  await waitFor(() => expect(onUpdateTrip).toHaveBeenCalledWith({ status: "confirmado" }));
  fireEvent.change(screen.getByLabelText("Vendedor"), { target: { value: "u-1" } });
  await waitFor(() => expect(onUpdateTrip).toHaveBeenCalledWith({ sellerUserId: "u-1" }));
});

it("con varios viajes se puede cambiar de viaje o abrir uno nuevo", () => {
  const onSelectTrip = vi.fn();
  const onNewTrip = vi.fn();
  const other = { ...trip, id: "trip-0", number: 10401, destination: "Salta" };
  render(<TripPanel trip={trip} trips={[trip, other]} onSelectTrip={onSelectTrip} onNewTrip={onNewTrip} {...noop} />);

  fireEvent.change(screen.getByLabelText("Viaje del contacto"), { target: { value: "trip-0" } });
  expect(onSelectTrip).toHaveBeenCalledWith("trip-0");
  fireEvent.click(screen.getByRole("button", { name: "Nuevo viaje" }));
  expect(onNewTrip).toHaveBeenCalled();
});

it("el cliente se edita en el resumen", async () => {
  const onUpdateCustomer = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={trip} onUpdateCustomer={onUpdateCustomer} {...noop} />);

  fireEvent.change(screen.getByLabelText("Mail"), { target: { value: "ana@example.com" } });
  fireEvent.change(screen.getByLabelText("Teléfono"), { target: { value: "+5491112345678" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar cliente" }));

  await waitFor(() => expect(onUpdateCustomer).toHaveBeenCalledWith({ email: "ana@example.com", phoneNumber: "+5491112345678" }));
});

it("un documento que vence antes del regreso lo explica", () => {
  render(<TripPanel trip={withPassenger} {...noop} />);
  fireEvent.click(screen.getByRole("tab", { name: "Pasajeros" }));

  expect(screen.getByText("El documento vence antes del regreso")).toBeTruthy();
});

it("el documento del pasajero se completa después", async () => {
  const onUpdatePassenger = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={withPassenger} onUpdatePassenger={onUpdatePassenger} {...noop} />);
  fireEvent.click(screen.getByRole("tab", { name: "Pasajeros" }));
  fireEvent.click(screen.getByRole("button", { name: "Editar Pérez, Ana" }));

  fireEvent.change(screen.getByLabelText("Vence"), { target: { value: "2030-01-01" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar pasajero" }));

  await waitFor(() => expect(onUpdatePassenger).toHaveBeenCalledWith("p-1", { documentExpiresAt: "2030-01-01" }));
});

it("quitar un pasajero no pregunta: ofrece deshacer", async () => {
  const onDeletePassenger = vi.fn().mockResolvedValue(undefined);
  const onAddPassenger = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={withPassenger} {...noop} onDeletePassenger={onDeletePassenger} onAddPassenger={onAddPassenger} />);
  fireEvent.click(screen.getByRole("tab", { name: "Pasajeros" }));
  fireEvent.click(screen.getByRole("button", { name: "Editar Pérez, Ana" }));

  fireEvent.click(screen.getByRole("button", { name: "Quitar pasajero" }));
  await waitFor(() => expect(onDeletePassenger).toHaveBeenCalledWith(withPassenger.passengers[0]));
  fireEvent.click(await screen.findByRole("button", { name: "Deshacer" }));

  await waitFor(() => expect(onAddPassenger).toHaveBeenCalledWith({
    firstName: "Ana", lastName: "Pérez", kind: "adulto", isHolder: true,
    documentType: "Pasaporte", documentNumber: "AAA111", documentExpiresAt: "2026-09-13",
  }));
});

it("el pago completo se guarda entero y acepta coma decimal", async () => {
  const onSavePayment = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={trip} {...noop} onSavePayment={onSavePayment} />);
  fireEvent.click(screen.getByRole("tab", { name: "Pagos" }));

  fireEvent.change(screen.getByLabelText("Total USD"), { target: { value: "3200" } });
  fireEvent.change(screen.getByLabelText("Cambio del día"), { target: { value: "1450,50" } });
  fireEvent.change(screen.getByLabelText("Total en pesos"), { target: { value: "4.641.600" } });
  fireEvent.change(screen.getByLabelText("Método"), { target: { value: "Transferencia" } });
  fireEvent.change(screen.getByLabelText("Seña USD"), { target: { value: "500" } });
  fireEvent.change(screen.getByLabelText("Vence la seña"), { target: { value: "2026-09-01" } });
  fireEvent.click(screen.getByLabelText("Seña cobrada"));
  fireEvent.change(screen.getByLabelText("No reembolsable USD"), { target: { value: "150" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar pago" }));

  await waitFor(() => expect(onSavePayment).toHaveBeenCalledWith({
    totalUsd: 3200, rate: 1450.5, totalArs: 4641600, method: "Transferencia",
    depositUsd: 500, depositDueAt: "2026-09-01", depositPaid: true, nonRefundableUsd: 150,
  }));
});

it("un monto ilegible no se guarda y dice por qué", () => {
  const onSavePayment = vi.fn();
  render(<TripPanel trip={trip} {...noop} onSavePayment={onSavePayment} />);
  fireEvent.click(screen.getByRole("tab", { name: "Pagos" }));

  fireEvent.change(screen.getByLabelText("Total USD"), { target: { value: "tres mil" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar pago" }));

  expect(screen.getByRole("alert").textContent).toContain("Total USD");
  expect(onSavePayment).not.toHaveBeenCalled();
});

it("un control por acción que cerró el dato lo dice y no se reabre a mano", () => {
  const byData: TripDetail = { ...trip, controls: [{
    id: "doc", itemId: "control-vto-doc", stage: "control", closing: "accion", title: "Vencimiento de DNI o pasaporte",
    actionLabel: "Avisar al pasajero", critical: true, status: "cerrado", dueAt: "2026-08-27", evidence: [],
  }] };
  render(<TripPanel trip={byData} {...noop} />);
  fireEvent.click(screen.getByRole("tab", { name: "Controles" }));

  expect(screen.getByText("Dato cargado")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Reabrir" })).toBeNull();
});

it("el estado arranca en consulta, antes de cotizado", () => {
  render(<TripPanel trip={{ ...trip, status: "consulta" }} {...noop} />);
  const options = Array.from((screen.getByLabelText("Estado del viaje") as HTMLSelectElement).options).map((o) => o.text);
  expect(options).toEqual(["Consulta", "Cotizado", "Confirmado", "En viaje", "Cerrado"]);
  expect((screen.getByLabelText("Estado del viaje") as HTMLSelectElement).value).toBe("consulta");
});
