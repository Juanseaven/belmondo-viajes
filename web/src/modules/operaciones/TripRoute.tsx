"use client";

import { useState } from "react";
import { ChevronDown, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CountrySelect } from "./countries";
import type { Country, Stop } from "./types";

/** Un país mientras se edita: las fechas, vacías hasta que alguien las cargue. */
export type StopDraft = Stop & { arriveAt: string; leaveAt: string };

/** De dónde sale el viaje si nadie dice otra cosa: la agencia vende desde ahí. */
export const DEFAULT_ORIGIN = "AR";

export const emptyStop = (role: Stop["role"]): StopDraft => ({ country: "", role, arriveAt: "", leaveAt: "" });

/** Lo que se manda: los países elegidos, con las fechas que tengan. */
export const toStops = (stops: StopDraft[]): Stop[] =>
  stops.filter((stop) => stop.country).map((stop) => ({
    country: stop.country, role: stop.role,
    ...(stop.arriveAt ? { arriveAt: stop.arriveAt } : {}),
    ...(stop.leaveAt ? { leaveAt: stop.leaveAt } : {}),
  }));

/**
 * De dónde sale y a dónde va, como en un buscador de requisitos. Cada país de
 * «Va a» dice si es destino o escala; las fechas por país son opcionales y van
 * plegadas, porque casi siempre alcanzan las del viaje.
 */
export function TripRoute({ origin, stops, countries, onOrigin, onStops, withStopDates }: {
  origin: string;
  stops: StopDraft[];
  countries: Country[];
  onOrigin: (code: string) => void;
  onStops: (stops: StopDraft[]) => void;
  withStopDates?: boolean;
}) {
  const [showDates, setShowDates] = useState(false);
  const set = (index: number, patch: Partial<StopDraft>) =>
    onStops(stops.map((stop, i) => (i === index ? { ...stop, ...patch } : stop)));
  const labels = stopLabels(stops);

  return (
    <div className="trip-route">
      <div className="trip-route-field">
        <small>Sale de</small>
        <CountrySelect label="Sale de" value={origin} countries={countries} onChange={(code) => onOrigin(code || DEFAULT_ORIGIN)} />
      </div>
      <div className="trip-route-field">
        <small>Va a</small>
        {stops.map((stop, index) => (
          <div key={index} className="trip-route-stop">
            <div className="trip-route-country">
              <CountrySelect label={labels[index]} value={stop.country} countries={countries}
                emptyLabel={stop.role === "escala" ? "Elegí la escala" : "Elegí un país"}
                onChange={(code) => set(index, { country: code })} />
              {stop.role === "escala" && <span className="trip-route-role">Escala</span>}
              <Button size="icon-xs" type="button" variant="ghost" aria-label={`Quitar ${labels[index]}`} title="Quitar"
                onClick={() => onStops(stops.filter((_, i) => i !== index))}><Trash2 /></Button>
            </div>
            {withStopDates && showDates && (
              <div className="trip-date-fields">
                <label>Llega<input type="date" aria-label={`Llega a ${labels[index]}`} value={stop.arriveAt} onChange={(event) => set(index, { arriveAt: event.target.value })} /></label>
                <label>Sale<input type="date" aria-label={`Sale de ${labels[index]}`} value={stop.leaveAt} onChange={(event) => set(index, { leaveAt: event.target.value })} /></label>
              </div>
            )}
          </div>
        ))}
        <div className="trip-form-actions">
          <Button size="sm" type="button" variant="ghost" onClick={() => onStops([...stops, emptyStop("destino")])}><Plus />Agregar destino</Button>
          <Button size="sm" type="button" variant="ghost" onClick={() => onStops([...stops, emptyStop("escala")])}><Plus />Agregar escala</Button>
          {withStopDates && stops.length > 0 && (
            <Button size="sm" type="button" variant="ghost" aria-expanded={showDates} onClick={() => setShowDates((value) => !value)}>
              <ChevronDown aria-hidden />Fechas en cada país
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

/** «Destino», «Destino 2», «Escala»: con uno solo de cada, sin número. */
function stopLabels(stops: StopDraft[]) {
  const total = { destino: 0, escala: 0 };
  for (const stop of stops) total[stop.role] += 1;
  const seen = { destino: 0, escala: 0 };
  return stops.map((stop) => {
    seen[stop.role] += 1;
    const name = stop.role === "escala" ? "Escala" : "Destino";
    return total[stop.role] > 1 ? `${name} ${seen[stop.role]}` : name;
  });
}
