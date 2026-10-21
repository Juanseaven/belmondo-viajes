"use client";

import { useState } from "react";
import { Check, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { PaymentInput } from "./api";
import { dayMonth, dayOf, money, parseAmount, shortDay, today } from "./format";
import { Field, PanelEditor, describeChanges } from "./PanelEditor";
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
 * Pagos se lee como plata: el total en grande, los pesos debajo, una barra de
 * cobrado contra total. Cobrado es la seña si figura cobrada; si no, cero.
 */
export function PaymentTab({ payment, onEdit }: { payment: Payment | null; onEdit: () => void }) {
  if (payment?.totalUsd === undefined) {
    return (
      <div className="panel-empty">
        <strong>Todavía no hay cotización</strong>
        <p>Cargá el total y la seña: con eso el sistema avisa cuándo cobrarla.</p>
        <Button size="sm" onClick={onEdit}>Cargar cotización</Button>
      </div>
    );
  }

  const total = payment.totalUsd;
  const collected = payment.depositPaidAt && payment.depositUsd ? payment.depositUsd : 0;
  const share = total > 0 ? Math.min(100, (collected / total) * 100) : 0;

  return (
    <>
      <section className="panel-card money">
        <small>Total del viaje</small>
        <strong className="money-total">USD {money(total)}</strong>
        {payment.totalArs !== undefined && (
          <span className="money-ars">$ {money(payment.totalArs)}{payment.rate !== undefined && ` al cambio de $ ${money(payment.rate)}`}</span>
        )}
        <div className="money-track" aria-hidden><i style={{ width: `${share}%` }} /></div>
        <div className="money-split">
          <span>Cobrado <b>USD {money(collected)}</b></span>
          <span>Falta <b>USD {money(Math.max(0, total - collected))}</b></span>
        </div>
      </section>

      <Deposit payment={payment} />

      {(payment.method || payment.nonRefundableUsd !== undefined) && (
        <section className="panel-card">
          <dl className="panel-kv is-split">
            <dt>Forma de pago</dt>
            <dd>{payment.method || <span className="muted">Sin cargar</span>}</dd>
            <dt>No reembolsable</dt>
            <dd>{payment.nonRefundableUsd !== undefined ? `USD ${money(payment.nonRefundableUsd)}` : <span className="muted">Sin cargar</span>}</dd>
          </dl>
        </section>
      )}

      <div><Button size="sm" variant="outline" onClick={onEdit}><Pencil aria-hidden />Editar pago</Button></div>
    </>
  );
}

/** La seña: con tilde verde si se cobró; ocre con el vencimiento si no; roja si venció. */
function Deposit({ payment }: { payment: Payment }) {
  if (!payment.depositUsd) return null;
  const amount = `USD ${money(payment.depositUsd)}`;
  if (payment.depositPaidAt) {
    return (
      <section className="panel-card deposit is-paid">
        <h3>Seña</h3>
        <div className="deposit-row">
          <span className="deposit-mark" aria-hidden><Check /></span>
          <span>{amount} cobrada<small>El {dayMonth(payment.depositPaidAt.slice(0, 10))}</small></span>
        </div>
      </section>
    );
  }
  const late = payment.depositDueAt !== undefined && dayOf(payment.depositDueAt) < today();
  return (
    <section className={`panel-card deposit ${late ? "is-late" : "is-due"}`}>
      <h3>Seña</h3>
      <div className="deposit-row">
        <span className="deposit-mark" aria-hidden>!</span>
        <span>{amount} sin cobrar
          <small>{payment.depositDueAt ? `${late ? "Venció" : "Vence"} el ${shortDay(payment.depositDueAt)}` : "Sin fecha para cobrarla"}</small>
        </span>
      </div>
    </section>
  );
}

/**
 * El formulario del pago, con el esquema de Editar. Se guarda entero (PUT):
 * siempre manda todos los campos, así que un campo vacío borra el dato. El
 * total en pesos es un campo propio y no USD × cambio: la agencia todavía no
 * definió cómo se calcula.
 */
export function PaymentEditor({ tripNumber, payment, onSave, onBack }: {
  tripNumber: number;
  payment: Payment | null;
  onSave: (input: PaymentInput) => Promise<void> | void;
  onBack: () => void;
}) {
  const initial = fromPayment(payment);
  const [values, setValues] = useState(initial);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const set = <K extends keyof typeof values>(key: K, value: (typeof values)[K]) => setValues((current) => ({ ...current, [key]: value }));

  const changed = describeChanges([
    values.totalUsd !== initial.totalUsd || values.rate !== initial.rate || values.totalArs !== initial.totalArs ? "el total" : "",
    values.depositUsd !== initial.depositUsd || values.depositDueAt !== initial.depositDueAt || values.depositPaid !== initial.depositPaid ? "la seña" : "",
    values.method !== initial.method || values.nonRefundableUsd !== initial.nonRefundableUsd ? "las condiciones" : "",
  ].filter(Boolean));

  const save = async () => {
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
  };

  return (
    <PanelEditor title={`Pago del viaje #${tripNumber}`} saveLabel="Guardar pago" changes={changed} saving={saving} error={error}
      canSave={Boolean(changed)} onSave={() => void save()} onBack={onBack}>
      <section className="editor-section">
        <h4>Total</h4>
        <Field label="Total USD"><input inputMode="decimal" value={values.totalUsd} onChange={(e) => set("totalUsd", e.target.value)} placeholder="Sin cargar" /></Field>
        <div className="field-pair">
          <Field label="Cambio del día"><input inputMode="decimal" value={values.rate} onChange={(e) => set("rate", e.target.value)} placeholder="Sin cargar" /></Field>
          <Field label="Total en pesos"><input inputMode="decimal" value={values.totalArs} onChange={(e) => set("totalArs", e.target.value)} placeholder="Sin cargar" /></Field>
        </div>
      </section>
      <section className="editor-section">
        <h4>Seña</h4>
        <div className="field-pair">
          <Field label="Seña USD"><input inputMode="decimal" value={values.depositUsd} onChange={(e) => set("depositUsd", e.target.value)} placeholder="Sin cargar" /></Field>
          <Field label="Vence la seña"><input type="date" value={values.depositDueAt} onChange={(e) => set("depositDueAt", e.target.value)} /></Field>
        </div>
        <label className="check-field"><input type="checkbox" checked={values.depositPaid} onChange={(e) => set("depositPaid", e.target.checked)} />Seña cobrada</label>
      </section>
      <section className="editor-section">
        <h4>Condiciones</h4>
        <div className="field-pair">
          <Field label="Forma de pago"><input list="trip-payment-methods" value={values.method} onChange={(e) => set("method", e.target.value)} placeholder="Elegir" /></Field>
          <Field label="No reembolsable USD"><input inputMode="decimal" value={values.nonRefundableUsd} onChange={(e) => set("nonRefundableUsd", e.target.value)} placeholder="Sin cargar" /></Field>
        </div>
        <datalist id="trip-payment-methods">{METHODS.map((method) => <option key={method} value={method} />)}</datalist>
      </section>
    </PanelEditor>
  );
}
