"use client";

import { Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import type { EstadoFilter, SalidaFilter, TripFilters } from "./tripFilters";
import type { Seller } from "./types";

const ESTADOS: { value: EstadoFilter; label: string }[] = [
  { value: "abiertos", label: "Abiertos" },
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

/** La barra de filtros. Cada cambio es inmediato: no hay botón «Aplicar». */
export function TripFiltersBar({ value, sellers, onChange }: { value: TripFilters; sellers: Seller[]; onChange: (next: TripFilters) => void }) {
  const set = <K extends keyof TripFilters>(key: K, next: TripFilters[K]) => onChange({ ...value, [key]: next });

  return (
    <div className="trips-filters">
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
      <div className="trips-chips" role="group" aria-label="Estado">
        {ESTADOS.map((estado) => (
          <button key={estado.value} type="button" className="trips-chip" aria-pressed={value.estado === estado.value} onClick={() => set("estado", estado.value)}>
            {estado.label}
          </button>
        ))}
        <button type="button" className="trips-chip is-late" aria-pressed={value.atrasados} onClick={() => set("atrasados", !value.atrasados)}>
          Con algo atrasado
        </button>
      </div>
      <div className="trips-selects">
        <label>Salida
          <select value={value.salida} onChange={(event) => onChange({ ...value, salida: event.target.value as SalidaFilter, desde: "", hasta: "" })}>
            {SALIDAS.map((salida) => <option key={salida.value} value={salida.value}>{salida.label}</option>)}
          </select>
        </label>
        {value.salida === "rango" && <>
          <label>Desde<input type="date" value={value.desde} onChange={(event) => set("desde", event.target.value)} /></label>
          <label>Hasta<input type="date" value={value.hasta} onChange={(event) => set("hasta", event.target.value)} /></label>
        </>}
        <label>Vendedor
          <select value={value.vendedor} onChange={(event) => set("vendedor", event.target.value)}>
            <option value="">Todos</option>
            <option value="ninguno">Sin asignar</option>
            {sellers.map((seller) => <option key={seller.id} value={seller.id}>{seller.name}</option>)}
          </select>
        </label>
      </div>
    </div>
  );
}
