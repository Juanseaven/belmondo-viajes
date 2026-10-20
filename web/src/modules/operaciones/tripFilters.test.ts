import { expect, it } from "vitest";

import { filtersFromParams, filtersToParams, toListParams } from "./tripFilters";

it("sin parámetros: abiertos, por salida, sin más filtros", () => {
  const filters = filtersFromParams(new URLSearchParams());
  expect(filters).toEqual({ q: "", estado: "abiertos", salida: "", desde: "", hasta: "", vendedor: "", atrasados: false, orden: "salida" });
  expect(filtersToParams(filters).toString()).toBe("");
});

it("ida y vuelta por la URL conserva todo", () => {
  const params = new URLSearchParams("tab=todos&q=cancun&estado=confirmado&salida=rango&desde=2026-11-01&hasta=2026-11-30&vendedor=u-1&atrasados=1&orden=total");
  const filters = filtersFromParams(params);
  expect(filters).toEqual({ q: "cancun", estado: "confirmado", salida: "rango", desde: "2026-11-01", hasta: "2026-11-30", vendedor: "u-1", atrasados: true, orden: "total" });
  expect(filtersToParams(filters).toString()).toBe("q=cancun&estado=confirmado&salida=rango&desde=2026-11-01&hasta=2026-11-30&vendedor=u-1&atrasados=1&orden=total");
});

it("las salidas relativas se resuelven contra hoy", () => {
  const base = filtersFromParams(new URLSearchParams());
  expect(toListParams({ ...base, salida: "30d" }, "2026-10-20")).toMatchObject({ salidaDesde: "2026-10-20", salidaHasta: "2026-11-19" });
  expect(toListParams({ ...base, salida: "mes" }, "2026-10-20")).toMatchObject({ salidaDesde: "2026-10-01", salidaHasta: "2026-10-31" });
  expect(toListParams({ ...base, salida: "mes-proximo" }, "2026-12-20")).toMatchObject({ salidaDesde: "2027-01-01", salidaHasta: "2027-01-31" });
  expect(toListParams({ ...base, salida: "rango", desde: "2026-11-01" }, "2026-10-20")).toMatchObject({ salidaDesde: "2026-11-01", salidaHasta: undefined });
});

it("el estado y los demás filtros llegan como los espera el backend", () => {
  const base = filtersFromParams(new URLSearchParams());
  expect(toListParams(base, "2026-10-20")).toEqual({ q: undefined, estado: "abiertos", vendedor: undefined, atrasados: false, orden: "salida", salidaDesde: undefined, salidaHasta: undefined });
  expect(toListParams({ ...base, estado: "en viaje", q: " Rossi ", vendedor: "ninguno", atrasados: true }, "2026-10-20"))
    .toMatchObject({ estado: "en viaje", q: "Rossi", vendedor: "ninguno", atrasados: true });
});
