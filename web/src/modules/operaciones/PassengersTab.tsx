"use client";

import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { DocumentInput, NewPassenger, PermitInput } from "./api";
import { CountrySelect, countryName } from "./countries";
import { dayOf, shortDate } from "./format";
import type { Country, Passenger, Permit, PermitKind, TravelDocument, TripDetail } from "./types";

export type PassengerPatch = Partial<NewPassenger>;

export interface DocumentActions {
  onAddDocument?: (passengerId: string, input: DocumentInput) => Promise<void>;
  onUpdateDocument?: (id: string, input: Partial<DocumentInput>) => Promise<void>;
  onDeleteDocument?: (document: TravelDocument) => Promise<void>;
  onAddPermit?: (passengerId: string, input: PermitInput) => Promise<void>;
  onDeletePermit?: (permit: Permit) => Promise<void>;
}

interface PassengersTabProps extends DocumentActions {
  trip: TripDetail;
  countries?: Country[];
  onAddPassenger: (input: NewPassenger) => Promise<void> | void;
  onUpdatePassenger?: (id: string, input: PassengerPatch) => Promise<void>;
  onDeletePassenger?: (passenger: Passenger) => Promise<void>;
}

const PERMIT_LABEL: Record<PermitKind, string> = {
  visa: "Visa",
  autorizacion_electronica: "Autorización electrónica",
  residencia: "Residencia",
};

/** El documento con el que viaja: el que define qué se le pide en cada país. */
export const travelDocument = (passenger: Passenger) =>
  passenger.documents.find((document) => document.id === passenger.travelDocumentId);

const documentLabel = (document: TravelDocument, countries: Country[]) =>
  `${document.type} ${document.number || countryName(countries, document.country)}`.trim();

/**
 * Los pasajeros se cargan de a pedazos: nombre y apellido alcanzan para el alta,
 * y los documentos se completan cuando llega la foto. Quien tiene dos
 * nacionalidades carga sus dos pasaportes y elige con cuál viaja. Quitar no
 * pregunta: se hace y queda «Deshacer».
 */
export function PassengersTab({ trip, countries = [], onAddPassenger, onUpdatePassenger, onDeletePassenger, ...docs }: PassengersTabProps) {
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
                countries={countries}
                onCancel={() => setEditingId(undefined)}
                onSave={async (patch) => {
                  if (!onUpdatePassenger || Object.keys(patch).length === 0) return setEditingId(undefined);
                  if (await run(() => onUpdatePassenger(passenger.id, patch))) setEditingId(undefined);
                }}
                onRemove={onDeletePassenger ? () => void remove(passenger) : undefined}
              />
              <DocumentsEditor passenger={passenger} countries={countries} run={run} onUpdatePassenger={onUpdatePassenger} {...docs} />
              <PermitsEditor passenger={passenger} countries={countries} run={run} {...docs} />
            </li>
          ) : (
            <PassengerRow
              key={passenger.id}
              passenger={passenger}
              countries={countries}
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

function PassengerRow({ passenger, countries, returnAt, onEdit }: { passenger: Passenger; countries: Country[]; returnAt?: string; onEdit?: () => void }) {
  const document = travelDocument(passenger);
  const expires = dayOf(document?.expiresAt);
  // El criterio por defecto del control del documento: que llegue al regreso.
  // Lo que pide cada destino se ve en la pestaña Requisitos.
  const expiresTooSoon = Boolean(expires && returnAt && expires < dayOf(returnAt));
  const foreign = document && document.country !== "AR" ? ` ${countryName(countries, document.country)}` : "";
  const facts = [
    passenger.isHolder ? "Titular" : undefined,
    passenger.kind === "menor" ? "Menor" : undefined,
    document ? `${document.type}${foreign} ${document.number ?? ""}`.trim() : "Sin documento",
    expires ? `vence ${shortDate(expires)}` : undefined,
    passenger.documents.length > 1 ? `${passenger.documents.length} documentos` : undefined,
    passenger.permits.length > 0 ? `${passenger.permits.length} ${passenger.permits.length === 1 ? "visa o permiso" : "visas o permisos"}` : undefined,
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

/** Guarda solo lo que cambió: un PATCH entero pisaría lo que otra persona editó. */
function changed<T extends Record<string, unknown>>(values: T, initial: T): Partial<T> {
  const patch: Partial<T> = {};
  for (const key of Object.keys(values) as (keyof T)[]) {
    const value = typeof values[key] === "string" ? (values[key] as string).trim() : values[key];
    if (value !== initial[key]) patch[key] = value as T[keyof T];
  }
  return patch;
}

/** Solo lo que tiene valor: en un alta, un campo vacío es un campo sin cargar. */
function filled<T extends Record<string, unknown>>(values: T): Partial<T> {
  const out: Partial<T> = {};
  for (const key of Object.keys(values) as (keyof T)[]) {
    const value = typeof values[key] === "string" ? (values[key] as string).trim() : values[key];
    if (value !== "" && value !== undefined) out[key] = value as T[keyof T];
  }
  return out;
}

function PassengerForm({ passenger, countries, onSave, onCancel, onRemove }: {
  passenger: Passenger;
  countries: Country[];
  onSave: (patch: PassengerPatch) => Promise<void> | void;
  onCancel: () => void;
  onRemove?: () => void;
}) {
  const initial = {
    firstName: passenger.firstName, lastName: passenger.lastName, kind: passenger.kind,
    isHolder: passenger.isHolder, birthDate: dayOf(passenger.birthDate), residenceCountry: passenger.residenceCountry,
  };
  const [values, setValues] = useState(initial);
  const [saving, setSaving] = useState(false);
  const set = <K extends keyof typeof values>(key: K, value: (typeof values)[K]) => setValues((current) => ({ ...current, [key]: value }));

  return (
    <form className="trip-payment-form passenger-form" onSubmit={async (event) => {
      event.preventDefault();
      setSaving(true);
      try { await onSave(changed(values, initial) as PassengerPatch); } finally { setSaving(false); }
    }}>
      <div className="trip-date-fields">
        <label>Nombre<input value={values.firstName} onChange={(e) => set("firstName", e.target.value)} required /></label>
        <label>Apellido<input value={values.lastName} onChange={(e) => set("lastName", e.target.value)} required /></label>
      </div>
      <div className="trip-date-fields">
        <label>Nacimiento<input type="date" value={values.birthDate} onChange={(e) => set("birthDate", e.target.value)} /></label>
        <label>Residencia
          <CountrySelect label="Residencia" value={values.residenceCountry} countries={countries} onChange={(code) => set("residenceCountry", code || "AR")} />
        </label>
      </div>
      <div className="trip-date-fields">
        {/* Con fecha de nacimiento, si es menor lo dice la fecha a la salida. */}
        <label>Edad
          <select value={values.kind} disabled={Boolean(values.birthDate)} onChange={(e) => set("kind", e.target.value as Passenger["kind"])}>
            <option value="adulto">Adulto</option>
            <option value="menor">Menor</option>
          </select>
        </label>
        <label className="trip-check"><input type="checkbox" checked={values.isHolder} onChange={(e) => set("isHolder", e.target.checked)} />Titular</label>
      </div>
      <div className="trip-form-actions">
        <Button size="sm" type="submit" disabled={saving}>{saving ? "Guardando…" : "Guardar pasajero"}</Button>
        <Button size="sm" type="button" variant="ghost" onClick={onCancel}>Cancelar</Button>
        {onRemove && <Button size="sm" type="button" variant="ghost" className="trip-remove" onClick={onRemove}>Quitar pasajero</Button>}
      </div>
    </form>
  );
}

type Run = (action: () => Promise<void> | void) => Promise<boolean>;

function DocumentsEditor({ passenger, countries, run, onUpdatePassenger, onAddDocument, onUpdateDocument, onDeleteDocument }: DocumentActions & {
  passenger: Passenger;
  countries: Country[];
  run: Run;
  onUpdatePassenger?: (id: string, input: PassengerPatch) => Promise<void>;
}) {
  const [editing, setEditing] = useState<string | "nuevo">();
  return (
    <div className="passenger-subsection">
      <h4>Documentos</h4>
      {passenger.documents.length === 0 && <p className="trip-note">Sin documentos cargados.</p>}
      <ul className="passenger-items">
        {passenger.documents.map((document) => editing === document.id ? (
          <li key={document.id}>
            <DocumentForm document={document} countries={countries} onCancel={() => setEditing(undefined)} onSave={async (input) => {
              if (!onUpdateDocument || Object.keys(input).length === 0) return setEditing(undefined);
              if (await run(() => onUpdateDocument(document.id, input))) setEditing(undefined);
            }} />
          </li>
        ) : (
          <li key={document.id} className="passenger-item">
            <label className="trip-check">
              <input type="radio" name={`viaja-con-${passenger.id}`} aria-label={`Viaja con ${documentLabel(document, countries)}`}
                checked={document.id === passenger.travelDocumentId} disabled={!onUpdatePassenger}
                onChange={() => { if (onUpdatePassenger) void run(() => onUpdatePassenger(passenger.id, { travelDocumentId: document.id })); }} />
              <span>
                {document.type} · {countryName(countries, document.country)}{document.number ? ` · ${document.number}` : ""}
                <small>{document.expiresAt ? `Vence ${shortDate(document.expiresAt)}` : "Sin vencimiento cargado"}{document.id === passenger.travelDocumentId ? ` · viaja con este${passenger.travelDocumentManual ? ", elegido a mano" : ""}` : ""}</small>
              </span>
            </label>
            {onUpdateDocument && <Button variant="ghost" size="icon-xs" aria-label={`Editar ${documentLabel(document, countries)}`} onClick={() => setEditing(document.id)}><Pencil /></Button>}
            {onDeleteDocument && <Button variant="ghost" size="icon-xs" aria-label={`Quitar ${documentLabel(document, countries)}`} onClick={() => void run(() => onDeleteDocument(document))}><Trash2 /></Button>}
          </li>
        ))}
      </ul>
      {passenger.documents.length > 1 && (passenger.travelDocumentManual ? (
        onUpdatePassenger && (
          <Button size="sm" variant="ghost" onClick={() => void run(() => onUpdatePassenger(passenger.id, { travelDocumentId: "" }))}>
            Que lo elija el sistema
          </Button>
        )
      ) : (
        <p className="trip-note">El sistema elige con cuál viaja: el que cumple con todos los países, y el DNI si alcanza. Marcá otro para elegirlo a mano.</p>
      ))}
      {onAddDocument && (editing === "nuevo" ? (
        <DocumentForm countries={countries} onCancel={() => setEditing(undefined)} onSave={async (input) => {
          if (await run(() => onAddDocument(passenger.id, input as DocumentInput))) setEditing(undefined);
        }} />
      ) : (
        <Button size="sm" variant="outline" onClick={() => setEditing("nuevo")}><Plus />Agregar documento</Button>
      ))}
    </div>
  );
}

function DocumentForm({ document, countries, onSave, onCancel }: {
  document?: TravelDocument;
  countries: Country[];
  onSave: (input: Partial<DocumentInput>) => Promise<void> | void;
  onCancel: () => void;
}) {
  const initial = {
    type: document?.type ?? "Pasaporte", country: document?.country ?? "AR", number: document?.number ?? "",
    issuedAt: dayOf(document?.issuedAt), expiresAt: dayOf(document?.expiresAt),
  };
  const [values, setValues] = useState(initial);
  const set = <K extends keyof typeof values>(key: K, value: (typeof values)[K]) => setValues((current) => ({ ...current, [key]: value }));
  return (
    <form className="trip-payment-form" onSubmit={(event) => {
      event.preventDefault();
      void onSave((document ? changed(values, initial) : filled(values)) as Partial<DocumentInput>);
    }}>
      <div className="trip-date-fields">
        <label>Tipo
          <select value={values.type} onChange={(e) => set("type", e.target.value as TravelDocument["type"])}>
            <option value="Pasaporte">Pasaporte</option>
            <option value="DNI">DNI</option>
          </select>
        </label>
        <label>País<CountrySelect label="País del documento" value={values.country} countries={countries} onChange={(code) => set("country", code)} required /></label>
      </div>
      <label>Número<input value={values.number} onChange={(e) => set("number", e.target.value)} /></label>
      <div className="trip-date-fields">
        <label>Emitido<input type="date" value={values.issuedAt} onChange={(e) => set("issuedAt", e.target.value)} /></label>
        <label>Vence<input type="date" value={values.expiresAt} onChange={(e) => set("expiresAt", e.target.value)} /></label>
      </div>
      <div className="trip-form-actions">
        <Button size="sm" type="submit">Guardar documento</Button>
        <Button size="sm" type="button" variant="ghost" onClick={onCancel}>Cancelar</Button>
      </div>
    </form>
  );
}

function PermitsEditor({ passenger, countries, run, onAddPermit, onDeletePermit }: DocumentActions & { passenger: Passenger; countries: Country[]; run: Run }) {
  const [adding, setAdding] = useState(false);
  if (!onAddPermit && passenger.permits.length === 0) return null;
  return (
    <div className="passenger-subsection">
      <h4>Visas y autorizaciones</h4>
      {passenger.permits.length === 0 && <p className="trip-note">Ninguna cargada.</p>}
      <ul className="passenger-items">
        {passenger.permits.map((permit) => (
          <li key={permit.id} className="passenger-item">
            <span>
              {PERMIT_LABEL[permit.kind]} · {countryName(countries, permit.country)}{permit.label ? ` · ${permit.label}` : ""}
              <small>{permit.expiresAt ? `Vence ${shortDate(permit.expiresAt)}` : "Sin vencimiento cargado"}</small>
            </span>
            {onDeletePermit && <Button variant="ghost" size="icon-xs" aria-label={`Quitar ${PERMIT_LABEL[permit.kind]} de ${countryName(countries, permit.country)}`} onClick={() => void run(() => onDeletePermit(permit))}><Trash2 /></Button>}
          </li>
        ))}
      </ul>
      {onAddPermit && (adding ? (
        <PermitForm countries={countries} onCancel={() => setAdding(false)} onSave={async (input) => {
          if (await run(() => onAddPermit(passenger.id, input))) setAdding(false);
        }} />
      ) : (
        <Button size="sm" variant="outline" onClick={() => setAdding(true)}><Plus />Agregar visa o autorización</Button>
      ))}
    </div>
  );
}

function PermitForm({ countries, onSave, onCancel }: { countries: Country[]; onSave: (input: PermitInput) => Promise<void> | void; onCancel: () => void }) {
  const [values, setValues] = useState({ kind: "visa" as PermitKind, country: "", label: "", validFrom: "", expiresAt: "" });
  const set = <K extends keyof typeof values>(key: K, value: (typeof values)[K]) => setValues((current) => ({ ...current, [key]: value }));
  return (
    <form className="trip-payment-form" onSubmit={(event) => {
      event.preventDefault();
      void onSave(filled(values) as PermitInput);
    }}>
      <div className="trip-date-fields">
        <label>Qué es
          <select aria-label="Qué es" value={values.kind} onChange={(e) => set("kind", e.target.value as PermitKind)}>
            <option value="visa">Visa</option>
            <option value="autorizacion_electronica">Autorización electrónica (eTA, ESTA)</option>
            <option value="residencia">Residencia</option>
          </select>
        </label>
        <label>País<CountrySelect label="País que la otorgó" value={values.country} countries={countries} onChange={(code) => set("country", code)} required /></label>
      </div>
      <label>Detalle<input aria-label="Detalle" value={values.label} placeholder="B1/B2, ESTA…" onChange={(e) => set("label", e.target.value)} /></label>
      <div className="trip-date-fields">
        <label>Desde<input type="date" aria-label="Vale desde" value={values.validFrom} onChange={(e) => set("validFrom", e.target.value)} /></label>
        <label>Vence<input type="date" aria-label="Vence la visa" value={values.expiresAt} onChange={(e) => set("expiresAt", e.target.value)} /></label>
      </div>
      <div className="trip-form-actions">
        <Button size="sm" type="submit">Guardar visa</Button>
        <Button size="sm" type="button" variant="ghost" onClick={onCancel}>Cancelar</Button>
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

/**
 * La misma persona, lista para volver a cargarse: sus datos y el documento con
 * el que viajaba. Los demás documentos y las visas no vuelven solos.
 */
function asNewPassenger(passenger: Passenger): NewPassenger {
  const document = travelDocument(passenger);
  return {
    firstName: passenger.firstName, lastName: passenger.lastName, kind: passenger.kind,
    isHolder: passenger.isHolder, birthDate: dayOf(passenger.birthDate) || undefined,
    residenceCountry: passenger.residenceCountry,
    documentType: document?.type, documentCountry: document?.country, documentNumber: document?.number,
    documentExpiresAt: dayOf(document?.expiresAt) || undefined,
  };
}
