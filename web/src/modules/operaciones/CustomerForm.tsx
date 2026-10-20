"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import type { Customer, CustomerUpdate } from "./types";

const fromCustomer = (customer: Customer) => ({
  name: customer.name, email: customer.email ?? "", phoneNumber: customer.phoneNumber ?? "",
  documentType: customer.documentType ?? "", documentNumber: customer.documentNumber ?? "",
});

/**
 * El cliente es de la agencia, no de WhatsApp: mail, teléfono y documento son
 * lo que WhatsApp no da. Mail y teléfono cierran solos el control de contacto.
 */
export function CustomerForm({ customer, onSave }: { customer: Customer; onSave?: (input: CustomerUpdate) => Promise<void> }) {
  const [values, setValues] = useState(() => fromCustomer(customer));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const initial = fromCustomer(customer);
  const set = (key: keyof typeof values, value: string) => setValues((current) => ({ ...current, [key]: value }));

  // Por contenido y no por objeto: la ficha se recarga ante cualquier evento
  // del módulo, y eso no debe borrar lo que se está tipeando.
  const saved = JSON.stringify(initial);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { setValues(fromCustomer(customer)); }, [saved]);

  const patch: CustomerUpdate = {};
  for (const key of Object.keys(values) as (keyof typeof values)[]) {
    if (values[key].trim() !== initial[key]) patch[key] = values[key].trim();
  }
  const dirty = Object.keys(patch).length > 0;

  return (
    <form className="trip-details-form" onSubmit={async (event) => {
      event.preventDefault();
      if (!onSave || !dirty) return;
      setSaving(true);
      setError(undefined);
      try {
        await onSave(patch);
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : "No se pudo guardar el cliente");
      } finally {
        setSaving(false);
      }
    }}>
      <label>Nombre<input value={values.name} onChange={(e) => set("name", e.target.value)} required /></label>
      <div className="trip-date-fields">
        <label>Mail<input type="email" value={values.email} onChange={(e) => set("email", e.target.value)} placeholder="Sin completar" /></label>
        <label>Teléfono<input type="tel" value={values.phoneNumber} onChange={(e) => set("phoneNumber", e.target.value)} placeholder="Sin completar" /></label>
      </div>
      <div className="trip-date-fields">
        <label>Tipo de documento
          <select value={values.documentType} onChange={(e) => set("documentType", e.target.value)}>
            <option value="">Sin cargar</option>
            <option value="DNI">DNI</option>
            <option value="Pasaporte">Pasaporte</option>
          </select>
        </label>
        <label>Número de documento<input value={values.documentNumber} onChange={(e) => set("documentNumber", e.target.value)} placeholder="Sin completar" /></label>
      </div>
      {error && <p className="trip-form-error" role="alert">{error}</p>}
      <Button size="sm" type="submit" disabled={saving || !onSave || !dirty}>{saving ? "Guardando…" : "Guardar cliente"}</Button>
    </form>
  );
}
