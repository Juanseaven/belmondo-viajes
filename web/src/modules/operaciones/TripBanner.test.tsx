import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { TripBanner } from "./TripBanner";
import type { TripDetail } from "./types";

afterEach(cleanup);

it("resume una operación parcial sin inventar destino ni fechas", () => {
  const trip: TripDetail = {
    id: "trip-draft", number: 10483, customerId: "customer-1", status: "cotizado",
    destination: "", createdAt: "2026-08-21", passengerCount: 1,
    customer: { id: "customer-1", name: "Ana", customerSince: "2026-08-21", contactIds: ["contact-1"] },
    passengers: [{ id: "p-1", tripId: "trip-draft", firstName: "Ana", lastName: "Pérez", kind: "adulto", isHolder: false, residenceCountry: "AR", documents: [], permits: [] }],
    payment: null, controls: [], originCountry: "AR", itinerary: [], requirements: [],
  };

  render(<TripBanner trip={trip} />);

  expect(screen.getByText("Datos por completar")).toBeTruthy();
  expect(screen.getByText("1 pax")).toBeTruthy();
});
