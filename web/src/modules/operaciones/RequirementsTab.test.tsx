import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { RequirementsTab } from "./RequirementsTab";
import type { Country, Requirement, TripDetail } from "./types";

afterEach(cleanup);

const countries: Country[] = [
  { code: "AR", name: "Argentina" }, { code: "CA", name: "Canadá" }, { code: "US", name: "Estados Unidos" },
  { code: "JP", name: "Japón" },
];

const base: TripDetail = {
  id: "trip-1", number: 10482, customerId: "customer-1", status: "cotizado", destination: "Canadá",
  departAt: "2026-12-10", returnAt: "2026-12-24", createdAt: "2026-08-20T12:00:00Z", passengerCount: 2,
  customer: { id: "customer-1", name: "Juan Pérez", customerSince: "2026-08-20", contactIds: ["contact-1"] },
  itinerary: [{ country: "CA", role: "destino" }, { country: "US", role: "escala" }],
  passengers: [
    { id: "maria", tripId: "trip-1", firstName: "María", lastName: "Pérez", kind: "adulto", isHolder: true, residenceCountry: "AR", documents: [], permits: [] },
    { id: "tomas", tripId: "trip-1", firstName: "Tomás", lastName: "Pérez", kind: "menor", isHolder: false, residenceCountry: "AR", documents: [], permits: [] },
  ],
  payment: null, controls: [], requirements: [],
};

const req = (r: Partial<Requirement>): Requirement => ({ key: "visa", status: "cumple", text: "", confirmable: false, ...r });

it("sin itinerario ni pasajeros enseña qué cargar", () => {
  render(<RequirementsTab trip={{ ...base, itinerary: [], passengers: [] }} countries={countries} />);
  expect(screen.getByText(/Cargá el país de destino y los pasajeros/)).toBeTruthy();
});

it("agrupa por pasajero y país y dice cada requisito con palabras", () => {
  const trip = { ...base, requirements: [
    req({ passengerId: "maria", country: "CA", key: "visa", status: "falta", text: "Canadá pide visa y no está cargada",
      sources: [{ source: "wikipedia", url: "https://en.wikipedia.org/w/index.php?oldid=1", quote: "Visa required" }] }),
    req({ passengerId: "maria", country: "US", role: "escala", key: "transito", status: "cumple", text: "Estados Unidos no pide visa: la escala no necesita visa de tránsito" }),
    req({ passengerId: "tomas", key: "menor", status: "falta", confirmable: true, text: "Es menor: necesita la autorización de viaje si no viaja con ambos padres" }),
  ] };
  render(<RequirementsTab trip={trip} countries={countries} />);

  const maria = screen.getByRole("region", { name: "María Pérez" });
  expect(within(maria).getByText("Canadá")).toBeTruthy();
  expect(within(maria).getByText("Estados Unidos · escala")).toBeTruthy();
  expect(within(maria).getByText("Canadá pide visa y no está cargada")).toBeTruthy();
  expect(within(maria).getByText("Falta")).toBeTruthy();
  const link = within(maria).getByRole("link", { name: "Fuente: Wikipedia" });
  expect(link.getAttribute("href")).toBe("https://en.wikipedia.org/w/index.php?oldid=1");

  const tomas = screen.getByRole("region", { name: "Tomás Pérez" });
  expect(within(tomas).getByText(/autorización de viaje/)).toBeTruthy();
});

it("solo lo confirmable que no cumple se confirma", async () => {
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  const verificar = req({ passengerId: "maria", country: "JP", key: "visa", status: "verificar", confirmable: true, text: "No se sabe qué visa pide Japón: las fuentes no coinciden" });
  const trip = { ...base, itinerary: [{ country: "JP", role: "destino" as const }], requirements: [
    verificar,
    req({ passengerId: "maria", country: "JP", key: "vigencia", status: "falta", missingData: true, text: "Falta cargar el vencimiento del Pasaporte" }),
  ] };
  render(<RequirementsTab trip={trip} countries={countries} onConfirm={onConfirm} />);

  const buttons = screen.getAllByRole("button", { name: /^Confirmar/ });
  expect(buttons).toHaveLength(1);
  fireEvent.click(buttons[0]);
  await waitFor(() => expect(onConfirm).toHaveBeenCalledWith(verificar));
  expect(screen.getByText(/el aviso no cierra «Requisitos migratorios»/)).toBeTruthy();
});

it("lo confirmado dice quién lo confirmó", () => {
  const trip = { ...base, requirements: [req({ passengerId: "tomas", key: "menor", status: "cumple", confirmable: true,
    confirmed: true, confirmedByName: "Ana Gil", confirmedAt: "2026-10-20T15:00:00Z", text: "Es menor: necesita la autorización de viaje" })] };
  render(<RequirementsTab trip={trip} countries={countries} onConfirm={vi.fn()} />);

  expect(screen.getByText("Confirmado por Ana Gil · 20 oct")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /^Confirmar/ })).toBeNull();
});

it("mientras se buscan los requisitos de un país lo dice una sola vez", () => {
  const buscando = (key: string) => req({ passengerId: "maria", country: "CA", key, status: "verificar", confirmable: true,
    text: `No se sabe ${key} de Canadá: buscando requisitos` });
  const trip = { ...base, itinerary: [{ country: "CA", role: "destino" as const }],
    requirements: [buscando("visa"), buscando("documento"), buscando("vacunas")] };
  render(<RequirementsTab trip={trip} countries={countries} onConfirm={vi.fn()} />);

  expect(screen.getByText("Buscando los requisitos de Canadá…")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /^Confirmar/ })).toBeNull();
});
