import type { TripListParams, TripSort } from "./types";

/**
 * Los filtros de «Todos los viajes». Viven en la URL, como los del espejo: una
 * recarga o un enlace pasado a otra persona muestran la misma lista.
 */
export type EstadoFilter = "abiertos" | "consulta" | "cotizado" | "confirmado" | "en viaje" | "cerrado";
export type SalidaFilter = "" | "30d" | "mes" | "mes-proximo" | "rango";

export interface TripFilters {
  q: string;
  estado: EstadoFilter;
  salida: SalidaFilter;
  desde: string;
  hasta: string;
  /** Vacío: todos. «ninguno»: sin asignar. Si no, el id del vendedor. */
  vendedor: string;
  atrasados: boolean;
  orden: TripSort;
}

const ESTADOS: EstadoFilter[] = ["abiertos", "consulta", "cotizado", "confirmado", "en viaje", "cerrado"];
const SALIDAS: SalidaFilter[] = ["", "30d", "mes", "mes-proximo", "rango"];
const ORDENES: TripSort[] = ["salida", "numero", "total"];

const DEFAULTS: TripFilters = { q: "", estado: "abiertos", salida: "", desde: "", hasta: "", vendedor: "", atrasados: false, orden: "salida" };

const pick = <T extends string>(value: string | null, allowed: T[], fallback: T): T =>
  allowed.includes(value as T) ? (value as T) : fallback;

export function filtersFromParams(params: URLSearchParams): TripFilters {
  const salida = pick(params.get("salida"), SALIDAS, "");
  return {
    q: params.get("q") ?? "",
    estado: pick(params.get("estado"), ESTADOS, "abiertos"),
    salida,
    desde: salida === "rango" ? params.get("desde") ?? "" : "",
    hasta: salida === "rango" ? params.get("hasta") ?? "" : "",
    vendedor: params.get("vendedor") ?? "",
    atrasados: params.get("atrasados") === "1",
    orden: pick(params.get("orden"), ORDENES, "salida"),
  };
}

/** Solo lo que difiere del valor por defecto: la URL queda corta y legible. */
export function filtersToParams(filters: TripFilters): URLSearchParams {
  const params = new URLSearchParams();
  const set = (key: string, value: string) => { if (value) params.set(key, value); };
  set("q", filters.q);
  set("estado", filters.estado === DEFAULTS.estado ? "" : filters.estado);
  set("salida", filters.salida);
  if (filters.salida === "rango") {
    set("desde", filters.desde);
    set("hasta", filters.hasta);
  }
  set("vendedor", filters.vendedor);
  set("atrasados", filters.atrasados ? "1" : "");
  set("orden", filters.orden === DEFAULTS.orden ? "" : filters.orden);
  return params;
}

export const hasActiveFilters = (filters: TripFilters) => filtersToParams({ ...filters, orden: DEFAULTS.orden }).toString() !== "";

export const defaultFilters = (): TripFilters => ({ ...DEFAULTS });

const iso = (date: Date) => date.toISOString().slice(0, 10);
const fromDay = (day: string) => {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d));
};

/** Las salidas relativas se resuelven acá, contra el día de quien mira. */
function departRange(filters: TripFilters, today: string): { salidaDesde?: string; salidaHasta?: string } {
  const now = fromDay(today);
  const year = now.getUTCFullYear();
  const month = now.getUTCMonth();
  switch (filters.salida) {
    case "30d":
      return { salidaDesde: today, salidaHasta: iso(new Date(Date.UTC(year, month, now.getUTCDate() + 30))) };
    case "mes":
      return { salidaDesde: iso(new Date(Date.UTC(year, month, 1))), salidaHasta: iso(new Date(Date.UTC(year, month + 1, 0))) };
    case "mes-proximo":
      return { salidaDesde: iso(new Date(Date.UTC(year, month + 1, 1))), salidaHasta: iso(new Date(Date.UTC(year, month + 2, 0))) };
    case "rango":
      return { salidaDesde: filters.desde || undefined, salidaHasta: filters.hasta || undefined };
    default:
      return { salidaDesde: undefined, salidaHasta: undefined };
  }
}

export function toListParams(filters: TripFilters, today: string): TripListParams {
  return {
    q: filters.q.trim() || undefined,
    estado: filters.estado,
    vendedor: filters.vendedor || undefined,
    atrasados: filters.atrasados,
    orden: filters.orden,
    ...departRange(filters, today),
  };
}
