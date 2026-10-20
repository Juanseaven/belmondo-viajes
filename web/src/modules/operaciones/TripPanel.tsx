"use client";

import { useEffect, useState } from "react";
import { Check, ChevronDown, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { Control, Payment, TripDetail } from "./types";
import { nextControl } from "./types";

type Tab = "resumen" | "pasajeros" | "pagos" | "controles";

export function TripPanel({ trip, collapsed, onClose, onUpdateTrip, onRunControl, onCloseControl, onReopenControl, onAddPassenger, onSavePayment }: {
  trip: TripDetail;
  collapsed?: boolean;
  onClose?: () => void;
  onUpdateTrip?: (input: { destination?: string; departAt?: string; returnAt?: string }) => Promise<void>;
  onRunControl: (control: Control) => void;
  onCloseControl: (control: Control) => void;
  onReopenControl: (control: Control) => void;
  onAddPassenger: (input: { firstName: string; lastName: string }) => void;
  onSavePayment: (input: Partial<Payment>) => void;
}) {
  const [tab, setTab] = useState<Tab>("resumen");
  const [showPassengerForm, setShowPassengerForm] = useState(false);
	const [showRest, setShowRest] = useState(false);
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const next = nextControl(trip.controls);
  const rest = trip.controls.filter((control) => control.status === "abierto" && control.id !== next?.id);

  return (
    <aside className={`contact-details-panel trip-panel${collapsed ? " is-collapsed" : ""}`} aria-label="Viaje" aria-hidden={collapsed || undefined} inert={collapsed || undefined}>
      <header className="operation-head"><div><strong>Viaje #{trip.number}</strong><small>{trip.destination || "Datos por completar"}</small></div>{onClose && <Button variant="ghost" size="icon-xs" aria-label="Ocultar viaje" onClick={onClose}><X /></Button>}</header>
      <div className="detail-tabs operation-tabs" role="tablist" aria-label="Secciones del viaje">
        {(["resumen", "pasajeros", "pagos", "controles"] as Tab[]).map((value) => <button key={value} role="tab" aria-selected={tab === value} onClick={() => setTab(value)}>{value[0].toUpperCase() + value.slice(1)}</button>)}
      </div>
      <div className="trip-panel-body">
        {tab === "resumen" && <>
          <section className="trip-next"><h3>Lo que sigue</h3>{next ? <ControlRow control={next} onRun={onRunControl} onClose={onCloseControl} onReopen={onReopenControl} /> : <p>Este viaje no tiene controles pendientes.</p>}
          {rest.length > 0 && <><button type="button" className="trip-rest-trigger" aria-expanded={showRest} onClick={() => setShowRest((value) => !value)}><ChevronDown aria-hidden />Ver {rest.length} {rest.length === 1 ? "control restante" : "controles restantes"}</button>{showRest && <ul>{rest.map((control) => <ControlRow key={control.id} control={control} onRun={onRunControl} onClose={onCloseControl} onReopen={onReopenControl} />)}</ul>}</>}</section>
          <section className="detail-section"><h3>Viaje</h3><TripDetailsForm trip={trip} onSave={onUpdateTrip} /></section>
          <section className="detail-section"><h3>Contacto</h3><div className="detail-rows"><Row label="Cliente" value={trip.customer.name} /><Row label="Vendedor" value={trip.sellerName || "Sin asignar"} /><Row label="Mail" value={trip.customer.email || "Sin completar"} /><Row label="Teléfono" value={trip.customer.phoneNumber || "Sin completar"} /></div></section>
        </>}
        {tab === "pasajeros" && <section className="detail-section"><h3>Pasajeros</h3>{trip.passengers.length === 0 ? <div className="detail-empty trip-empty"><strong>Cargá los pasajeros para empezar</strong><p>Nombre y apellido alcanzan; el documento se completa después.</p></div> : <ul className="passenger-list">{trip.passengers.map((p) => <li key={p.id}><strong>{p.lastName}, {p.firstName}</strong><small>{p.isHolder ? "Titular · " : ""}{p.documentType || "Sin documento"} {p.documentNumber || ""}</small></li>)}</ul>}
          {!showPassengerForm ? <Button size="sm" onClick={() => setShowPassengerForm(true)}><Plus />Agregar pasajero</Button> : <form className="trip-inline-form" onSubmit={(event) => { event.preventDefault(); onAddPassenger({ firstName, lastName }); setFirstName(""); setLastName(""); setShowPassengerForm(false); }}><input aria-label="Nombre" value={firstName} onChange={(e) => setFirstName(e.target.value)} required /><input aria-label="Apellido" value={lastName} onChange={(e) => setLastName(e.target.value)} required /><Button size="sm" type="submit">Guardar pasajero</Button></form>}
        </section>}
        {tab === "pagos" && <PaymentTab payment={trip.payment} onSave={onSavePayment} />}
        {tab === "controles" && <section className="detail-section"><h3>Todos los controles</h3><ul className="trip-controls">{trip.controls.map((control) => <ControlRow key={control.id} control={control} onRun={onRunControl} onClose={onCloseControl} onReopen={onReopenControl} />)}</ul></section>}
      </div>
    </aside>
  );
}

function ControlRow({ control, onRun, onClose, onReopen }: { control: Control; onRun: (c: Control) => void; onClose: (c: Control) => void; onReopen: (c: Control) => void }) {
  const done = control.status === "cerrado";
  return <li className={`${control.critical ? "trip-control critical" : "trip-control"}${done ? " done" : ""}`}><span className="trip-control-mark">{done && <Check aria-hidden />}</span><span><strong>{control.title}</strong>{control.dueAt && <small>Vence {control.dueAt.slice(0, 10)}</small>}</span>{!done && control.closing === "accion" && <Button size="sm" variant="outline" onClick={() => onRun(control)}>{control.actionLabel || "Hacer ahora"}</Button>}{!done && control.closing === "manual" && <Button size="sm" variant="outline" onClick={() => onClose(control)}>Confirmar</Button>}{done && control.closing !== "derivado" && <Button size="sm" variant="ghost" onClick={() => onReopen(control)}>Reabrir</Button>}</li>;
}

function Row({ label, value }: { label: string; value: string }) { return <div className="detail-row"><span>{label}</span><strong>{value}</strong></div>; }

function TripDetailsForm({ trip, onSave }: { trip: TripDetail; onSave?: (input: { destination?: string; departAt?: string; returnAt?: string }) => Promise<void> }) {
  const [destination, setDestination] = useState(trip.destination);
  const [departAt, setDepartAt] = useState(trip.departAt?.slice(0, 10) ?? "");
  const [returnAt, setReturnAt] = useState(trip.returnAt?.slice(0, 10) ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => {
    setDestination(trip.destination);
    setDepartAt(trip.departAt?.slice(0, 10) ?? "");
    setReturnAt(trip.returnAt?.slice(0, 10) ?? "");
  }, [trip.destination, trip.departAt, trip.returnAt]);

  const originalDepart = trip.departAt?.slice(0, 10) ?? "";
  const originalReturn = trip.returnAt?.slice(0, 10) ?? "";
  const dirty = destination.trim() !== trip.destination || departAt !== originalDepart || returnAt !== originalReturn;

  return <form className="trip-details-form" onSubmit={async (event) => {
    event.preventDefault();
    if (!onSave) return;
    const input: { destination?: string; departAt?: string; returnAt?: string } = {};
    if (destination.trim() !== trip.destination) input.destination = destination.trim();
    if (departAt && departAt !== originalDepart) input.departAt = departAt;
    if (returnAt && returnAt !== originalReturn) input.returnAt = returnAt;
    setSaving(true);
    setError(undefined);
    try {
      await onSave(input);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar el viaje");
    } finally {
      setSaving(false);
    }
  }}>
    <label>Destino<input value={destination} onChange={(event) => setDestination(event.target.value)} placeholder="Sin completar" /></label>
    <div className="trip-date-fields">
      <label>Salida<input type="date" value={departAt} onChange={(event) => setDepartAt(event.target.value)} /></label>
      <label>Regreso<input type="date" value={returnAt} onChange={(event) => setReturnAt(event.target.value)} /></label>
    </div>
    {error && <p className="trip-form-error" role="alert">{error}</p>}
    <Button size="sm" type="submit" disabled={saving || !onSave || !dirty}>{saving ? "Guardando…" : "Guardar viaje"}</Button>
  </form>;
}

function PaymentTab({ payment, onSave }: { payment: Payment | null; onSave: (input: Partial<Payment>) => void }) {
  const [total, setTotal] = useState(payment?.totalUsd?.toString() ?? "");
  const [method, setMethod] = useState(payment?.method ?? "");
  return <section className="detail-section"><h3>Pagos</h3><form className="trip-payment-form" onSubmit={(event) => { event.preventDefault(); onSave({ totalUsd: total ? Number(total) : undefined, method }); }}><label>Total USD<input inputMode="decimal" value={total} onChange={(e) => setTotal(e.target.value)} /></label><label>Método<input value={method} onChange={(e) => setMethod(e.target.value)} /></label><Button size="sm" type="submit">Guardar pago</Button></form></section>;
}
