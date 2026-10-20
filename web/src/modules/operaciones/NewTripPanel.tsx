"use client";

import { useState } from "react";
import { ArrowLeft, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { PaymentInput } from "./api";

type Tab = "resumen" | "pasajeros" | "pagos" | "controles";
type TripSeed = { destination?: string; departAt?: string; returnAt?: string };

interface NewTripPanelProps {
  contactName: string;
  collapsed?: boolean;
  onClose?: () => void;
  /** Presente cuando el contacto ya tiene un viaje: vuelve a él sin crear nada. */
  onCancel?: () => void;
  onCreate: (input: TripSeed) => Promise<void>;
  onAddPassenger?: (input: { firstName: string; lastName: string }) => Promise<void>;
  onSavePayment?: (input: PaymentInput) => Promise<void>;
}

export function NewTripPanel({ contactName, collapsed, onClose, onCancel, onCreate, onAddPassenger, onSavePayment }: NewTripPanelProps) {
  const [tab, setTab] = useState<Tab>("resumen");
  const [destination, setDestination] = useState("");
  const [departAt, setDepartAt] = useState("");
  const [returnAt, setReturnAt] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [total, setTotal] = useState("");
  const [method, setMethod] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  const save = async (action: () => Promise<void>) => {
    setSaving(true);
    setError(undefined);
    try {
      await action();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar");
    } finally {
      setSaving(false);
    }
  };

  return (
    <aside className={`contact-details-panel trip-panel${collapsed ? " is-collapsed" : ""}`} aria-label="Viaje" aria-hidden={collapsed || undefined} inert={collapsed || undefined}>
      <header className="operation-head">
        <div><strong>Operación nueva</strong><small>Se crea al guardar el primer dato</small></div>
        <span className="trip-head-actions">
          {onCancel && <Button variant="ghost" size="icon-xs" aria-label="Volver al viaje" title="Volver al viaje" onClick={onCancel}><ArrowLeft /></Button>}
          {onClose && <Button variant="ghost" size="icon-xs" aria-label="Ocultar viaje" onClick={onClose}><X /></Button>}
        </span>
      </header>
      <div className="detail-tabs operation-tabs" role="tablist" aria-label="Secciones de la operación">
        {(["resumen", "pasajeros", "pagos", "controles"] as Tab[]).map((value) => (
          <button key={value} type="button" role="tab" aria-selected={tab === value} onClick={() => setTab(value)}>
            {value[0].toUpperCase() + value.slice(1)}
          </button>
        ))}
      </div>
      <div className="trip-panel-body draft-operation">
        {error && <p className="trip-form-error" role="alert">{error}</p>}
        {tab === "resumen" && <>
          <section className="detail-section">
            <h3>Viaje</h3>
            <form className="trip-details-form" onSubmit={(event) => {
              event.preventDefault();
              const input: TripSeed = {};
              if (destination.trim()) input.destination = destination.trim();
              if (departAt) input.departAt = departAt;
              if (returnAt) input.returnAt = returnAt;
              void save(() => onCreate(input));
            }}>
              <label>Destino<input value={destination} onChange={(event) => setDestination(event.target.value)} placeholder="Sin completar" /></label>
              <div className="trip-date-fields">
                <label>Salida<input type="date" value={departAt} onChange={(event) => setDepartAt(event.target.value)} /></label>
                <label>Regreso<input type="date" value={returnAt} onChange={(event) => setReturnAt(event.target.value)} /></label>
              </div>
              <Button size="sm" type="submit" disabled={saving || (!destination.trim() && !departAt && !returnAt)}>{saving ? "Guardando…" : "Guardar viaje"}</Button>
            </form>
          </section>
          <section className="detail-section">
            <h3>Contacto</h3>
            <div className="draft-contact"><span className="draft-contact-avatar">{contactName.trim().charAt(0).toUpperCase() || "?"}</span><span><strong>{contactName}</strong><small>Contacto de WhatsApp</small></span></div>
          </section>
        </>}
        {tab === "pasajeros" && <section className="detail-section">
          <h3>Pasajeros</h3>
          <div className="detail-empty trip-empty"><strong>Todavía no hay pasajeros</strong><p>Podés empezar por acá; la operación se crea al guardar.</p></div>
          <form className="trip-inline-form" onSubmit={(event) => {
            event.preventDefault();
            if (!onAddPassenger) return;
            void save(async () => {
              await onAddPassenger({ firstName: firstName.trim(), lastName: lastName.trim() });
              setFirstName("");
              setLastName("");
            });
          }}>
            <input aria-label="Nombre" placeholder="Nombre" value={firstName} onChange={(event) => setFirstName(event.target.value)} required />
            <input aria-label="Apellido" placeholder="Apellido" value={lastName} onChange={(event) => setLastName(event.target.value)} required />
            <Button size="sm" type="submit" disabled={saving || !onAddPassenger}>Guardar pasajero</Button>
          </form>
        </section>}
        {tab === "pagos" && <section className="detail-section">
          <h3>Pagos</h3>
          <form className="trip-payment-form" onSubmit={(event) => {
            event.preventDefault();
            if (!onSavePayment) return;
            void save(() => onSavePayment({ totalUsd: total ? Number(total) : undefined, method: method.trim() || undefined }));
          }}>
            <label>Total USD<input inputMode="decimal" value={total} onChange={(event) => setTotal(event.target.value)} placeholder="Sin completar" /></label>
            <label>Método<input value={method} onChange={(event) => setMethod(event.target.value)} placeholder="Sin completar" /></label>
            <Button size="sm" type="submit" disabled={saving || !onSavePayment || (!total && !method.trim())}>Guardar pago</Button>
          </form>
        </section>}
        {tab === "controles" && <section className="detail-section"><h3>Controles</h3><div className="detail-empty trip-empty"><strong>Se generan con la operación</strong><p>Aparecerán cuando guardes el primer dato.</p></div></section>}
      </div>
    </aside>
  );
}
