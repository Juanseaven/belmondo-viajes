"use client";

import { ChevronDown, Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { EstadoFilter, SalidaFilter, TripFilters } from "./tripFilters";
import type { Seller } from "./types";

const ESTADOS: { value: EstadoFilter; label: string }[] = [
  { value: "abiertos", label: "Abiertos" },
  { value: "consulta", label: "Consultas" },
  { value: "cotizado", label: "Cotizados" },
  { value: "confirmado", label: "Confirmados" },
  { value: "en viaje", label: "En viaje" },
  { value: "cerrado", label: "Cerrados" },
];

const SALIDAS: { value: SalidaFilter; label: string }[] = [
  { value: "", label: "Cualquier fecha" },
  { value: "30d", label: "Próximos 30 días" },
  { value: "mes", label: "Este mes" },
  { value: "mes-proximo", label: "El mes que viene" },
  { value: "rango", label: "Elegir fechas…" },
];

/** Cuántos viajes hay en un chip de estado. «Abiertos» suma todos menos los cerrados. */
function countFor(value: EstadoFilter, byStatus?: Record<string, number>) {
  if (!byStatus) return undefined;
  if (value === "abiertos") {
    return Object.entries(byStatus).reduce((sum, [status, n]) => (status === "cerrado" ? sum : sum + n), 0);
  }
  return byStatus[value] ?? 0;
}

/**
 * La barra de filtros, en dos renglones: arriba lo que se escribe o se elige de
 * una lista, abajo los estados con cuántos viajes hay en cada uno. Cada cambio
 * es inmediato: no hay botón «Aplicar».
 */
export function TripFiltersBar({ value, sellers, byStatus, onChange }: {
  value: TripFilters;
  sellers: Seller[];
  /** Cuántos viajes hay en cada estado, con los demás filtros aplicados. */
  byStatus?: Record<string, number>;
  onChange: (next: TripFilters) => void;
}) {
  const set = <K extends keyof TripFilters>(key: K, next: TripFilters[K]) => onChange({ ...value, [key]: next });
  const salida = SALIDAS.find((option) => option.value === value.salida)?.label ?? "Cualquier fecha";
  const vendedor = value.vendedor === "" ? "Todos"
    : value.vendedor === "ninguno" ? "Sin asignar"
      : sellers.find((seller) => seller.id === value.vendedor)?.name ?? "Otro";

  return (
    <div className="trips-filters">
      <div className="trips-filters-row">
        <div className="trips-search">
          <Search aria-hidden />
          <Input
            type="search"
            aria-label="Buscar viajes"
            placeholder="Buscar por cliente, pasajero, destino o número"
            value={value.q}
            onChange={(event) => set("q", event.target.value)}
          />
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button type="button" className="filter-drop" aria-label={`Salida: ${salida}`}>Salida: <b>{salida}</b><ChevronDown aria-hidden /></button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" sideOffset={6}>
            <DropdownMenuRadioGroup value={value.salida} onValueChange={(next) => onChange({ ...value, salida: next as SalidaFilter, desde: "", hasta: "" })}>
              {SALIDAS.map((option) => <DropdownMenuRadioItem key={option.value} value={option.value}>{option.label}</DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        {value.salida === "rango" && <>
          <label className="filter-date">Desde<input type="date" value={value.desde} onChange={(event) => set("desde", event.target.value)} /></label>
          <label className="filter-date">Hasta<input type="date" value={value.hasta} onChange={(event) => set("hasta", event.target.value)} /></label>
        </>}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button type="button" className="filter-drop" aria-label={`Vendedor: ${vendedor}`}>Vendedor: <b>{vendedor}</b><ChevronDown aria-hidden /></button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" sideOffset={6}>
            <DropdownMenuRadioGroup value={value.vendedor} onValueChange={(next) => set("vendedor", next)}>
              <DropdownMenuRadioItem value="">Todos</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="ninguno">Sin asignar</DropdownMenuRadioItem>
              {sellers.length > 0 && <DropdownMenuSeparator />}
              {sellers.map((seller) => <DropdownMenuRadioItem key={seller.id} value={seller.id}>{seller.name}</DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div className="trips-filters-row" role="group" aria-label="Estado">
        {ESTADOS.map((estado) => {
          const count = countFor(estado.value, byStatus);
          return (
            <button key={estado.value} type="button" className="trips-chip" aria-pressed={value.estado === estado.value} onClick={() => set("estado", estado.value)}>
              {estado.label}{count !== undefined && <> <em>{count}</em></>}
            </button>
          );
        })}
        <span className="trips-chip-sep" aria-hidden />
        <button type="button" className="trips-chip is-late" aria-pressed={value.atrasados} onClick={() => set("atrasados", !value.atrasados)}>
          <i aria-hidden />Con algo atrasado
        </button>
      </div>
    </div>
  );
}
