"use client";

import { useState } from "react";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import { Field, PanelEditor, describeChanges } from "./PanelEditor";
import { DEFAULT_ORIGIN, StopDates, TripRoute, emptyStop, toStops, type StopDraft } from "./TripRoute";
import type { Country, Stop } from "./types";

export type TripSeed = { originCountry?: string; itinerary?: Stop[]; departAt?: string; returnAt?: string };

interface NewTripPanelProps {
  contactName: string;
  avatarUrl?: string;
  countries?: Country[];
  onCancel: () => void;
  onCreate: (input: TripSeed) => Promise<void>;
}

/**
 * Un viaje nuevo, con el mismo esquema que «Editar»: recorrido y fechas, y un
 * solo botón abajo. El cliente sale del contacto: se crea con su nombre y se
 * completa después.
 */
export function NewTripPanel({ contactName, avatarUrl, countries = [], onCancel, onCreate }: NewTripPanelProps) {
  const [origin, setOrigin] = useState(DEFAULT_ORIGIN);
  const [stops, setStops] = useState<StopDraft[]>([emptyStop("destino")]);
  const [departAt, setDepartAt] = useState("");
  const [returnAt, setReturnAt] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const itinerary = toStops(stops);

  const changes = describeChanges([
    origin !== DEFAULT_ORIGIN || itinerary.length > 0 ? "el recorrido" : "",
    departAt || returnAt ? "las fechas" : "",
  ].filter(Boolean));

  const create = async () => {
    const input: TripSeed = { originCountry: origin };
    if (itinerary.length > 0) input.itinerary = itinerary;
    if (departAt) input.departAt = departAt;
    if (returnAt) input.returnAt = returnAt;
    setSaving(true);
    setError(undefined);
    try {
      await onCreate(input);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo crear el viaje");
    } finally {
      setSaving(false);
    }
  };

  return (
    <PanelEditor title="Nuevo viaje" saveLabel="Crear viaje" changes={changes} saving={saving} error={error}
      onSave={() => void create()} onBack={onCancel}>
      <section className="editor-section">
        <h4>Recorrido</h4>
        <TripRoute origin={origin} stops={stops} countries={countries} onOrigin={setOrigin} onStops={setStops} />
      </section>
      <section className="editor-section">
        <h4>Fechas</h4>
        <div className="field-pair">
          <Field label="Salida"><input type="date" value={departAt} onChange={(event) => setDepartAt(event.target.value)} /></Field>
          <Field label="Regreso"><input type="date" value={returnAt} onChange={(event) => setReturnAt(event.target.value)} /></Field>
        </div>
        <StopDates stops={stops} countries={countries} onStops={setStops} />
      </section>
      <section className="editor-section">
        <h4>Cliente</h4>
        <div className="draft-contact">
          <ContactAvatar name={contactName} src={avatarUrl} className="avatar-36" />
          <span><strong>{contactName}</strong><small>Se crea con el nombre del contacto; los datos se completan después.</small></span>
        </div>
      </section>
    </PanelEditor>
  );
}
