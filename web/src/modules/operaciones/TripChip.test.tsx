import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { TripChip } from "./TripChip";
import type { TripSummary } from "./types";

afterEach(cleanup);

const trip: TripSummary = {
  id: "trip-1", number: 4, customerId: "customer-1", status: "consulta",
  destination: "Estados Unidos", departAt: "2026-10-29", returnAt: "2026-11-07",
  createdAt: "2026-10-20", passengerCount: 1,
  nextControl: { id: "control-1", itemId: "nombre", title: "Revisar orden de nombre y apellido", critical: true, closing: "manual", dueAt: "2026-10-20" },
};

it("dice el número y el destino, y si el próximo control se pasó", () => {
  render(<TripChip trip={trip} from="2026-10-21" />);

  expect(screen.getByText("#4 Estados Unidos")).toBeTruthy();
  expect(screen.getByText("Atrasado")).toBeTruthy();
});

it("sin nada atrasado, solo el viaje", () => {
  render(<TripChip trip={trip} from="2026-10-20" />);

  expect(screen.queryByText("Atrasado")).toBeNull();
});
