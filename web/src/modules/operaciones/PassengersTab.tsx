"use client";

import { useState } from "react";
import { Pencil, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { NewPassenger } from "./api";
import { dayOf, shortDate } from "./format";
import type { Passenger, TripDetail } from "./types";

export type PassengerPatch = Partial<NewPassenger>;

interface PassengersTabProps {
  trip: TripDetail;
  onAddPassenger: (input: NewPassenger) => Promise<void> | void;
  onUpdatePassenger?: (id: string, input: PassengerPatch) => Promise<void>;
  onDeletePassenger?: (passenger: Passenger) => Promise<void>;
}

/**
 * Los pasajeros se cargan de a pedazos: nombre y apellido alcanzan para el alta,
 * y el documento se completa cuando llega la foto. Quitar no pregunta: se hace y
 * queda «Deshacer», que vuelve a cargar a la misma persona con todos sus datos.
 */
export function PassengersTab({ trip, onAddPassenger, onUpdatePassenger, onDeletePassenger }: PassengersTabProps) {
  const [adding, setAdding] = useState(false);
  const [editingId, setEditingId] = useState<string>();
  const [removed, setRemoved] = useState<Passenger>();
  const [error, setError] = useState<string>();

  const run = async (action: () => Promise<void> | void) => {
    setError(undefined);
    try {
      await action();
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar el pasajero");
      return false;
    }
  };

  const remove = async (passenger: Passenger) => {
    if (!onDeletePassenger) return;
    if (await run(() => onDeletePassenger(passenger))) {
      setEditingId(undefined);
      setRemoved(passenger);
    }
  };

  const undo = async (passenger: Passenger) => {
    if (await run(() => onAddPassenger(asNewPassenger(passenger)))) setRemoved(undefined);
  };

  return (
    <section className="detail-section">
      <h3>Pasajeros</h3>
      {error && <p className="trip-form-error" role="alert">{error}</p>}
      {removed && (
        <p className="trip-undo" role="status">
          Quitaste a {removed.firstName} {removed.lastName}.
          <button type="button" onClick={() => void undo(removed)}>Deshacer</button>
        </p>
      )}
      {trip.passengers.length === 0 ? (
        <div className="detail-empty trip-empty">
          <strong>Cargá los pasajeros para empezar</strong>
          <p>Nombre y apellido alcanzan; el documento se completa después.</p>
        </div>
      ) : (
        <ul className="passenger-list">
          {trip.passengers.map((passenger) => editingId === passenger.id ? (
            <li key={passenger.id}>
              <PassengerForm
                passenger={passenger}
                onCancel={() => setEditingId(undefined)}
                onSave={async (patch) => {
                  if (!onUpdatePassenger || Object.keys(patch).length === 0) return setEditingId(undefined);
                  if (await run(() => onUpdatePassenger(passenger.id, patch))) setEditingId(undefined);
                }}
                onRemove={onDeletePassenger ? () => void remove(passenger) : undefined}
              />
            </li>
          ) : (
            <PassengerRow
              key={passenger.id}
              passenger={passenger}
              returnAt={trip.returnAt}
              onEdit={onUpdatePassenger || onDeletePassenger ? () => { setAdding(false); setEditingId(passenger.id); } : undefined}
            />
          ))}
        </ul>
      )}
      {!adding ? (
        <Button size="sm" onClick={() => { setEditingId(undefined); setAdding(true); }}><Plus />Agregar pasajero</Button>
      ) : (
        <NewPassengerForm onSave={async (input) => { if (await run(() => onAddPassenger(input))) setAdding(false); }} />
      )}
    </section>
  );
}

function PassengerRow({ passenger, returnAt, onEdit }: { passenger: Passenger; returnAt?: string; onEdit?: () => void }) {
  const expires = dayOf(passenger.documentExpiresAt);
  // El mismo criterio que el backend usa para volver crítico el control del
  // documento. Acá se dice en palabras: nada rojo sin explicación.
  const expiresTooSoon = Boolean(expires && returnAt && expires < dayOf(returnAt));
  const facts = [
    passenger.isHolder ? "Titular" : undefined,
    passenger.kind === "menor" ? "Menor" : undefined,
    passenger.documentType ? `${passenger.documentType} ${passenger.documentNumber ?? ""}`.trim() : "Sin documento",
    expires ? `vence ${shortDate(expires)}` : undefined,
  ].filter(Boolean).join(" · ");

  return (
    <li className="passenger-row">
      <span>
        <strong>{passenger.lastName}, {passenger.firstName}</strong>
        <small>{facts}</small>
        {expiresTooSoon && <small className="trip-warn">El documento vence antes del regreso</small>}
      </span>
      {onEdit && (
        <Button variant="ghost" size="icon-xs" aria-label={`Editar ${passenger.lastName}, ${passenger.firstName}`} onClick={onEdit}>
          <Pencil />
        </Button>
      )}
    </li>
  );
}

function PassengerForm({ passenger, onSave, onCancel, onRemove }: {
  passenger: Passenger;
  onSave: (patch: PassengerPatch) => Promise<void> | void;
  onCancel: () => void;
  onRemove?: () => void;
}) {
  const initial = {
    firstName: passenger.firstName, lastName: passenger.lastName, kind: passenger.kind,
    isHolder: passenger.isHolder, documentType: passenger.documentType ?? "",
    documentNumber: passenger.documentNumber ?? "", documentExpiresAt: dayOf(passenger.documentExpiresAt),
  };
  const [values, setValues] = useState(initial);
  const [saving, setSaving] = useState(false);
  const set = <K extends keyof typeof values>(key: K, value: (typeof values)[K]) => setValues((current) => ({ ...current, [key]: value }));

  return (
    <form className="trip-payment-form passenger-form" onSubmit={async (event) => {
      event.preventDefault();
      // Solo viaja lo que cambió: un PATCH con el formulario entero pisaría lo
      // que otra persona haya editado mientras tanto en otro campo.
      const patch: PassengerPatch = {};
      for (const key of Object.keys(values) as (keyof typeof values)[]) {
        const value = typeof values[key] === "string" ? (values[key] as string).trim() : values[key];
        if (value !== initial[key]) (patch as Record<string, unknown>)[key] = value;
      }
      setSaving(true);
      try { await onSave(patch); } finally { setSaving(false); }
    }}>
      <div className="trip-date-fields">
        <label>Nombre<input value={values.firstName} onChange={(e) => set("firstName", e.target.value)} required /></label>
        <label>Apellido<input value={values.lastName} onChange={(e) => set("lastName", e.target.value)} required /></label>
      </div>
      <div className="trip-date-fields">
        <label>Edad
          <select value={values.kind} onChange={(e) => set("kind", e.target.value as Passenger["kind"])}>
            <option value="adulto">Adulto</option>
            <option value="menor">Menor</option>
          </select>
        </label>
        <label className="trip-check"><input type="checkbox" checked={values.isHolder} onChange={(e) => set("isHolder", e.target.checked)} />Titular</label>
      </div>
      <div className="trip-date-fields">
        <label>Documento
          <select value={values.documentType} onChange={(e) => set("documentType", e.target.value as Passenger["documentType"] & string)}>
            <option value="">Sin cargar</option>
            <option value="DNI">DNI</option>
            <option value="Pasaporte">Pasaporte</option>
          </select>
        </label>
        <label>Número<input value={values.documentNumber} onChange={(e) => set("documentNumber", e.target.value)} /></label>
      </div>
      <label>Vence<input type="date" value={values.documentExpiresAt} onChange={(e) => set("documentExpiresAt", e.target.value)} /></label>
      <div className="trip-form-actions">
        <Button size="sm" type="submit" disabled={saving}>{saving ? "Guardando…" : "Guardar pasajero"}</Button>
        <Button size="sm" type="button" variant="ghost" onClick={onCancel}>Cancelar</Button>
        {onRemove && <Button size="sm" type="button" variant="ghost" className="trip-remove" onClick={onRemove}>Quitar pasajero</Button>}
      </div>
    </form>
  );
}

function NewPassengerForm({ onSave }: { onSave: (input: NewPassenger) => Promise<void> }) {
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  return (
    <form className="trip-inline-form" onSubmit={(event) => {
      event.preventDefault();
      void onSave({ firstName: firstName.trim(), lastName: lastName.trim() });
    }}>
      <input aria-label="Nombre" placeholder="Nombre" value={firstName} onChange={(e) => setFirstName(e.target.value)} required />
      <input aria-label="Apellido" placeholder="Apellido" value={lastName} onChange={(e) => setLastName(e.target.value)} required />
      <Button size="sm" type="submit">Guardar pasajero</Button>
    </form>
  );
}

/** La misma persona, lista para volver a cargarse con todo lo que tenía. */
function asNewPassenger(passenger: Passenger): NewPassenger {
  return {
    firstName: passenger.firstName, lastName: passenger.lastName, kind: passenger.kind,
    isHolder: passenger.isHolder, documentType: passenger.documentType,
    documentNumber: passenger.documentNumber, documentExpiresAt: dayOf(passenger.documentExpiresAt) || undefined,
  };
}
