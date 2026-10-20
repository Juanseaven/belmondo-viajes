"use client";

import { useEffect, useState } from "react";
import { Check, ChevronDown, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ItineraryInput, NewPassenger, PaymentInput } from "./api";
import { CustomerForm } from "./CustomerForm";
import { dayOf, shortDate, shortInstant, today } from "./format";
import { PassengersTab, type DocumentActions, type PassengerPatch } from "./PassengersTab";
import { PaymentTab } from "./PaymentTab";
import { RequirementsTab } from "./RequirementsTab";
import { DEFAULT_ORIGIN, TripRoute, toStops, type StopDraft } from "./TripRoute";
import type { Control, Country, CustomerUpdate, Passenger, Requirement, Seller, TripDetail, TripStatus, TripSummary, TripUpdate } from "./types";
import { nextControl } from "./types";

type Tab = "resumen" | "pasajeros" | "requisitos" | "pagos" | "controles";

const STATUSES: { value: TripStatus; label: string }[] = [
  { value: "consulta", label: "Consulta" },
  { value: "cotizado", label: "Cotizado" },
  { value: "confirmado", label: "Confirmado" },
  { value: "en viaje", label: "En viaje" },
  { value: "cerrado", label: "Cerrado" },
];

export interface TripPanelProps extends DocumentActions {
  trip: TripDetail;
  /** El catálogo de países, para elegirlos por nombre. */
  countries?: Country[];
  /** Los viajes del contacto. Con más de uno, el encabezado deja cambiar. */
  trips?: TripSummary[];
  sellers?: Seller[];
  collapsed?: boolean;
  onClose?: () => void;
  onSelectTrip?: (tripId: string) => void;
  onNewTrip?: () => void;
  onUpdateTrip?: (input: TripUpdate) => Promise<void>;
  onRunControl: (control: Control) => void;
  onCloseControl: (control: Control) => void;
  onReopenControl: (control: Control) => void;
  onAddPassenger: (input: NewPassenger) => Promise<void> | void;
  onUpdatePassenger?: (id: string, input: PassengerPatch) => Promise<void>;
  onDeletePassenger?: (passenger: Passenger) => Promise<void>;
  onSavePayment: (input: PaymentInput) => Promise<void> | void;
  onUpdateCustomer?: (input: CustomerUpdate) => Promise<void>;
  onSaveItinerary?: (input: ItineraryInput) => Promise<void>;
  onConfirmRequirement?: (requirement: Requirement) => Promise<void>;
}

export function TripPanel({
  trip, trips, sellers, countries = [], collapsed, onClose, onSelectTrip, onNewTrip, onUpdateTrip,
  onRunControl, onCloseControl, onReopenControl, onAddPassenger, onUpdatePassenger,
  onDeletePassenger, onSavePayment, onUpdateCustomer, onSaveItinerary, onConfirmRequirement, ...documentActions
}: TripPanelProps) {
  const [tab, setTab] = useState<Tab>("resumen");
  const [showRest, setShowRest] = useState(false);
  const next = nextControl(trip.controls);
  const rest = trip.controls.filter((control) => control.status === "abierto" && control.id !== next?.id);

  return (
    <aside className={`contact-details-panel trip-panel${collapsed ? " is-collapsed" : ""}`} aria-label="Viaje" aria-hidden={collapsed || undefined} inert={collapsed || undefined}>
      <header className="operation-head">
        <div>
          {trips && trips.length > 1 && onSelectTrip ? <>
            <select className="trip-switcher" aria-label="Viaje del contacto" value={trip.id} onChange={(event) => onSelectTrip(event.target.value)}>
              {trips.map((item) => (
                <option key={item.id} value={item.id}>Viaje #{item.number}{item.destination ? ` · ${item.destination}` : ""}</option>
              ))}
            </select>
            <small>{trips.length} viajes de este contacto</small>
          </> : <>
            <strong>Viaje #{trip.number}</strong>
            <small>{trip.destination || "Datos por completar"}</small>
          </>}
        </div>
        <span className="trip-head-actions">
          {onNewTrip && <Button variant="ghost" size="icon-xs" aria-label="Nuevo viaje" title="Nuevo viaje" onClick={onNewTrip}><Plus /></Button>}
          {onClose && <Button variant="ghost" size="icon-xs" aria-label="Ocultar viaje" onClick={onClose}><X /></Button>}
        </span>
      </header>
      <TripHeadFields trip={trip} sellers={sellers} onUpdateTrip={onUpdateTrip} />
      <div className="detail-tabs operation-tabs" role="tablist" aria-label="Secciones del viaje">
        {(["resumen", "pasajeros", "requisitos", "pagos", "controles"] as Tab[]).map((value) => <button key={value} type="button" role="tab" aria-selected={tab === value} onClick={() => setTab(value)}>{value[0].toUpperCase() + value.slice(1)}</button>)}
      </div>
      <div className="trip-panel-body">
        {tab === "resumen" && <>
          <section className="trip-next"><h3>Lo que sigue</h3>{next ? <ul><ControlRow control={next} onRun={onRunControl} onClose={onCloseControl} onReopen={onReopenControl} /></ul> : <p>Este viaje no tiene controles pendientes.</p>}
          {rest.length > 0 && <><button type="button" className="trip-rest-trigger" aria-expanded={showRest} onClick={() => setShowRest((value) => !value)}><ChevronDown aria-hidden />Ver {rest.length} {rest.length === 1 ? "control restante" : "controles restantes"}</button>{showRest && <ul>{rest.map((control) => <ControlRow key={control.id} control={control} onRun={onRunControl} onClose={onCloseControl} onReopen={onReopenControl} />)}</ul>}</>}</section>
          <section className="detail-section"><h3>Viaje</h3><TripForm trip={trip} countries={countries} onUpdateTrip={onUpdateTrip} onSaveItinerary={onSaveItinerary} /></section>
          <section className="detail-section"><h3>Cliente</h3><CustomerForm customer={trip.customer} onSave={onUpdateCustomer} /><p className="trip-note">Cliente desde {shortDate(trip.customer.customerSince)}</p></section>
        </>}
        {tab === "pasajeros" && <PassengersTab trip={trip} countries={countries} onAddPassenger={onAddPassenger} onUpdatePassenger={onUpdatePassenger} onDeletePassenger={onDeletePassenger} {...documentActions} />}
        {tab === "requisitos" && <RequirementsTab trip={trip} countries={countries} onConfirm={onConfirmRequirement} />}
        {tab === "pagos" && <PaymentTab payment={trip.payment} onSave={onSavePayment} />}
        {tab === "controles" && <section className="detail-section"><h3>Todos los controles</h3><ul className="trip-controls">{trip.controls.map((control) => <ControlRow key={control.id} control={control} onRun={onRunControl} onClose={onCloseControl} onReopen={onReopenControl} />)}</ul></section>}
      </div>
    </aside>
  );
}

/** Estado y vendedor se guardan al elegirlos: no hay nada que confirmar. */
function TripHeadFields({ trip, sellers, onUpdateTrip }: { trip: TripDetail; sellers?: Seller[]; onUpdateTrip?: (input: TripUpdate) => Promise<void> }) {
  const [error, setError] = useState<string>();
  const options = sellers ?? [];
  // Un vendedor dado de baja ya no figura en la lista, pero el viaje lo sigue
  // nombrando: se agrega para que el selector no mienta diciendo «Sin asignar».
  const sellerOptions = trip.sellerUserId && !options.some((seller) => seller.id === trip.sellerUserId)
    ? [...options, { id: trip.sellerUserId, name: trip.sellerName || "Vendedor anterior" }]
    : options;

  const save = async (input: TripUpdate) => {
    if (!onUpdateTrip) return;
    setError(undefined);
    try {
      await onUpdateTrip(input);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar el viaje");
    }
  };

  return (
    <div className="trip-head-fields">
      <span>
        <small>Estado</small>
        <select aria-label="Estado del viaje" value={trip.status} disabled={!onUpdateTrip} onChange={(event) => void save({ status: event.target.value as TripStatus })}>
          {STATUSES.map((status) => <option key={status.value} value={status.value}>{status.label}</option>)}
        </select>
      </span>
      <span>
        <small>Vendedor</small>
        <select aria-label="Vendedor" value={trip.sellerUserId ?? ""} disabled={!onUpdateTrip} onChange={(event) => void save({ sellerUserId: event.target.value })}>
          <option value="">Sin asignar</option>
          {sellerOptions.map((seller) => <option key={seller.id} value={seller.id}>{seller.name}</option>)}
        </select>
      </span>
      {error && <p className="trip-form-error" role="alert">{error}</p>}
    </div>
  );
}

function ControlRow({ control, onRun, onClose, onReopen }: { control: Control; onRun: (c: Control) => void; onClose: (c: Control) => void; onReopen: (c: Control) => void }) {
  const done = control.status === "cerrado";
  const overdue = !done && control.dueAt !== undefined && dayOf(control.dueAt) < today();
  return (
    <li className={`trip-control${control.critical ? " critical" : ""}${done ? " done" : ""}`}>
      <span className="trip-control-mark">{done && <Check aria-hidden />}</span>
      <span>
        <strong>{control.title}</strong>
        {done ? <small>{closedBy(control)}</small> : control.dueAt && <small className={overdue ? "trip-warn" : undefined}>{overdue ? "Venció" : "Vence"} {shortDate(control.dueAt)}</small>}
      </span>
      {!done && control.closing === "accion" && <Button size="sm" variant="outline" onClick={() => onRun(control)}>{control.actionLabel || "Hacer ahora"}</Button>}
      {!done && control.closing === "manual" && <Button size="sm" variant="outline" onClick={() => onClose(control)}>Confirmar</Button>}
      {/* Sin prueba, el cierre fue del dato: lo gobierna el dato, no una persona. */}
      {done && control.closing !== "derivado" && control.evidence.length > 0 && <Button size="sm" variant="ghost" onClick={() => onReopen(control)}>Reabrir</Button>}
    </li>
  );
}

/** Todo dice quién y cuándo: nadie se acuerda de un viaje que no vendió. */
function closedBy(control: Control) {
  const last = [...control.evidence].sort((a, b) => b.closedAt.localeCompare(a.closedAt))[0];
  // Un derivado, o uno por acción que el dato cerró antes de que hiciera falta
  // avisar: no hubo persona ni mensaje.
  if (control.closing === "derivado" || !last) return "Dato cargado";
  return `Cerrado${last.closedByName ? ` por ${last.closedByName}` : ""} · ${shortInstant(last.closedAt)}`;
}

/**
 * El viaje: de dónde sale, a dónde va y cuándo. Un solo botón guarda todo; el
 * origen y los países van juntos porque el origen no puede estar entre ellos.
 * El nombre del viaje no se escribe: sale de los destinos.
 */
function TripForm({ trip, countries, onUpdateTrip, onSaveItinerary }: {
  trip: TripDetail;
  countries: Country[];
  onUpdateTrip?: (input: TripUpdate) => Promise<void>;
  onSaveItinerary?: (input: ItineraryInput) => Promise<void>;
}) {
  const savedOrigin = trip.originCountry || DEFAULT_ORIGIN;
  const fromTrip = (): StopDraft[] => (trip.itinerary ?? []).map((stop) => ({ ...stop, arriveAt: dayOf(stop.arriveAt), leaveAt: dayOf(stop.leaveAt) }));
  const [origin, setOrigin] = useState(savedOrigin);
  const [stops, setStops] = useState(fromTrip);
  const [departAt, setDepartAt] = useState(dayOf(trip.departAt));
  const [returnAt, setReturnAt] = useState(dayOf(trip.returnAt));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const itineraryKey = JSON.stringify(trip.itinerary ?? []);

  useEffect(() => { setOrigin(savedOrigin); setStops(fromTrip()); }, [savedOrigin, itineraryKey]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setDepartAt(dayOf(trip.departAt));
    setReturnAt(dayOf(trip.returnAt));
  }, [trip.departAt, trip.returnAt]);

  const routeDirty = origin !== savedOrigin || JSON.stringify(toStops(stops)) !== JSON.stringify(toStops(fromTrip()));
  const datesDirty = departAt !== dayOf(trip.departAt) || returnAt !== dayOf(trip.returnAt);

  return <form className="trip-details-form" onSubmit={async (event) => {
    event.preventDefault();
    setSaving(true);
    setError(undefined);
    try {
      if (routeDirty && onSaveItinerary) await onSaveItinerary({ originCountry: origin, itinerary: toStops(stops) });
      if (datesDirty && onUpdateTrip) {
        const input: TripUpdate = {};
        if (departAt) input.departAt = departAt;
        if (returnAt) input.returnAt = returnAt;
        await onUpdateTrip(input);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar el viaje");
    } finally {
      setSaving(false);
    }
  }}>
    <TripRoute origin={origin} stops={stops} countries={countries} onOrigin={setOrigin} onStops={setStops} withStopDates />
    {stops.length === 0 && <p className="trip-note">Sin países cargados: con ellos se buscan los requisitos de entrada.</p>}
    <div className="trip-date-fields">
      <label>Salida<input type="date" value={departAt} onChange={(event) => setDepartAt(event.target.value)} /></label>
      <label>Regreso<input type="date" value={returnAt} onChange={(event) => setReturnAt(event.target.value)} /></label>
    </div>
    {error && <p className="trip-form-error" role="alert">{error}</p>}
    <Button size="sm" type="submit" disabled={saving || (!routeDirty && !datesDirty) || (routeDirty && !onSaveItinerary) || (datesDirty && !onUpdateTrip)}>
      {saving ? "Guardando…" : "Guardar viaje"}
    </Button>
  </form>;
}
