import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { TripPanel } from "./TripPanel";
import type { TripDetail } from "./types";

afterEach(cleanup);

const trip: TripDetail = {
  id: "trip-1", number: 10482, customerId: "customer-1", status: "cotizado",
  destination: "Iguazú", departAt: "2026-09-12T00:00:00Z", returnAt: "2026-09-15T00:00:00Z",
  createdAt: "2026-08-20T12:00:00Z", passengerCount: 0, sellerName: "Ana Gil",
  customer: { id: "customer-1", name: "Ana Pérez", phoneNumber: "+5491126126437", customerSince: "2026-08-20", contactIds: ["contact-1"] },
  originCountry: "AR", itinerary: [], requirements: [],
  passengers: [], payment: null,
  controls: [
    { id: "normal", itemId: "normal", stage: "carga", closing: "manual", title: "Revisar nombres", critical: false, status: "abierto", dueAt: "2026-08-19", evidence: [] },
    { id: "critical", itemId: "critical", stage: "control", closing: "accion", title: "Avisar por pasaporte", actionLabel: "Avisar al pasajero", message: "Tu pasaporte vence antes del regreso.", critical: true, status: "abierto", dueAt: "2026-08-25", evidence: [] },
  ],
};

const noop = {
  onRunControl: vi.fn(), onCloseControl: vi.fn(), onReopenControl: vi.fn(),
  onAddPassenger: vi.fn(), onSavePayment: vi.fn(),
};

const pasaporte = { id: "d-1", passengerId: "p-1", type: "Pasaporte" as const, country: "AR", number: "AAA111", expiresAt: "2026-09-13" };

const withPassenger: TripDetail = {
  ...trip,
  passengers: [{
    id: "p-1", tripId: "trip-1", firstName: "Ana", lastName: "Pérez", kind: "adulto", isHolder: true,
    residenceCountry: "AR", travelDocumentId: "d-1", documents: [pasaporte], permits: [],
  }],
};

const countries = [
  { code: "AR", name: "Argentina" }, { code: "ES", name: "España" }, { code: "IT", name: "Italia" },
  { code: "US", name: "Estados Unidos" }, { code: "PA", name: "Panamá" },
];

const tab = (name: string) => screen.getByRole("tab", { name: new RegExp(`^${name}`) });
const openPassenger = () => {
  fireEvent.click(tab("Pasajeros"));
  fireEvent.click(screen.getByRole("button", { name: "Abrir Pérez, Ana" }));
};
const openEditor = () => fireEvent.click(screen.getByRole("button", { name: "Editar viaje y cliente" }));

// --- Resumen ---------------------------------------------------------------

it("prioriza la excepción crítica y pliega el resto", () => {
  render(<TripPanel trip={trip} {...noop} />);

  expect(screen.getByText("Lo que sigue").nextElementSibling?.textContent).toContain("Avisar por pasaporte");
  expect(screen.queryByText("Revisar nombres")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "1 control más" }));
  expect(screen.getByText("Revisar nombres")).toBeTruthy();
});

it("los datos del viaje se leen sin formularios abiertos", () => {
  render(<TripPanel trip={trip} countries={countries} {...noop} />);

  expect(screen.queryAllByRole("textbox")).toHaveLength(0);
  expect(screen.getByText("sáb 12/09/2026")).toBeTruthy();
  expect(screen.getByText("Sin destinos")).toBeTruthy();
  expect(screen.getByText("Sin cotizar")).toBeTruthy();
  expect(screen.getByText("+54 9 11 2612-6437")).toBeTruthy();
  expect(screen.getByText("Iguazú · vende Ana Gil")).toBeTruthy();
});

it("las pestañas dicen cuántos pasajeros, qué falta y cuántos controles hay abiertos", () => {
  const conFalta: TripDetail = { ...withPassenger, requirements: [
    { passengerId: "p-1", country: "US", key: "visa", status: "falta", confirmable: false, text: "Visa" },
    { passengerId: "p-1", country: "US", key: "estadia", status: "verificar", confirmable: true, text: "Estadía" },
  ] };
  render(<TripPanel trip={conFalta} {...noop} />);

  expect(tab("Pasajeros").textContent).toBe("Pasajeros 1");
  expect(tab("Requisitos").textContent).toBe("Requisitos 1");
  expect(tab("Controles").textContent).toBe("Controles 2");
  expect(tab("Pagos").textContent).toBe("Pagos");
});

it("la acción precarga el composer y no cierra el control", () => {
  const run = vi.fn();
  const close = vi.fn();
  render(<TripPanel trip={trip} {...noop} onRunControl={run} onCloseControl={close} />);

  fireEvent.click(screen.getByRole("button", { name: "Avisar al pasajero" }));

  expect(run).toHaveBeenCalledWith(trip.controls[1]);
  expect(close).not.toHaveBeenCalled();
});

// --- Encabezado -------------------------------------------------------------

it("el estado se cambia tocando su etiqueta, con los cinco estados en orden", async () => {
  const user = userEvent.setup();
  const onUpdateTrip = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={{ ...trip, status: "consulta" }} onUpdateTrip={onUpdateTrip} {...noop} />);

  await user.click(screen.getByRole("button", { name: "Estado del viaje: Consulta" }));
  const options = screen.getAllByRole("menuitemradio").map((option) => option.textContent);
  expect(options).toEqual(["Consulta", "Cotizado", "Confirmado", "En viaje", "Cerrado"]);
  await user.click(screen.getByRole("menuitemradio", { name: "Confirmado" }));

  await waitFor(() => expect(onUpdateTrip).toHaveBeenCalledWith({ status: "confirmado" }));
});

it("con varios viajes, el número del viaje abre el menú para cambiar", async () => {
  const user = userEvent.setup();
  const onSelectTrip = vi.fn();
  const onNewTrip = vi.fn();
  const other = { ...trip, id: "trip-0", number: 10401, destination: "Salta" };
  render(<TripPanel trip={trip} trips={[trip, other]} onSelectTrip={onSelectTrip} onNewTrip={onNewTrip} {...noop} />);

  await user.click(screen.getByRole("button", { name: "Viaje #10482, cambiar de viaje" }));
  await user.click(screen.getByRole("menuitemradio", { name: /Viaje #10401/ }));
  expect(onSelectTrip).toHaveBeenCalledWith("trip-0");
  fireEvent.click(screen.getByRole("button", { name: "Nuevo viaje" }));
  expect(onNewTrip).toHaveBeenCalled();
});

// --- Editar viaje y cliente -------------------------------------------------

it("editar guarda lo que cambió del viaje y del cliente con un solo botón", async () => {
  const onUpdateTrip = vi.fn().mockResolvedValue(undefined);
  const onUpdateCustomer = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={trip} sellers={[{ id: "u-1", name: "Ana Gil" }]} onUpdateTrip={onUpdateTrip} onUpdateCustomer={onUpdateCustomer} {...noop} />);
  openEditor();

  fireEvent.change(screen.getByLabelText("Salida"), { target: { value: "2026-12-10" } });
  fireEvent.change(screen.getByLabelText("Vendedor"), { target: { value: "u-1" } });
  fireEvent.change(screen.getByLabelText("Mail"), { target: { value: "ana@example.com" } });
  expect(screen.getByText("Cambiaste las fechas, el vendedor y el cliente")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

  await waitFor(() => expect(onUpdateTrip).toHaveBeenCalledWith({ departAt: "2026-12-10", returnAt: "2026-09-15", sellerUserId: "u-1" }));
  await waitFor(() => expect(onUpdateCustomer).toHaveBeenCalledWith({ email: "ana@example.com" }));
  expect(await screen.findByRole("button", { name: "Editar viaje y cliente" })).toBeTruthy();
});

it("si una parte no se guarda, lo dice y deja lo demás guardado", async () => {
  const onUpdateTrip = vi.fn().mockResolvedValue(undefined);
  const onUpdateCustomer = vi.fn().mockRejectedValue(new Error("ya hay otro cliente con ese mail"));
  render(<TripPanel trip={trip} onUpdateTrip={onUpdateTrip} onUpdateCustomer={onUpdateCustomer} {...noop} />);
  openEditor();

  fireEvent.change(screen.getByLabelText("Regreso"), { target: { value: "2026-09-20" } });
  fireEvent.change(screen.getByLabelText("Mail"), { target: { value: "ana@ejemplo.com" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("No se guardó el cliente: ya hay otro cliente con ese mail");
  expect(alert.textContent).toContain("Sí quedó guardado las fechas");
});

it("volver con cambios sin guardar pregunta antes de descartarlos", () => {
  render(<TripPanel trip={trip} onUpdateTrip={vi.fn()} {...noop} />);
  openEditor();

  fireEvent.change(screen.getByLabelText("Salida"), { target: { value: "2026-12-10" } });
  fireEvent.click(screen.getByRole("button", { name: "Volver" }));
  expect(screen.getByText("¿Descartar los cambios?")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Seguir editando" }));
  expect((screen.getByLabelText("Salida") as HTMLInputElement).value).toBe("2026-12-10");

  fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));
  fireEvent.click(screen.getByRole("button", { name: "Descartar" }));
  expect(screen.getByRole("button", { name: "Editar viaje y cliente" })).toBeTruthy();
});

it("el recorrido se carga como sale de, destinos y escalas, con las fechas por país plegadas", async () => {
  const onSaveItinerary = vi.fn().mockResolvedValue(undefined);
  const onUpdateTrip = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={trip} countries={countries} onSaveItinerary={onSaveItinerary} onUpdateTrip={onUpdateTrip} {...noop} />);
  openEditor();

  fireEvent.click(screen.getByRole("button", { name: "Agregar destino" }));
  fireEvent.change(screen.getByLabelText("Destino"), { target: { value: "ES" } });
  fireEvent.click(screen.getByRole("button", { name: "Agregar destino" }));
  fireEvent.change(screen.getByLabelText("Destino 2"), { target: { value: "IT" } });
  fireEvent.click(screen.getByRole("button", { name: "Agregar escala" }));
  fireEvent.change(screen.getByLabelText("Escala"), { target: { value: "PA" } });
  // Lo elegido se ve como una lista con su «✕».
  expect(screen.getByRole("button", { name: "Quitar Destino 1" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Fechas en cada país" }));
  fireEvent.change(screen.getByLabelText("Llega a España"), { target: { value: "2026-09-12" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

  await waitFor(() => expect(onSaveItinerary).toHaveBeenCalledWith({ originCountry: "AR", itinerary: [
    { country: "ES", role: "destino", arriveAt: "2026-09-12" }, { country: "IT", role: "destino" }, { country: "PA", role: "escala" },
  ] }));
  // Las fechas del viaje no cambiaron: no se tocan.
  expect(onUpdateTrip).not.toHaveBeenCalled();
});

it("cambiar de dónde sale se guarda con los países", async () => {
  const onSaveItinerary = vi.fn().mockResolvedValue(undefined);
  const conPais: TripDetail = { ...trip, itinerary: [{ country: "ES", role: "destino" }] };
  render(<TripPanel trip={conPais} countries={countries} onSaveItinerary={onSaveItinerary} {...noop} />);
  expect(screen.getByText("España")).toBeTruthy();
  openEditor();

  expect((screen.getByLabelText("Sale de") as HTMLSelectElement).value).toBe("AR");
  fireEvent.change(screen.getByLabelText("Sale de"), { target: { value: "US" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

  await waitFor(() => expect(onSaveItinerary).toHaveBeenCalledWith({ originCountry: "US", itinerary: [{ country: "ES", role: "destino" }] }));
});

it("si el servidor rechaza el origen, lo dice", async () => {
  const onSaveItinerary = vi.fn().mockRejectedValue(new Error("el viaje sale de España, no puede ser también destino"));
  const conPais: TripDetail = { ...trip, itinerary: [{ country: "ES", role: "destino" }] };
  render(<TripPanel trip={conPais} countries={countries} onSaveItinerary={onSaveItinerary} {...noop} />);
  openEditor();

  fireEvent.change(screen.getByLabelText("Sale de"), { target: { value: "ES" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

  expect((await screen.findByRole("alert")).textContent).toContain("no puede ser también destino");
});

// --- Controles ----------------------------------------------------------------

it("los controles van en dos grupos y los plazos se leen en castellano", () => {
  const conHecho: TripDetail = { ...trip, controls: [...trip.controls, {
    ...trip.controls[0], id: "done", title: "Cliente cargado", status: "cerrado",
    evidence: [{ id: "e-1", closedByName: "Juan", closedAt: "2026-10-03T15:00:00Z" }],
  }] };
  render(<TripPanel trip={conHecho} {...noop} />);
  fireEvent.click(tab("Controles"));

  expect(screen.getByText("Por hacer · 2")).toBeTruthy();
  expect(screen.getByText("Hechos · 1")).toBeTruthy();
  expect(screen.getByText("Atrasado desde el 19/08")).toBeTruthy();
  expect(screen.getByText("Importante")).toBeTruthy();
  expect(screen.getByText(/^Cerró Juan · 03\/10 a las/)).toBeTruthy();
  expect(screen.queryByText(/2026-08-19/)).toBeNull();
});

it("un control por acción que cerró el dato lo dice y no se reabre a mano", () => {
  const byData: TripDetail = { ...trip, controls: [{
    id: "doc", itemId: "control-vto-doc", stage: "control", closing: "accion", title: "Vencimiento de DNI o pasaporte",
    actionLabel: "Avisar al pasajero", critical: true, status: "cerrado", dueAt: "2026-08-27", evidence: [],
  }] };
  render(<TripPanel trip={byData} {...noop} />);
  fireEvent.click(tab("Controles"));

  expect(screen.getByText("Se completó con el dato cargado")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Reabrir" })).toBeNull();
});

// --- Pasajeros ----------------------------------------------------------------

it("un viaje vacío enseña qué cargar", () => {
  render(<TripPanel trip={trip} {...noop} />);
  fireEvent.click(tab("Pasajeros"));

  expect(screen.getByText("Cargá los pasajeros para empezar")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Agregar pasajero" })).toBeTruthy();
});

it("un pasajero dice con qué documento viaja y si vence antes del regreso", () => {
  render(<TripPanel trip={withPassenger} {...noop} />);
  fireEvent.click(tab("Pasajeros"));

  const card = screen.getByRole("listitem", { name: "Pérez, Ana" });
  expect(within(card).getByText("Titular")).toBeTruthy();
  expect(within(card).getByText(/Viaja con Pasaporte AAA111/)).toBeTruthy();
  expect(within(card).getByText("Vence el 13/09/2026, antes del regreso")).toBeTruthy();
});

it("un menor dice su edad a la salida", () => {
  const menor: TripDetail = { ...withPassenger, passengers: [{ ...withPassenger.passengers[0], isHolder: false, kind: "menor", birthDate: "2017-03-01" }] };
  render(<TripPanel trip={menor} {...noop} />);
  fireEvent.click(tab("Pasajeros"));

  expect(screen.getByText("Menor · 9 años")).toBeTruthy();
});

it("el documento del pasajero se completa después", async () => {
  const onUpdateDocument = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={withPassenger} countries={countries} onUpdatePassenger={vi.fn()} onUpdateDocument={onUpdateDocument} {...noop} />);
  openPassenger();
  fireEvent.click(screen.getByRole("button", { name: "Editar Pasaporte AAA111" }));

  fireEvent.change(screen.getByLabelText("Vence"), { target: { value: "2030-01-01" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar documento" }));

  await waitFor(() => expect(onUpdateDocument).toHaveBeenCalledWith("d-1", { expiresAt: "2030-01-01" }));
});

it("con varios documentos, el sistema elige con cuál viaja hasta que alguien elige a mano", async () => {
  const onUpdatePassenger = vi.fn().mockResolvedValue(undefined);
  const dni = { id: "d-2", passengerId: "p-1", type: "DNI" as const, country: "AR", number: "30111222", expiresAt: "2031-05-01" };
  const auto: TripDetail = { ...withPassenger, passengers: [{ ...withPassenger.passengers[0], documents: [pasaporte, dni] }] };
  const { rerender } = render(<TripPanel trip={auto} countries={countries} onUpdatePassenger={onUpdatePassenger} {...noop} />);
  openPassenger();

  expect(screen.getByText(/El sistema elige el que sirve para todos los países/)).toBeTruthy();
  expect(screen.getByText(/lo eligió el sistema/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Que lo elija el sistema" })).toBeNull();

  const manual: TripDetail = { ...auto, passengers: [{ ...auto.passengers[0], travelDocumentManual: true }] };
  rerender(<TripPanel trip={manual} countries={countries} onUpdatePassenger={onUpdatePassenger} {...noop} />);
  expect(screen.getByText(/elegido a mano/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Que lo elija el sistema" }));

  await waitFor(() => expect(onUpdatePassenger).toHaveBeenCalledWith("p-1", { travelDocumentId: "" }));
});

it("un pasajero con doble nacionalidad carga su otro pasaporte y elige con cuál viaja", async () => {
  const onAddDocument = vi.fn().mockResolvedValue(undefined);
  const onUpdatePassenger = vi.fn().mockResolvedValue(undefined);
  const trip2: TripDetail = { ...withPassenger, passengers: [{ ...withPassenger.passengers[0], documents: [pasaporte,
    { id: "d-2", passengerId: "p-1", type: "Pasaporte", country: "IT", number: "YA222", expiresAt: "2031-05-01" }] }] };
  render(<TripPanel trip={trip2} countries={countries} onUpdatePassenger={onUpdatePassenger} onAddDocument={onAddDocument} {...noop} />);
  openPassenger();

  fireEvent.click(screen.getByLabelText("Viaja con Pasaporte YA222"));
  await waitFor(() => expect(onUpdatePassenger).toHaveBeenCalledWith("p-1", { travelDocumentId: "d-2" }));

  fireEvent.click(screen.getByRole("button", { name: "Agregar documento" }));
  fireEvent.change(screen.getByLabelText("Tipo"), { target: { value: "Pasaporte" } });
  fireEvent.change(screen.getByLabelText("País del documento"), { target: { value: "ES" } });
  fireEvent.change(screen.getByLabelText("Número"), { target: { value: "PAE123" } });
  fireEvent.change(screen.getByLabelText("Emitido"), { target: { value: "2022-03-01" } });
  fireEvent.change(screen.getByLabelText("Vence"), { target: { value: "2032-03-01" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar documento" }));

  await waitFor(() => expect(onAddDocument).toHaveBeenCalledWith("p-1", {
    type: "Pasaporte", country: "ES", number: "PAE123", issuedAt: "2022-03-01", expiresAt: "2032-03-01",
  }));
});

it("las visas que ya tiene se cargan con su país y vencimiento", async () => {
  const onAddPermit = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={withPassenger} countries={countries} onUpdatePassenger={vi.fn()} onAddPermit={onAddPermit} {...noop} />);
  openPassenger();

  fireEvent.click(screen.getByRole("button", { name: "Agregar visa o autorización" }));
  fireEvent.change(screen.getByLabelText("Qué es"), { target: { value: "visa" } });
  fireEvent.change(screen.getByLabelText("País que la otorgó"), { target: { value: "US" } });
  fireEvent.change(screen.getByLabelText("Detalle"), { target: { value: "B1/B2" } });
  fireEvent.change(screen.getByLabelText("Vence la visa"), { target: { value: "2032-01-10" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar visa" }));

  await waitFor(() => expect(onAddPermit).toHaveBeenCalledWith("p-1", {
    kind: "visa", country: "US", label: "B1/B2", expiresAt: "2032-01-10",
  }));
});

it("la fecha de nacimiento y la residencia se cargan con «Editar datos»", async () => {
  const onUpdatePassenger = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={withPassenger} countries={countries} onUpdatePassenger={onUpdatePassenger} {...noop} />);
  openPassenger();
  fireEvent.click(screen.getByRole("button", { name: "Editar datos" }));

  fireEvent.change(screen.getByLabelText("Nacimiento"), { target: { value: "2010-04-02" } });
  fireEvent.change(screen.getByLabelText("Residencia"), { target: { value: "ES" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar pasajero" }));

  await waitFor(() => expect(onUpdatePassenger).toHaveBeenCalledWith("p-1", { birthDate: "2010-04-02", residenceCountry: "ES" }));
});

it("quitar un pasajero no pregunta: ofrece deshacer", async () => {
  const onDeletePassenger = vi.fn().mockResolvedValue(undefined);
  const onAddPassenger = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={withPassenger} {...noop} onDeletePassenger={onDeletePassenger} onAddPassenger={onAddPassenger} />);
  openPassenger();

  fireEvent.click(screen.getByRole("button", { name: "Quitar pasajero" }));
  await waitFor(() => expect(onDeletePassenger).toHaveBeenCalledWith(withPassenger.passengers[0]));
  fireEvent.click(await screen.findByRole("button", { name: "Deshacer" }));

  await waitFor(() => expect(onAddPassenger).toHaveBeenCalledWith({
    firstName: "Ana", lastName: "Pérez", kind: "adulto", isHolder: true, residenceCountry: "AR",
    documentType: "Pasaporte", documentCountry: "AR", documentNumber: "AAA111", documentExpiresAt: "2026-09-13",
  }));
});

// --- Requisitos ---------------------------------------------------------------

it("los requisitos tienen su pestaña", () => {
  const conRequisitos: TripDetail = { ...withPassenger, itinerary: [{ country: "ES", role: "destino" }], requirements: [
    { passengerId: "p-1", country: "ES", key: "vigencia", status: "falta", confirmable: false,
      text: "El Pasaporte vence el 13/09/2026 y España pide que dure hasta el 15/12/2026" },
  ] };
  render(<TripPanel trip={conRequisitos} countries={countries} {...noop} />);
  fireEvent.click(tab("Requisitos"));

  expect(screen.getByText(/España pide que dure hasta el 15\/12\/2026/)).toBeTruthy();
});

it("«Cargar visa» lleva a Pasajeros con el pasajero abierto y la visa por cargar", () => {
  const sinVisa: TripDetail = { ...withPassenger, itinerary: [{ country: "US", role: "destino" }], requirements: [
    { passengerId: "p-1", country: "US", key: "visa", status: "falta", confirmable: false, text: "Visa" },
  ] };
  render(<TripPanel trip={sinVisa} countries={countries} onAddPermit={vi.fn()} {...noop} />);
  fireEvent.click(tab("Requisitos"));
  fireEvent.click(screen.getByRole("button", { name: "Cargar visa" }));

  expect(tab("Pasajeros").getAttribute("aria-selected")).toBe("true");
  expect(screen.getByRole("button", { name: "Cerrar Pérez, Ana" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Guardar visa" })).toBeTruthy();
});

// --- Pagos --------------------------------------------------------------------

it("sin cotización, Pagos ofrece cargarla y el pago se guarda entero", async () => {
  const onSavePayment = vi.fn().mockResolvedValue(undefined);
  render(<TripPanel trip={trip} {...noop} onSavePayment={onSavePayment} />);
  fireEvent.click(tab("Pagos"));
  expect(screen.getByText("Todavía no hay cotización")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Cargar cotización" }));

  fireEvent.change(screen.getByLabelText("Total USD"), { target: { value: "3200" } });
  fireEvent.change(screen.getByLabelText("Cambio del día"), { target: { value: "1450,50" } });
  fireEvent.change(screen.getByLabelText("Total en pesos"), { target: { value: "4.641.600" } });
  fireEvent.change(screen.getByLabelText("Forma de pago"), { target: { value: "Transferencia" } });
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
  fireEvent.click(tab("Pagos"));
  fireEvent.click(screen.getByRole("button", { name: "Cargar cotización" }));

  fireEvent.change(screen.getByLabelText("Total USD"), { target: { value: "tres mil" } });
  fireEvent.click(screen.getByRole("button", { name: "Guardar pago" }));

  expect(screen.getByRole("alert").textContent).toContain("Total USD");
  expect(onSavePayment).not.toHaveBeenCalled();
});

it("con cotización, Pagos se lee como plata: total, cobrado y lo que falta", () => {
  const cotizado: TripDetail = { ...trip, payment: {
    tripId: "trip-1", totalUsd: 2480, rate: 1230, totalArs: 3050400, method: "Transferencia",
    depositUsd: 500, depositPaidAt: "2026-10-15T13:00:00Z", nonRefundableUsd: 300,
  } };
  render(<TripPanel trip={cotizado} {...noop} />);
  fireEvent.click(tab("Pagos"));

  expect(screen.getByText("USD 2.480")).toBeTruthy();
  expect(screen.getByText("$ 3.050.400 al cambio de $ 1.230")).toBeTruthy();
  expect(screen.getByText("USD 500 cobrada")).toBeTruthy();
  expect(screen.getByText("USD 1.980")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Editar pago" })).toBeTruthy();
});
