import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { defaultFilters } from "./tripFilters";
import { TripFiltersBar } from "./TripFiltersBar";
import { TripsTable } from "./TripsTable";
import type { TripRow } from "./types";

afterEach(cleanup);

// 2026-10-20 es martes.
const today = "2026-10-20";

const rows: TripRow[] = [
  { id: "t-1", number: 10482, status: "confirmado", destination: "Cancún", departAt: "2026-12-10T00:00:00Z",
    returnAt: "2026-12-20T00:00:00Z", customerName: "Agus Alonso", passengerCount: 2, sellerName: "Juanse",
    totalUsd: 3200, depositUsd: 500, depositDueAt: "2026-10-22", conversationId: "conv-1",
    next: { kind: "control", task: "Avisar que el documento vence", dueAt: "2026-10-11", critical: true } },
  { id: "t-2", number: 10440, status: "cotizado", destination: "", customerName: "Beto", passengerCount: 0, conversationId: undefined },
];

function table(overrides: Partial<Parameters<typeof TripsTable>[0]> = {}) {
  const props = { rows, total: 23, sort: "salida" as const, onSort: vi.fn(), onOpen: vi.fn(), onClear: vi.fn(), today, ...overrides };
  render(<TripsTable {...props} />);
  return props;
}

it("cada columna tiene el dato arriba y el detalle en gris abajo", () => {
  table();
  const first = screen.getAllByRole("row")[1];
  expect(within(first).getByText("Agus Alonso")).toBeTruthy();
  expect(within(first).getByText("2 pasajeros · Juanse")).toBeTruthy();
  expect(within(first).getByText("Confirmado").className).toContain("is-confirmado");
  expect(within(first).getByText("jue 10/12")).toBeTruthy();
  expect(within(first).getByText("vuelve el 20/12")).toBeTruthy();
  expect(within(first).getByText("USD 3.200")).toBeTruthy();
  expect(within(first).getByText("seña vence el jueves")).toBeTruthy();
  expect(within(first).getByText("Avisar que el documento vence")).toBeTruthy();
  expect(within(first).getByText("Hace 9 días")).toBeTruthy();
  expect(screen.getByText("23 viajes · ordenados por salida")).toBeTruthy();
});

it("lo que falta dice qué falta, y un viaje sin pendientes está al día", () => {
  table();
  const second = screen.getAllByRole("row")[2];
  expect(within(second).getByText("Sin destino")).toBeTruthy();
  expect(within(second).getByText("Sin fecha")).toBeTruthy();
  expect(within(second).getByText("Sin cotizar")).toBeTruthy();
  expect(within(second).getByText("Sin pasajeros · Sin asignar")).toBeTruthy();
  expect(within(second).getByText("Al día")).toBeTruthy();
  expect(within(second).queryByText("—")).toBeNull();
});

it("clic en una fila abre su chat; sin conversación no hace nada", () => {
  const props = table();
  fireEvent.click(within(screen.getAllByRole("row")[1]).getByText("Agus Alonso"));
  expect(props.onOpen).toHaveBeenCalledWith(rows[0]);
  fireEvent.click(within(screen.getAllByRole("row")[2]).getByText("Beto"));
  expect(props.onOpen).toHaveBeenCalledTimes(1);
});

it("se ordena desde el encabezado", () => {
  const props = table();
  fireEvent.click(screen.getByRole("button", { name: /Total/ }));
  expect(props.onSort).toHaveBeenCalledWith("total");
  expect(screen.getByRole("columnheader", { name: /Salida/ }).getAttribute("aria-sort")).toBe("ascending");
});

it("sin resultados ofrece limpiar los filtros", () => {
  const props = table({ rows: [], total: 0 });
  expect(screen.getByText("Ningún viaje coincide")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Limpiar filtros" }));
  expect(props.onClear).toHaveBeenCalled();
});

it("los chips de estado dicen cuántos viajes hay en cada uno", () => {
  render(<TripFiltersBar value={defaultFilters()} sellers={[]} byStatus={{ consulta: 3, cotizado: 2, confirmado: 2, "en viaje": 1, cerrado: 14 }} onChange={vi.fn()} />);

  expect(screen.getByRole("button", { name: "Abiertos 8" }).getAttribute("aria-pressed")).toBe("true");
  expect(screen.getByRole("button", { name: "Consultas 3" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Cerrados 14" })).toBeTruthy();
});

it("los filtros cambian de a uno y los desplegables dicen lo elegido", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  const value = defaultFilters();
  render(<TripFiltersBar value={value} sellers={[{ id: "u-1", name: "Lucía" }]} onChange={onChange} />);

  fireEvent.click(screen.getByRole("button", { name: "Confirmados" }));
  expect(onChange).toHaveBeenLastCalledWith({ ...value, estado: "confirmado" });

  await user.click(screen.getByRole("button", { name: "Vendedor: Todos" }));
  await user.click(screen.getByRole("menuitemradio", { name: "Sin asignar" }));
  expect(onChange).toHaveBeenLastCalledWith({ ...value, vendedor: "ninguno" });

  fireEvent.click(screen.getByRole("button", { name: "Con algo atrasado" }));
  expect(onChange).toHaveBeenLastCalledWith({ ...value, atrasados: true });

  fireEvent.change(screen.getByLabelText("Buscar viajes"), { target: { value: "rossi" } });
  expect(onChange).toHaveBeenLastCalledWith({ ...value, q: "rossi" });
});

it("elegir fechas muestra el rango", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  const { rerender } = render(<TripFiltersBar value={defaultFilters()} sellers={[]} onChange={onChange} />);
  expect(screen.queryByLabelText("Desde")).toBeNull();

  await user.click(screen.getByRole("button", { name: "Salida: Cualquier fecha" }));
  await user.click(screen.getByRole("menuitemradio", { name: "Elegir fechas…" }));
  expect(onChange).toHaveBeenLastCalledWith({ ...defaultFilters(), salida: "rango" });
  rerender(<TripFiltersBar value={{ ...defaultFilters(), salida: "rango" }} sellers={[]} onChange={onChange} />);
  expect(screen.getByRole("button", { name: "Salida: Elegir fechas…" })).toBeTruthy();
  expect(screen.getByLabelText("Desde")).toBeTruthy();
  expect(screen.getByLabelText("Hasta")).toBeTruthy();
});

it("las consultas se filtran como cualquier estado", () => {
  const onChange = vi.fn();
  render(<TripFiltersBar value={defaultFilters()} sellers={[]} onChange={onChange} />);
  fireEvent.click(screen.getByRole("button", { name: "Consultas" }));
  expect(onChange).toHaveBeenLastCalledWith({ ...defaultFilters(), estado: "consulta" });
});
