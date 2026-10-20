import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { defaultFilters } from "./tripFilters";
import { TripFiltersBar } from "./TripFiltersBar";
import { TripsTable } from "./TripsTable";
import type { TripRow } from "./types";

afterEach(cleanup);

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
  expect(within(first).getByText("2 pax · Juanse")).toBeTruthy();
  expect(within(first).getByText("Confirmado")).toBeTruthy();
  expect(within(first).getByText("10 dic")).toBeTruthy();
  expect(within(first).getByText("al 20 dic")).toBeTruthy();
  expect(within(first).getByText("USD 3.200")).toBeTruthy();
  expect(within(first).getByText("seña vence el jueves")).toBeTruthy();
  expect(within(first).getByText("Avisar que el documento vence")).toBeTruthy();
  expect(screen.getByText("23 viajes")).toBeTruthy();
});

it("lo que falta es una raya, y un viaje sin pendientes está al día", () => {
  table();
  const second = screen.getAllByRole("row")[2];
  expect(within(second).getAllByText("—").length).toBeGreaterThanOrEqual(3);
  expect(within(second).getByText("Al día")).toBeTruthy();
  expect(within(second).queryByText(/Sin completar/)).toBeNull();
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

it("los filtros cambian de a uno y dicen cuál está activo", () => {
  const onChange = vi.fn();
  const value = defaultFilters();
  render(<TripFiltersBar value={value} sellers={[{ id: "u-1", name: "Lucía" }]} onChange={onChange} />);

  expect(screen.getByRole("button", { name: "Abiertos" }).getAttribute("aria-pressed")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Confirmados" }));
  expect(onChange).toHaveBeenLastCalledWith({ ...value, estado: "confirmado" });

  fireEvent.change(screen.getByLabelText("Vendedor"), { target: { value: "ninguno" } });
  expect(onChange).toHaveBeenLastCalledWith({ ...value, vendedor: "ninguno" });

  fireEvent.click(screen.getByRole("button", { name: "Con algo atrasado" }));
  expect(onChange).toHaveBeenLastCalledWith({ ...value, atrasados: true });

  fireEvent.change(screen.getByLabelText("Buscar viajes"), { target: { value: "rossi" } });
  expect(onChange).toHaveBeenLastCalledWith({ ...value, q: "rossi" });
});

it("elegir fechas muestra el rango", () => {
  const onChange = vi.fn();
  const { rerender } = render(<TripFiltersBar value={defaultFilters()} sellers={[]} onChange={onChange} />);
  expect(screen.queryByLabelText("Desde")).toBeNull();

  fireEvent.change(screen.getByLabelText("Salida"), { target: { value: "rango" } });
  rerender(<TripFiltersBar value={{ ...defaultFilters(), salida: "rango" }} sellers={[]} onChange={onChange} />);
  expect(screen.getByLabelText("Desde")).toBeTruthy();
  expect(screen.getByLabelText("Hasta")).toBeTruthy();
});

it("las consultas se filtran como cualquier estado", () => {
  const onChange = vi.fn();
  render(<TripFiltersBar value={defaultFilters()} sellers={[]} onChange={onChange} />);
  fireEvent.click(screen.getByRole("button", { name: "Consultas" }));
  expect(onChange).toHaveBeenLastCalledWith({ ...defaultFilters(), estado: "consulta" });
});
