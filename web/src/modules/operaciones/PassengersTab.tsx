"use client";

import { useEffect, useState } from "react";
import { ChevronDown, IdCard, Pencil, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import type { DocumentInput, NewPassenger, PermitInput } from "./api";
import { CountrySelect, countryName } from "./countries";
import { dayOf, fullDate, today } from "./format";
import { Field } from "./PanelEditor";
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
  /** Un pasajero para abrir al llegar, y si hay que empezar a cargarle una visa. */
  focus?: { passengerId: string; addPermit?: boolean };
  onFocused?: () => void;
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

type Run = (action: () => Promise<void> | void) => Promise<boolean>;

/**
 * Una tarjeta por pasajero: quién es y con qué documento viaja. Al abrirla, sus
 * documentos y sus visas. Los pasajeros se cargan de a pedazos: nombre y
 * apellido alcanzan para el alta. Quitar no pregunta: se hace y queda «Deshacer».
 */
export function PassengersTab({ trip, countries = [], focus, onFocused, onAddPassenger, onUpdatePassenger, onDeletePassenger, ...docs }: PassengersTabProps) {
  const [adding, setAdding] = useState(false);
  const [removed, setRemoved] = useState<Passenger>();
  const [error, setError] = useState<string>();

  const run: Run = async (action) => {
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
    if (await run(() => onDeletePassenger(passenger))) setRemoved(passenger);
  };

  const undo = async (passenger: Passenger) => {
    if (await run(() => onAddPassenger(asNewPassenger(passenger)))) setRemoved(undefined);
  };

  return (
    <>
      {error && <p className="panel-error" role="alert">{error}</p>}
      {removed && (
        <p className="panel-undo" role="status">
          Quitaste a {removed.firstName} {removed.lastName}.
          <button type="button" onClick={() => void undo(removed)}>Deshacer</button>
        </p>
      )}
      {trip.passengers.length === 0 && !adding && (
        <div className="panel-empty">
          <strong>Cargá los pasajeros para empezar</strong>
          <p>Nombre y apellido alcanzan; el documento se completa después.</p>
        </div>
      )}
      {trip.passengers.length > 0 && (
        <ul className="pax-list">
          {trip.passengers.map((passenger) => (
            <PassengerCard key={passenger.id} passenger={passenger} trip={trip} countries={countries} run={run}
              focus={focus?.passengerId === passenger.id ? focus : undefined} onFocused={onFocused}
              onUpdatePassenger={onUpdatePassenger} onRemove={onDeletePassenger ? () => void remove(passenger) : undefined} {...docs} />
          ))}
        </ul>
      )}
      {adding ? (
        <NewPassengerForm onCancel={() => setAdding(false)} onSave={async (input) => { if (await run(() => onAddPassenger(input))) setAdding(false); }} />
      ) : (
        <button type="button" className="add-button" onClick={() => setAdding(true)}><Plus aria-hidden />Agregar pasajero</button>
      )}
    </>
  );
}

/** «Menor · 9 años»: la edad a la salida, o a hoy si el viaje no tiene fecha. */
function ageTag(passenger: Passenger, departAt?: string) {
  if (passenger.kind !== "menor") return "Adulto";
  if (!passenger.birthDate) return "Menor";
  const [by, bm, bd] = dayOf(passenger.birthDate).split("-").map(Number);
  const [y, m, d] = dayOf(departAt || today()).split("-").map(Number);
  const age = y - by - (m < bm || (m === bm && d < bd) ? 1 : 0);
  return `Menor · ${age} ${age === 1 ? "año" : "años"}`;
}

function PassengerCard({ passenger, trip, countries, run, focus, onFocused, onUpdatePassenger, onRemove, ...docs }: DocumentActions & {
  passenger: Passenger;
  trip: TripDetail;
  countries: Country[];
  run: Run;
  focus?: { passengerId: string; addPermit?: boolean };
  onFocused?: () => void;
  onUpdatePassenger?: (id: string, input: PassengerPatch) => Promise<void>;
  onRemove?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const [addingPermit, setAddingPermit] = useState(false);
  const name = `${passenger.lastName}, ${passenger.firstName}`;

  useEffect(() => {
    if (!focus) return;
    setOpen(true);
    if (focus.addPermit) setAddingPermit(true);
    onFocused?.();
  }, [focus, onFocused]);

  const document = travelDocument(passenger);
  const expires = dayOf(document?.expiresAt);
  // El criterio por defecto del control del documento: que llegue al regreso.
  // Lo que pide cada destino se ve en la pestaña Requisitos.
  const expiresTooSoon = Boolean(expires && trip.returnAt && expires < dayOf(trip.returnAt));

  if (editing) {
    return (
      <li className="panel-card pax-card">
        <PassengerForm passenger={passenger} countries={countries} onCancel={() => setEditing(false)} onSave={async (patch) => {
          if (!onUpdatePassenger || Object.keys(patch).length === 0) return setEditing(false);
          if (await run(() => onUpdatePassenger(passenger.id, patch))) setEditing(false);
        }} />
      </li>
    );
  }

  return (
    <li className={`panel-card pax-card${open ? " is-open" : ""}`} aria-label={name}>
      <div className="pax-head">
        <ContactAvatar name={`${passenger.firstName} ${passenger.lastName}`} className="avatar-36" />
        <div>
          <strong>{name}</strong>
          <span className="pax-tags">
            {passenger.isHolder && <span className="tag">Titular</span>}
            <span className="tag">{ageTag(passenger, trip.departAt)}</span>
          </span>
        </div>
        <Button variant="ghost" size="icon" className={open ? "icon-button is-open" : "icon-button"} aria-expanded={open}
          aria-label={open ? `Cerrar ${name}` : `Abrir ${name}`} onClick={() => setOpen((value) => !value)}>
          <ChevronDown aria-hidden />
        </Button>
      </div>
      <div className="pax-doc">
        <IdCard aria-hidden />
        {document ? (
          <div>
            Viaja con {document.type} {document.number || countryName(countries, document.country)}
            {expiresTooSoon
              ? <small className="is-warn">Vence el {fullDate(expires)}, antes del regreso</small>
              : <small>{expires ? `Vence el ${fullDate(expires)}` : "Sin vencimiento cargado"}</small>}
          </div>
        ) : (
          <div className="muted">Sin documento cargado</div>
        )}
      </div>
      {open && <>
        <DocumentsEditor passenger={passenger} countries={countries} run={run} onUpdatePassenger={onUpdatePassenger} {...docs} />
        <PermitsEditor passenger={passenger} countries={countries} run={run} adding={addingPermit} onAdding={setAddingPermit} {...docs} />
        <div className="pax-section pax-actions">
          {onUpdatePassenger && <Button size="sm" variant="outline" onClick={() => setEditing(true)}><Pencil aria-hidden />Editar datos</Button>}
          {onRemove && <Button size="sm" variant="ghost" className="danger-button" onClick={onRemove}>Quitar pasajero</Button>}
        </div>
      </>}
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

function PassengerForm({ passenger, countries, onSave, onCancel }: {
  passenger: Passenger;
  countries: Country[];
  onSave: (patch: PassengerPatch) => Promise<void> | void;
  onCancel: () => void;
}) {
  const initial = {
    firstName: passenger.firstName, lastName: passenger.lastName, kind: passenger.kind,
    isHolder: passenger.isHolder, birthDate: dayOf(passenger.birthDate), residenceCountry: passenger.residenceCountry,
  };
  const [values, setValues] = useState(initial);
  const [saving, setSaving] = useState(false);
  const set = <K extends keyof typeof values>(key: K, value: (typeof values)[K]) => setValues((current) => ({ ...current, [key]: value }));

  return (
    <form className="inline-form" aria-label={`Datos de ${passenger.lastName}, ${passenger.firstName}`} onSubmit={async (event) => {
      event.preventDefault();
      setSaving(true);
      try { await onSave(changed(values, initial) as PassengerPatch); } finally { setSaving(false); }
    }}>
      <div className="field-pair">
        <Field label="Nombre"><input value={values.firstName} onChange={(e) => set("firstName", e.target.value)} required /></Field>
        <Field label="Apellido"><input value={values.lastName} onChange={(e) => set("lastName", e.target.value)} required /></Field>
      </div>
      <div className="field-pair">
        <Field label="Nacimiento"><input type="date" value={values.birthDate} onChange={(e) => set("birthDate", e.target.value)} /></Field>
        <Field label="Residencia">
          <CountrySelect label="Residencia" value={values.residenceCountry} countries={countries} onChange={(code) => set("residenceCountry", code || "AR")} />
        </Field>
      </div>
      <div className="field-pair">
        {/* Con fecha de nacimiento, si es menor lo dice la fecha a la salida. */}
        <Field label="Edad">
          <select value={values.kind} disabled={Boolean(values.birthDate)} onChange={(e) => set("kind", e.target.value as Passenger["kind"])}>
            <option value="adulto">Adulto</option>
            <option value="menor">Menor</option>
          </select>
        </Field>
        <label className="check-field"><input type="checkbox" checked={values.isHolder} onChange={(e) => set("isHolder", e.target.checked)} />Titular</label>
      </div>
      <div className="inline-form-actions">
        <Button size="sm" type="submit" disabled={saving}>{saving ? "Guardando…" : "Guardar pasajero"}</Button>
        <Button size="sm" type="button" variant="ghost" onClick={onCancel}>Cancelar</Button>
      </div>
    </form>
  );
}

function DocumentsEditor({ passenger, countries, run, onUpdatePassenger, onAddDocument, onUpdateDocument, onDeleteDocument }: DocumentActions & {
  passenger: Passenger;
  countries: Country[];
  run: Run;
  onUpdatePassenger?: (id: string, input: PassengerPatch) => Promise<void>;
}) {
  const [editing, setEditing] = useState<string | "nuevo">();
  return (
    <div className="pax-section">
      <h4>
        <span>Documentos</span>
        {onAddDocument && editing !== "nuevo" && (
          <button type="button" className="link-button" aria-label="Agregar documento" onClick={() => setEditing("nuevo")}><Plus aria-hidden />Agregar</button>
        )}
      </h4>
      {passenger.documents.length === 0 && editing !== "nuevo" && <p className="pax-hint">Ninguno cargado.</p>}
      <ul className="doc-options">
        {passenger.documents.map((document) => editing === document.id ? (
          <li key={document.id}>
            <DocumentForm document={document} countries={countries} onCancel={() => setEditing(undefined)} onSave={async (input) => {
              if (!onUpdateDocument || Object.keys(input).length === 0) return setEditing(undefined);
              if (await run(() => onUpdateDocument(document.id, input))) setEditing(undefined);
            }} />
          </li>
        ) : (
          <li key={document.id} className={`doc-option${document.id === passenger.travelDocumentId ? " is-selected" : ""}`}>
            <label>
              <input type="radio" name={`viaja-con-${passenger.id}`} aria-label={`Viaja con ${documentLabel(document, countries)}`}
                checked={document.id === passenger.travelDocumentId} disabled={!onUpdatePassenger}
                onChange={() => { if (onUpdatePassenger) void run(() => onUpdatePassenger(passenger.id, { travelDocumentId: document.id })); }} />
              <span className="doc-radio" aria-hidden />
              <span>
                {document.type} · {countryName(countries, document.country)}{document.number ? ` · ${document.number}` : ""}
                <small>
                  {document.expiresAt ? `Vence el ${fullDate(document.expiresAt)}` : "Sin vencimiento cargado"}
                  {document.id === passenger.travelDocumentId ? (passenger.travelDocumentManual ? " · elegido a mano" : " · lo eligió el sistema") : ""}
                </small>
              </span>
            </label>
            {onUpdateDocument && <Button variant="ghost" size="icon-xs" aria-label={`Editar ${documentLabel(document, countries)}`} onClick={() => setEditing(document.id)}><Pencil /></Button>}
            {onDeleteDocument && <Button variant="ghost" size="icon-xs" aria-label={`Quitar ${documentLabel(document, countries)}`} onClick={() => void run(() => onDeleteDocument(document))}><Trash2 /></Button>}
          </li>
        ))}
      </ul>
      {passenger.documents.length > 1 && (passenger.travelDocumentManual ? (
        onUpdatePassenger && (
          <button type="button" className="link-button" onClick={() => void run(() => onUpdatePassenger(passenger.id, { travelDocumentId: "" }))}>
            Que lo elija el sistema
          </button>
        )
      ) : (
        <p className="pax-hint">El sistema elige el que sirve para todos los países, y el DNI si alcanza. Tocá otro para elegirlo vos.</p>
      ))}
      {editing === "nuevo" && onAddDocument && (
        <DocumentForm countries={countries} onCancel={() => setEditing(undefined)} onSave={async (input) => {
          if (await run(() => onAddDocument(passenger.id, input as DocumentInput))) setEditing(undefined);
        }} />
      )}
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
    <form className="inline-form" onSubmit={(event) => {
      event.preventDefault();
      void onSave((document ? changed(values, initial) : filled(values)) as Partial<DocumentInput>);
    }}>
      <div className="field-pair">
        <Field label="Tipo">
          <select value={values.type} onChange={(e) => set("type", e.target.value as TravelDocument["type"])}>
            <option value="Pasaporte">Pasaporte</option>
            <option value="DNI">DNI</option>
          </select>
        </Field>
        <Field label="País"><CountrySelect label="País del documento" value={values.country} countries={countries} onChange={(code) => set("country", code)} required /></Field>
      </div>
      <Field label="Número"><input value={values.number} onChange={(e) => set("number", e.target.value)} /></Field>
      <div className="field-pair">
        <Field label="Emitido"><input type="date" value={values.issuedAt} onChange={(e) => set("issuedAt", e.target.value)} /></Field>
        <Field label="Vence"><input type="date" value={values.expiresAt} onChange={(e) => set("expiresAt", e.target.value)} /></Field>
      </div>
      <div className="inline-form-actions">
        <Button size="sm" type="submit">Guardar documento</Button>
        <Button size="sm" type="button" variant="ghost" onClick={onCancel}>Cancelar</Button>
      </div>
    </form>
  );
}

function PermitsEditor({ passenger, countries, run, adding, onAdding, onAddPermit, onDeletePermit }: DocumentActions & {
  passenger: Passenger;
  countries: Country[];
  run: Run;
  adding: boolean;
  onAdding: (adding: boolean) => void;
}) {
  if (!onAddPermit && passenger.permits.length === 0) return null;
  return (
    <div className="pax-section">
      <h4>
        <span>Visas y permisos</span>
        {onAddPermit && !adding && (
          <button type="button" className="link-button" aria-label="Agregar visa o autorización" onClick={() => onAdding(true)}><Plus aria-hidden />Agregar</button>
        )}
      </h4>
      {passenger.permits.length === 0 && !adding && <p className="pax-hint">Ninguno cargado.</p>}
      {passenger.permits.length > 0 && (
        <ul className="doc-options">
          {passenger.permits.map((permit) => (
            <li key={permit.id} className="doc-option is-static">
              <span>
                {PERMIT_LABEL[permit.kind]} · {countryName(countries, permit.country)}{permit.label ? ` · ${permit.label}` : ""}
                <small>{permit.expiresAt ? `Vence el ${fullDate(permit.expiresAt)}` : "Sin vencimiento cargado"}</small>
              </span>
              {onDeletePermit && <Button variant="ghost" size="icon-xs" aria-label={`Quitar ${PERMIT_LABEL[permit.kind]} de ${countryName(countries, permit.country)}`} onClick={() => void run(() => onDeletePermit(permit))}><Trash2 /></Button>}
            </li>
          ))}
        </ul>
      )}
      {adding && onAddPermit && (
        <PermitForm countries={countries} onCancel={() => onAdding(false)} onSave={async (input) => {
          if (await run(() => onAddPermit(passenger.id, input))) onAdding(false);
        }} />
      )}
    </div>
  );
}

function PermitForm({ countries, onSave, onCancel }: { countries: Country[]; onSave: (input: PermitInput) => Promise<void> | void; onCancel: () => void }) {
  const [values, setValues] = useState({ kind: "visa" as PermitKind, country: "", label: "", validFrom: "", expiresAt: "" });
  const set = <K extends keyof typeof values>(key: K, value: (typeof values)[K]) => setValues((current) => ({ ...current, [key]: value }));
  return (
    <form className="inline-form" onSubmit={(event) => {
      event.preventDefault();
      void onSave(filled(values) as PermitInput);
    }}>
      <div className="field-pair">
        <Field label="Qué es">
          <select aria-label="Qué es" value={values.kind} onChange={(e) => set("kind", e.target.value as PermitKind)}>
            <option value="visa">Visa</option>
            <option value="autorizacion_electronica">Autorización electrónica (eTA, ESTA)</option>
            <option value="residencia">Residencia</option>
          </select>
        </Field>
        <Field label="País"><CountrySelect label="País que la otorgó" value={values.country} countries={countries} onChange={(code) => set("country", code)} required /></Field>
      </div>
      <Field label="Detalle"><input aria-label="Detalle" value={values.label} placeholder="B1/B2, ESTA…" onChange={(e) => set("label", e.target.value)} /></Field>
      <div className="field-pair">
        <Field label="Desde"><input type="date" aria-label="Vale desde" value={values.validFrom} onChange={(e) => set("validFrom", e.target.value)} /></Field>
        <Field label="Vence"><input type="date" aria-label="Vence la visa" value={values.expiresAt} onChange={(e) => set("expiresAt", e.target.value)} /></Field>
      </div>
      <div className="inline-form-actions">
        <Button size="sm" type="submit">Guardar visa</Button>
        <Button size="sm" type="button" variant="ghost" onClick={onCancel}>Cancelar</Button>
      </div>
    </form>
  );
}

function NewPassengerForm({ onSave, onCancel }: { onSave: (input: NewPassenger) => Promise<void>; onCancel: () => void }) {
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  return (
    <form className="panel-card inline-form" aria-label="Pasajero nuevo" onSubmit={(event) => {
      event.preventDefault();
      void onSave({ firstName: firstName.trim(), lastName: lastName.trim() });
    }}>
      <div className="field-pair">
        <Field label="Nombre"><input value={firstName} onChange={(e) => setFirstName(e.target.value)} required autoFocus /></Field>
        <Field label="Apellido"><input value={lastName} onChange={(e) => setLastName(e.target.value)} required /></Field>
      </div>
      <p className="pax-hint">Nombre y apellido alcanzan; el documento se completa después.</p>
      <div className="inline-form-actions">
        <Button size="sm" type="submit">Guardar pasajero</Button>
        <Button size="sm" type="button" variant="ghost" onClick={onCancel}>Cancelar</Button>
      </div>
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
