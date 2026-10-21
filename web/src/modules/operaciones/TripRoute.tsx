"use client";

import { useState } from "react";
import { ChevronDown, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CountrySelect, countryName } from "./countries";
import { Field } from "./PanelEditor";
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
 * De dónde sale y a dónde va. Los países elegidos se ven como una lista, cada
 * uno con su «✕»; el que se está agregando, como un selector.
 */
export function TripRoute({ origin, stops, countries, onOrigin, onStops }: {
  origin: string;
  stops: StopDraft[];
  countries: Country[];
  onOrigin: (code: string) => void;
  onStops: (stops: StopDraft[]) => void;
}) {
  const set = (index: number, patch: Partial<StopDraft>) =>
    onStops(stops.map((stop, i) => (i === index ? { ...stop, ...patch } : stop)));
  const labels = stopLabels(stops);

  return (
    <div className="trip-route">
      <Field label="Sale de">
        <CountrySelect label="Sale de" value={origin} countries={countries} onChange={(code) => onOrigin(code || DEFAULT_ORIGIN)} />
      </Field>
      {stops.length > 0 && (
        <ul className="route-stops">
          {stops.map((stop, index) => (
            <li key={index} className="route-stop">
              <span className="route-stop-role">{stop.role === "escala" ? "Escala" : "Destino"}</span>
              {stop.country ? (
                <span className="route-stop-name">{countryName(countries, stop.country)}</span>
              ) : (
                <CountrySelect label={labels[index]} value="" countries={countries}
                  emptyLabel={stop.role === "escala" ? "Elegí la escala" : "Elegí un país"}
                  onChange={(code) => set(index, { country: code })} />
              )}
              <Button size="icon-xs" type="button" variant="ghost" aria-label={`Quitar ${labels[index]}`} title="Quitar"
                onClick={() => onStops(stops.filter((_, i) => i !== index))}><X /></Button>
            </li>
          ))}
        </ul>
      )}
      <div className="route-actions">
        <button type="button" className="link-button" onClick={() => onStops([...stops, emptyStop("destino")])}><Plus aria-hidden />Agregar destino</button>
        <button type="button" className="link-button" onClick={() => onStops([...stops, emptyStop("escala")])}><Plus aria-hidden />Agregar escala</button>
      </div>
    </div>
  );
}

/**
 * Las fechas de cada país, plegadas: casi siempre alcanzan las del viaje.
 */
export function StopDates({ stops, countries, onStops }: {
  stops: StopDraft[];
  countries: Country[];
  onStops: (stops: StopDraft[]) => void;
}) {
  const [open, setOpen] = useState(() => stops.some((stop) => stop.arriveAt || stop.leaveAt));
  const chosen = stops.map((stop, index) => ({ stop, index })).filter(({ stop }) => stop.country);
  if (chosen.length === 0) return null;
  const set = (index: number, patch: Partial<StopDraft>) =>
    onStops(stops.map((stop, i) => (i === index ? { ...stop, ...patch } : stop)));

  return (
    <div className="stop-dates">
      <button type="button" className="link-button" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
        <ChevronDown aria-hidden />Fechas en cada país
      </button>
      {open && chosen.map(({ stop, index }) => {
        const name = countryName(countries, stop.country);
        return (
          <div key={index} className="stop-dates-row">
            <strong>{name}</strong>
            <div className="field-pair">
              <Field label="Llega"><input type="date" aria-label={`Llega a ${name}`} value={stop.arriveAt} onChange={(event) => set(index, { arriveAt: event.target.value })} /></Field>
              <Field label="Sale"><input type="date" aria-label={`Sale de ${name}`} value={stop.leaveAt} onChange={(event) => set(index, { leaveAt: event.target.value })} /></Field>
            </div>
          </div>
        );
      })}
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
