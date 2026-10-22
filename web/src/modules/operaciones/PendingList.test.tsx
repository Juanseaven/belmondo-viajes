import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { PendingList } from "./PendingList";
import type { PendingRow } from "./types";

afterEach(cleanup);

// 2026-10-20 es martes.
const today = "2026-10-20";

const base = {
  status: "confirmado" as const, passengerCount: 2, customerName: "Agus Alonso",
  destination: "Cancún", departAt: "2026-12-10T00:00:00Z", conversationId: "conv-1", more: 0,
};

const rows: PendingRow[] = [
  { ...base, id: "t-1", number: 10482, group: "atrasado", more: 2,
    task: { kind: "sena", task: "Cobrar la seña de USD 800", dueAt: "2026-10-18", critical: true } },
  { ...base, id: "t-2", number: 10451, customerName: "Coba", destination: "Iguazú", departAt: "2026-10-26",
    group: "semana", task: { kind: "control", task: "Enviar vouchers y T&C", dueAt: "2026-10-22", critical: false } },
  { ...base, id: "t-3", number: 10440, customerName: "Beto", destination: "", departAt: "2026-11-01", group: "salen", conversationId: undefined },
];

it("agrupa por urgencia, cuenta los viajes y dice el plazo en palabras", () => {
  render(<PendingList rows={rows} today={today} onOpen={vi.fn()} />);

  const late = screen.getByRole("region", { name: /^Atrasado/ });
  expect(within(late).getByText("1 viaje")).toBeTruthy();
  expect(within(late).getByText("Cobrar la seña de USD 800")).toBeTruthy();
  expect(within(late).getByText("Agus Alonso")).toBeTruthy();
  expect(within(late).getByText("#10482 · Cancún · sale el jue 10 dic")).toBeTruthy();
  expect(within(late).getByText("Hace 2 días")).toBeTruthy();
  expect(within(late).getByText("era para el 18/10")).toBeTruthy();
  expect(within(late).getByText("y 2 cosas más")).toBeTruthy();

  const week = screen.getByRole("region", { name: /^Esta semana/ });
  expect(within(week).getByText("Jueves")).toBeTruthy();
  expect(within(week).getByText("jue 22/10")).toBeTruthy();
  expect(within(week).queryByText(/cosas? más/)).toBeNull();
});

it("un viaje que sale pronto y está al día dice «Todo listo» y cuándo sale", () => {
  render(<PendingList rows={rows} today={today} onOpen={vi.fn()} />);

  const soon = screen.getByRole("region", { name: /^Salen en los próximos 14 días/ });
  expect(within(soon).getByText("1 viaje, todo listo")).toBeTruthy();
  expect(within(soon).getByText("Todo listo")).toBeTruthy();
  expect(within(soon).getByText("#10440 · sin destino · sale el dom 1 nov · 2 pasajeros")).toBeTruthy();
  expect(within(soon).getByText("En 12 días")).toBeTruthy();
});

it("abrir el chat pasa el viaje; sin conversación, abre el viaje", () => {
  const onOpen = vi.fn();
  render(<PendingList rows={rows} today={today} onOpen={onOpen} />);

  fireEvent.click(within(screen.getByRole("region", { name: /^Atrasado/ })).getByRole("button", { name: "Abrir chat" }));
  expect(onOpen).toHaveBeenCalledWith(rows[0]);

  const noChat = within(screen.getByRole("region", { name: /^Salen en los próximos 14 días/ })).getByRole("button", { name: "Abrir viaje" });
  expect(noChat.getAttribute("title")).toContain("no tiene WhatsApp");
  fireEvent.click(noChat);
  expect(onOpen).toHaveBeenLastCalledWith(rows.find((row) => !row.conversationId));
});

it("sin pendientes, dice qué va a aparecer", () => {
  render(<PendingList rows={[]} today={today} onOpen={vi.fn()} />);
  expect(screen.getByText("No hay nada pendiente.")).toBeTruthy();
  expect(screen.getByText(/próximos 14 días van a aparecer acá/)).toBeTruthy();
});
