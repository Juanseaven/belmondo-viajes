"use client";

import { useState } from "react";
import type { ItineraryInput } from "./api";
import { dayOf } from "./format";
import { Field, PanelEditor, describeChanges } from "./PanelEditor";
import { DEFAULT_ORIGIN, StopDates, TripRoute, toStops, type StopDraft } from "./TripRoute";
import type { Country, Customer, CustomerUpdate, Seller, TripDetail, TripUpdate } from "./types";

const fromCustomer = (customer: Customer) => ({
  name: customer.name, email: customer.email ?? "", phoneNumber: customer.phoneNumber ?? "",
  documentType: customer.documentType ?? "", documentNumber: customer.documentNumber ?? "",
});

const fromTrip = (trip: TripDetail): StopDraft[] =>
  (trip.itinerary ?? []).map((stop) => ({ ...stop, arriveAt: dayOf(stop.arriveAt), leaveAt: dayOf(stop.leaveAt) }));

/**
 * El viaje y su cliente en una sola pantalla, con un solo «Guardar cambios».
 * Guarda solo lo que cambió, en tres pasos: el itinerario, el viaje y el
 * cliente. Si uno falla, lo dice y deja guardado lo anterior.
 */
export function TripEditor({ trip, countries, sellers = [], onBack, onUpdateTrip, onSaveItinerary, onUpdateCustomer }: {
  trip: TripDetail;
  countries: Country[];
  sellers?: Seller[];
  onBack: () => void;
  onUpdateTrip?: (input: TripUpdate) => Promise<void>;
  onSaveItinerary?: (input: ItineraryInput) => Promise<void>;
  onUpdateCustomer?: (input: CustomerUpdate) => Promise<void>;
}) {
  const savedOrigin = trip.originCountry || DEFAULT_ORIGIN;
  const [origin, setOrigin] = useState(savedOrigin);
  const [stops, setStops] = useState(() => fromTrip(trip));
  const [departAt, setDepartAt] = useState(dayOf(trip.departAt));
  const [returnAt, setReturnAt] = useState(dayOf(trip.returnAt));
  const [seller, setSeller] = useState(trip.sellerUserId ?? "");
  const [customer, setCustomer] = useState(() => fromCustomer(trip.customer));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  const initialCustomer = fromCustomer(trip.customer);
  const customerPatch: CustomerUpdate = {};
  for (const key of Object.keys(customer) as (keyof typeof customer)[]) {
    if (customer[key].trim() !== initialCustomer[key]) customerPatch[key] = customer[key].trim();
  }

  const routeDirty = origin !== savedOrigin || JSON.stringify(toStops(stops)) !== JSON.stringify(toStops(fromTrip(trip)));
  const datesDirty = departAt !== dayOf(trip.departAt) || returnAt !== dayOf(trip.returnAt);
  const sellerDirty = seller !== (trip.sellerUserId ?? "");
  const customerDirty = Object.keys(customerPatch).length > 0;

  const changes = describeChanges([
    routeDirty ? "el recorrido" : "",
    datesDirty ? "las fechas" : "",
    sellerDirty ? "el vendedor" : "",
    customerDirty ? "el cliente" : "",
  ].filter(Boolean));

  // Un vendedor dado de baja ya no figura en la lista, pero el viaje lo sigue
  // nombrando: se agrega para que el selector no mienta diciendo «Sin asignar».
  const sellerOptions = trip.sellerUserId && !sellers.some((s) => s.id === trip.sellerUserId)
    ? [...sellers, { id: trip.sellerUserId, name: trip.sellerName || "Vendedor anterior" }]
    : sellers;

  const save = async () => {
    setSaving(true);
    setError(undefined);
    const done: string[] = [];
    const step = async (what: string, run: () => Promise<void>) => {
      try {
        await run();
        done.push(what);
      } catch (cause) {
        const reason = cause instanceof Error ? cause.message : "error desconocido";
        throw new Error(`No se guardó ${what}: ${reason}.${done.length ? ` Sí quedó guardado ${done.join(" y ")}.` : ""}`);
      }
    };
    try {
      if (routeDirty && onSaveItinerary) {
        await step("el recorrido", () => onSaveItinerary({ originCountry: origin, itinerary: toStops(stops) }));
      }
      if ((datesDirty || sellerDirty) && onUpdateTrip) {
        const input: TripUpdate = {};
        if (datesDirty) {
          if (departAt) input.departAt = departAt;
          if (returnAt) input.returnAt = returnAt;
        }
        if (sellerDirty) input.sellerUserId = seller;
        await step(sellerDirty && !datesDirty ? "el vendedor" : "las fechas", () => onUpdateTrip(input));
      }
      if (customerDirty && onUpdateCustomer) {
        await step("el cliente", () => onUpdateCustomer(customerPatch));
      }
      onBack();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar");
    } finally {
      setSaving(false);
    }
  };

  const setField = (key: keyof typeof customer, value: string) => setCustomer((current) => ({ ...current, [key]: value }));

  return (
    <PanelEditor title={`Editar viaje #${trip.number}`} changes={changes} saving={saving} error={error}
      canSave={Boolean(changes)} onSave={() => void save()} onBack={onBack}>
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
        <h4>Vendedor</h4>
        <Field label="Vende">
          <select aria-label="Vendedor" value={seller} onChange={(event) => setSeller(event.target.value)}>
            <option value="">Sin asignar</option>
            {sellerOptions.map((option) => <option key={option.id} value={option.id}>{option.name}</option>)}
          </select>
        </Field>
      </section>
      <section className="editor-section">
        <h4>Cliente</h4>
        <Field label="Nombre"><input value={customer.name} onChange={(e) => setField("name", e.target.value)} required /></Field>
        <div className="field-pair">
          <Field label="Teléfono"><input type="tel" value={customer.phoneNumber} onChange={(e) => setField("phoneNumber", e.target.value)} placeholder="Sin cargar" /></Field>
          <Field label="Mail"><input type="email" value={customer.email} onChange={(e) => setField("email", e.target.value)} placeholder="nombre@mail.com" /></Field>
        </div>
        <div className="field-pair">
          <Field label="Documento">
            <select value={customer.documentType} onChange={(e) => setField("documentType", e.target.value)}>
              <option value="">Elegir</option>
              <option value="DNI">DNI</option>
              <option value="Pasaporte">Pasaporte</option>
            </select>
          </Field>
          <Field label="Número"><input value={customer.documentNumber} onChange={(e) => setField("documentNumber", e.target.value)} placeholder="Sin cargar" /></Field>
        </div>
      </section>
    </PanelEditor>
  );
}
