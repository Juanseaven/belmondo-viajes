"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import type { PaymentInput } from "./api";
import { dayOf, money, parseAmount, shortDate, shortInstant, today } from "./format";
import type { Payment } from "./types";

const METHODS = ["Transferencia", "Efectivo", "Tarjeta de crédito", "Tarjeta de débito", "Mercado Pago"];

const AMOUNTS = [
  ["totalUsd", "Total USD"],
  ["rate", "Cambio del día"],
  ["totalArs", "Total en pesos"],
  ["depositUsd", "Seña USD"],
  ["nonRefundableUsd", "No reembolsable USD"],
] as const;

type AmountKey = (typeof AMOUNTS)[number][0];

const fromPayment = (payment: Payment | null) => ({
  totalUsd: payment?.totalUsd?.toString().replace(".", ",") ?? "",
  rate: payment?.rate?.toString().replace(".", ",") ?? "",
  totalArs: payment?.totalArs?.toString().replace(".", ",") ?? "",
  depositUsd: payment?.depositUsd?.toString().replace(".", ",") ?? "",
  nonRefundableUsd: payment?.nonRefundableUsd?.toString().replace(".", ",") ?? "",
  method: payment?.method ?? "",
  depositDueAt: dayOf(payment?.depositDueAt),
  depositPaid: Boolean(payment?.depositPaidAt),
});

/**
 * El pago se guarda entero (PUT): el formulario siempre manda todos los campos,
 * así que un campo vacío borra el dato. El total en pesos es un campo propio y
 * no USD × cambio: la agencia todavía no definió cómo se calcula.
 */
export function PaymentTab({ payment, onSave }: { payment: Payment | null; onSave: (input: PaymentInput) => Promise<void> | void }) {
  const [values, setValues] = useState(() => fromPayment(payment));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const set = <K extends keyof typeof values>(key: K, value: (typeof values)[K]) => setValues((current) => ({ ...current, [key]: value }));

  // Otra persona puede guardar el pago mientras esta pestaña está abierta: lo
  // que llega por el websocket reemplaza lo que se ve. La dependencia es el
  // contenido y no el objeto, porque la ficha se recarga ante cualquier evento
  // del módulo y eso borraría lo que se está tipeando.
  const saved = JSON.stringify(payment);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { setValues(fromPayment(payment)); }, [saved]);

  const depositLate = !payment?.depositPaidAt && payment?.depositDueAt && dayOf(payment.depositDueAt) < today();

  return (
    <section className="detail-section">
      <h3>Pagos</h3>
      {payment?.depositPaidAt && <p className="trip-note">Seña cobrada el {shortInstant(payment.depositPaidAt)}{payment.depositUsd !== undefined && ` · USD ${money(payment.depositUsd)}`}</p>}
      {depositLate && <p className="trip-warn">La seña venció el {shortDate(payment!.depositDueAt!)} y no figura cobrada</p>}
      <form className="trip-payment-form" onSubmit={async (event) => {
        event.preventDefault();
        const amounts = {} as Record<AmountKey, number | undefined>;
        const invalid: string[] = [];
        for (const [key, label] of AMOUNTS) {
          const amount = parseAmount(values[key]);
          if (Number.isNaN(amount)) invalid.push(label);
          amounts[key] = amount;
        }
        if (invalid.length > 0) {
          setError(`No se entiende el monto de ${invalid.join(", ")}. Escribilo con números, por ejemplo 1450,50.`);
          return;
        }
        setSaving(true);
        setError(undefined);
        try {
          await onSave({
            totalUsd: amounts.totalUsd, rate: amounts.rate, totalArs: amounts.totalArs,
            method: values.method.trim() || undefined,
            depositUsd: amounts.depositUsd, depositDueAt: values.depositDueAt || undefined,
            depositPaid: values.depositPaid, nonRefundableUsd: amounts.nonRefundableUsd,
          });
        } catch (cause) {
          setError(cause instanceof Error ? cause.message : "No se pudo guardar el pago");
        } finally {
          setSaving(false);
        }
      }}>
        {error && <p className="trip-form-error" role="alert">{error}</p>}
        <div className="trip-date-fields">
          <label>Total USD<input inputMode="decimal" value={values.totalUsd} onChange={(e) => set("totalUsd", e.target.value)} placeholder="Sin completar" /></label>
          <label>Cambio del día<input inputMode="decimal" value={values.rate} onChange={(e) => set("rate", e.target.value)} placeholder="Sin completar" /></label>
        </div>
        <div className="trip-date-fields">
          <label>Total en pesos<input inputMode="decimal" value={values.totalArs} onChange={(e) => set("totalArs", e.target.value)} placeholder="Sin completar" /></label>
          <label>Método<input list="trip-payment-methods" value={values.method} onChange={(e) => set("method", e.target.value)} placeholder="Sin completar" /></label>
        </div>
        <datalist id="trip-payment-methods">{METHODS.map((method) => <option key={method} value={method} />)}</datalist>
        <div className="trip-date-fields">
          <label>Seña USD<input inputMode="decimal" value={values.depositUsd} onChange={(e) => set("depositUsd", e.target.value)} placeholder="Sin completar" /></label>
          <label>Vence la seña<input type="date" value={values.depositDueAt} onChange={(e) => set("depositDueAt", e.target.value)} /></label>
        </div>
        <label className="trip-check"><input type="checkbox" checked={values.depositPaid} onChange={(e) => set("depositPaid", e.target.checked)} />Seña cobrada</label>
        <label>No reembolsable USD<input inputMode="decimal" value={values.nonRefundableUsd} onChange={(e) => set("nonRefundableUsd", e.target.value)} placeholder="Sin completar" /></label>
        <Button size="sm" type="submit" disabled={saving}>{saving ? "Guardando…" : "Guardar pago"}</Button>
      </form>
    </section>
  );
}
