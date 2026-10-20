import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { TripChip } from "./TripChip";
import type { TripSummary } from "./types";

afterEach(cleanup);

it("nombra qué está atrasado", () => {
  const trip: TripSummary = {
    id: "trip-1", number: 10482, customerId: "customer-1", status: "cotizado",
    destination: "Iguazú", departAt: "2026-09-12", returnAt: "2026-09-15",
    createdAt: "2026-08-20", passengerCount: 2,
    nextControl: { id: "control-1", itemId: "visa", title: "Avisar por visa", critical: true, closing: "accion", dueAt: "2020-01-01" },
  };
  render(<TripChip trip={trip} />);

  expect(screen.getByText("Avisar por visa")).toBeTruthy();
  expect(screen.queryByText(/^Atrasado$/)).toBeNull();
});
