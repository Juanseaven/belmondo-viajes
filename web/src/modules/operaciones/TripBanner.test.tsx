import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { TripBanner } from "./TripBanner";
import type { Control, TripDetail } from "./types";

afterEach(cleanup);

const draft: TripDetail = {
  id: "trip-draft", number: 10483, customerId: "customer-1", status: "cotizado",
  destination: "", createdAt: "2026-08-21", passengerCount: 1,
  customer: { id: "customer-1", name: "Ana", customerSince: "2026-08-21", contactIds: ["contact-1"] },
  passengers: [{ id: "p-1", tripId: "trip-draft", firstName: "Ana", lastName: "Pérez", kind: "adulto", isHolder: false, residenceCountry: "AR", documents: [], permits: [] }],
  payment: null, controls: [], originCountry: "AR", itinerary: [], requirements: [],
};

it("resume una operación parcial sin inventar destino ni fechas", () => {
  render(<TripBanner trip={draft} />);

  expect(screen.getByText("Viaje #10483 · Datos por completar")).toBeTruthy();
  expect(screen.getByText("1 pasajero")).toBeTruthy();
  expect(screen.queryByText(/atrasado/)).toBeNull();
});

it("cuenta los controles importantes atrasados", () => {
  const control = (id: string, critical: boolean, dueAt: string): Control => ({
    id, itemId: id, title: id, critical, dueAt, closing: "manual", stage: "control", status: "abierto", evidence: [],
  });
  const trip = { ...draft, departAt: "2026-10-29", returnAt: "2026-11-07", controls: [
    control("a", true, "2026-10-19"), control("b", true, "2026-10-20"), control("c", false, "2026-10-01"),
  ] };

  render(<TripBanner trip={trip} from="2026-10-21" />);

  expect(screen.getByText("29 oct – 7 nov")).toBeTruthy();
  expect(screen.getByText("2 controles atrasados")).toBeTruthy();
});
