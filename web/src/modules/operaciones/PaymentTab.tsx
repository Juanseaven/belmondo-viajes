"use client";

import { useState } from "react";
import { Check, Pencil, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { NewPaymentRecord, PaymentInput } from "./api";
import { dayMonth, dayOf, money, parseAmount, shortDay, today } from "./format";
import { Field, PanelEditor, describeChanges } from "./PanelEditor";
import type { Payment, PaymentRecord } from "./types";

const METHODS = ["Transferencia", "Efectivo", "Tarjeta de crédito", "Tarjeta de débito", "Mercado Pago"];

/**
 * Lo que puede faltar para dar el total o la seña por alcanzados: un pago en
 * pesos pasado a dólares con un cambio con decimales queda unos centavos corto.
 * Es la misma tolerancia que usa el servidor para la seña.
 */
export const PAID_TOLERANCE = 1;

const AMOUNTS = [
  ["totalUsd", "Total USD"],
  ["rate", "Cambio del día"],
  ["depositUsd", "Seña USD"],
  ["nonRefundableUsd", "No reembolsable USD"],
] as const;

type AmountKey = (typeof AMOUNTS)[number][0];

const decimal = (value?: number) => value?.toString().replace(".", ",") ?? "";

const fromPayment = (payment: Payment | null) => ({
  totalUsd: decimal(payment?.totalUsd),
  rate: decimal(payment?.rate),
  depositUsd: decimal(payment?.depositUsd),
  nonRefundableUsd: decimal(payment?.nonRefundableUsd),
  method: payment?.method ?? "",
  depositDueAt: dayOf(payment?.depositDueAt),
});

/** «USD 1.234,56»: los centavos solo si los hay. */
const usd = (value: number) => `USD ${money(Math.round(value * 100) / 100)}`;

/**
 * Pagos se lee como plata: el total en grande, los pesos debajo y una barra de
 * pagado contra total que sube con cada pago registrado. Debajo, los pagos, la
 * seña y las condiciones.
 */
export function PaymentTab({ payment, records = [], paidUsd = 0, onEdit, onRegister, onDeleteRecord, onAddRecord }: {
  payment: Payment | null;
  records?: PaymentRecord[];
  paidUsd?: number;
  onEdit: () => void;
  onRegister?: () => void;
  onDeleteRecord?: (record: PaymentRecord) => Promise<void>;
  /** Deshacer un borrado lo vuelve a registrar con los mismos datos. */
  onAddRecord?: (input: NewPaymentRecord) => Promise<void>;
}) {
  const [removed, setRemoved] = useState<PaymentRecord>();
  const [error, setError] = useState<string>();

  const run = async (action: () => Promise<void>) => {
    setError(undefined);
    try {
      await action();
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar el pago");
      return false;
    }
  };

  const remove = async (record: PaymentRecord) => {
    if (onDeleteRecord && await run(() => onDeleteRecord(record))) setRemoved(record);
  };
  const undo = async (record: PaymentRecord) => {
    if (!onAddRecord) return;
    if (await run(() => onAddRecord({
      amount: record.amount, currency: record.currency, rate: record.rate, paidAt: dayOf(record.paidAt),
      method: record.method, note: record.note,
    }))) setRemoved(undefined);
  };

  const hasQuote = payment?.totalUsd !== undefined;

  return (
    <>
      {error && <p className="panel-error" role="alert">{error}</p>}
      {removed && (
        <p className="panel-undo" role="status">
          Borraste el pago de {usd(removed.amountUsd)}.
          {onAddRecord && <button type="button" onClick={() => void undo(removed)}>Deshacer</button>}
        </p>
      )}

      {hasQuote ? <TotalCard payment={payment!} paidUsd={paidUsd} /> : (
        <div className="panel-empty">
          <strong>Todavía no hay cotización</strong>
          <p>Cargá el total y la seña: con eso el sistema avisa cuándo cobrarla.</p>
          <Button size="sm" onClick={onEdit}>Cargar cotización</Button>
        </div>
      )}

      <PaymentRecords records={records} deposit={payment ?? undefined} onRegister={onRegister}
        onDelete={onDeleteRecord ? (record) => void remove(record) : undefined} />

      {payment && <Deposit payment={payment} />}

      {payment && (payment.method || payment.nonRefundableUsd !== undefined) && (
        <section className="panel-card">
          <dl className="panel-kv is-split">
            <dt>Forma de pago</dt>
            <dd>{payment.method || <span className="muted">Sin cargar</span>}</dd>
            <dt>No reembolsable</dt>
            <dd>{payment.nonRefundableUsd !== undefined ? `USD ${money(payment.nonRefundableUsd)}` : <span className="muted">Sin cargar</span>}</dd>
          </dl>
        </section>
      )}

      {hasQuote && <div><Button size="sm" variant="outline" onClick={onEdit}><Pencil aria-hidden />Editar cotización</Button></div>}
    </>
  );
}

/** El total, los pesos y la barra de pagado contra total. */
function TotalCard({ payment, paidUsd }: { payment: Payment; paidUsd: number }) {
  const total = payment.totalUsd ?? 0;
  const complete = total > 0 && paidUsd > total - PAID_TOLERANCE;
  const over = paidUsd - total >= PAID_TOLERANCE;
  const share = total > 0 ? Math.min(100, (paidUsd / total) * 100) : 0;
  return (
    <section className="panel-card money">
      <small>Total del viaje</small>
      <strong className="money-total">USD {money(total)}</strong>
      {payment.totalArs !== undefined && (
        <span className="money-ars">$ {money(payment.totalArs)}{payment.rate !== undefined && ` al cambio de $ ${money(payment.rate)}`}</span>
      )}
      <div className={complete ? "money-track is-complete" : "money-track"} aria-hidden><i style={{ width: `${complete ? 100 : share}%` }} /></div>
      <div className="money-split">
        <span>Pagado <b>{usd(paidUsd)}</b></span>
        {over ? <span className="is-soon">Pagado de más <b>{usd(paidUsd - total)}</b></span>
          : complete ? <span className="is-ok"><b>Pago completo</b></span>
            : <span>Falta <b>{usd(total - paidUsd)}</b></span>}
      </div>
    </section>
  );
}

/**
 * La lista de pagos, del más nuevo al más viejo. El pago con el que se
 * completó la seña lo dice.
 */
function PaymentRecords({ records, deposit, onRegister, onDelete }: {
  records: PaymentRecord[];
  deposit?: Payment;
  onRegister?: () => void;
  onDelete?: (record: PaymentRecord) => void;
}) {
  const depositRecord = depositReachedBy(records, deposit);
  return (
    <section className="panel-card pay-records">
      <h3>Pagos{onRegister && <button type="button" className="link-button" onClick={onRegister}><Plus aria-hidden />Registrar pago</button>}</h3>
      {records.length === 0 ? (
        <p className="panel-card-text">Todavía no hay pagos. Cada pago que se registra suma a la barra hasta completar el total.</p>
      ) : (
        <ul className="pay-list">
          {records.map((record) => (
            <li key={record.id} className="pay-row">
              <span className="pay-mark" aria-hidden><Check /></span>
              <span className="pay-copy">
                <strong>{record.method || "Pago"}{record.id === depositRecord && " · seña"}</strong>
                <small>{shortDay(record.paidAt)}{record.createdByName && ` · cargó ${record.createdByName}`}</small>
                {record.note && <small>{record.note}</small>}
              </span>
              <span className="pay-amount">
                {usd(record.amountUsd)}
                {record.currency === "ARS" && <small>$ {money(record.amount)}</small>}
              </span>
              {onDelete && (
                <button type="button" className="pay-delete" aria-label={`Borrar el pago de ${usd(record.amountUsd)}`} title="Borrar el pago"
                  onClick={() => onDelete(record)}><Trash2 aria-hidden /></button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** El pago que alcanzó la seña, sumando en orden de fecha como el servidor. */
function depositReachedBy(records: PaymentRecord[], deposit?: Payment) {
  if (!deposit?.depositUsd || !deposit.depositPaidAt || deposit.depositPaidBefore) return undefined;
  let acc = 0;
  const ordered = [...records].sort((a, b) => dayOf(a.paidAt).localeCompare(dayOf(b.paidAt)) || a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id));
  for (const record of ordered) {
    acc += record.amountUsd;
    if (acc > deposit.depositUsd - PAID_TOLERANCE) return record.id;
  }
  return undefined;
}

/** La seña: con tilde verde si se cobró; ocre con el vencimiento si no; roja si venció. */
function Deposit({ payment }: { payment: Payment }) {
  if (!payment.depositUsd && !payment.depositPaidBefore) return null;
  const amount = payment.depositUsd ? `USD ${money(payment.depositUsd)}` : "La seña";
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
 * La cotización, con el esquema de Editar. Se guarda entera (PUT): siempre
 * manda todos los campos, así que un campo vacío borra el dato. El total en
 * pesos no se escribe: es total por cambio. La seña cobrada tampoco: la dan los
 * pagos registrados.
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
    values.totalUsd !== initial.totalUsd || values.rate !== initial.rate ? "el total" : "",
    values.depositUsd !== initial.depositUsd || values.depositDueAt !== initial.depositDueAt ? "la seña" : "",
    values.method !== initial.method || values.nonRefundableUsd !== initial.nonRefundableUsd ? "las condiciones" : "",
  ].filter(Boolean));

  const total = parseAmount(values.totalUsd);
  const rate = parseAmount(values.rate);
  const pesos = total !== undefined && rate !== undefined && !Number.isNaN(total) && !Number.isNaN(rate) ? Math.round(total * rate) : undefined;

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
    // Una seña sin monto no se puede dar por cobrada sumando pagos. La vieja,
    // cobrada antes de los pagos registrados, no lo necesita.
    if (values.depositDueAt && amounts.depositUsd === undefined && !payment?.depositPaidBefore) {
      setError("Cargá el monto de la seña: sin él no se sabe cuándo está cobrada.");
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      await onSave({
        totalUsd: amounts.totalUsd, rate: amounts.rate,
        method: values.method.trim() || undefined,
        depositUsd: amounts.depositUsd, depositDueAt: values.depositDueAt || undefined,
        nonRefundableUsd: amounts.nonRefundableUsd,
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar la cotización");
    } finally {
      setSaving(false);
    }
  };

  return (
    <PanelEditor title={`Cotización del viaje #${tripNumber}`} saveLabel="Guardar cambios" changes={changed} saving={saving} error={error}
      canSave={Boolean(changed)} onSave={() => void save()} onBack={onBack}>
      <section className="editor-section">
        <h4>Total</h4>
        <div className="field-pair">
          <Field label="Total USD"><input inputMode="decimal" value={values.totalUsd} onChange={(e) => set("totalUsd", e.target.value)} placeholder="Sin cargar" /></Field>
          <Field label="Cambio del día"><input inputMode="decimal" value={values.rate} onChange={(e) => set("rate", e.target.value)} placeholder="Sin cargar" /></Field>
        </div>
        <p className="editor-hint">
          {pesos !== undefined ? <>Total en pesos: <b>$ {money(pesos)}</b> (se calcula solo)</> : "Con el total y el cambio, el total en pesos se calcula solo."}
        </p>
      </section>
      <section className="editor-section">
        <h4>Seña</h4>
        <div className="field-pair">
          <Field label="Seña USD"><input inputMode="decimal" value={values.depositUsd} onChange={(e) => set("depositUsd", e.target.value)} placeholder="Sin cargar" /></Field>
          <Field label="Cobrarla antes del"><input type="date" value={values.depositDueAt} onChange={(e) => set("depositDueAt", e.target.value)} /></Field>
        </div>
        <p className="editor-hint">Se da por cobrada sola cuando los pagos registrados llegan a su monto.</p>
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

/**
 * Registrar un pago. En pesos pide el cambio de ese día (viene puesto el de la
 * cotización) y dice a cuántos dólares equivale y cuánto queda por pagar.
 */
export function PaymentRecordEditor({ payment, paidUsd = 0, onSave, onBack }: {
  payment: Payment | null;
  paidUsd?: number;
  onSave: (input: NewPaymentRecord) => Promise<void>;
  onBack: () => void;
}) {
  const [currency, setCurrency] = useState<"USD" | "ARS">("USD");
  const [amount, setAmount] = useState("");
  const [rate, setRate] = useState(decimal(payment?.rate));
  const [paidAt, setPaidAt] = useState(today());
  const [method, setMethod] = useState(payment?.method ?? "");
  const [note, setNote] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  const value = parseAmount(amount);
  const rateValue = parseAmount(rate);
  const valid = value !== undefined && !Number.isNaN(value) && value > 0;
  const rateValid = rateValue !== undefined && !Number.isNaN(rateValue) && rateValue > 0;
  const equivalent = valid ? (currency === "USD" ? value : rateValid ? Math.round((value / rateValue) * 100) / 100 : undefined) : undefined;
  const total = payment?.totalUsd;

  const hint = () => {
    if (equivalent === undefined) return currency === "ARS" ? "Con el monto y el cambio, se calcula a cuántos dólares equivale." : undefined;
    const prefix = currency === "ARS" ? <>Equivale a <b>{usd(equivalent)}</b>. </> : null;
    if (total === undefined) return prefix;
    const left = total - paidUsd - equivalent;
    return <>{prefix}{left < PAID_TOLERANCE ? "Con este pago se completa el total." : `Con este pago faltan ${usd(left)}.`}</>;
  };

  const save = async () => {
    if (!valid) {
      setError("Escribí el monto con números, por ejemplo 1450,50.");
      return;
    }
    if (currency === "ARS" && !rateValid) {
      setError("Un pago en pesos necesita el cambio del día.");
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      await onSave({
        amount: value, currency, rate: currency === "ARS" ? rateValue : undefined, paidAt,
        method: method.trim() || undefined, note: note.trim() || undefined,
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo registrar el pago");
      setSaving(false);
    }
  };

  return (
    <PanelEditor title="Registrar pago" saveLabel="Registrar pago" changes={equivalent !== undefined ? usd(equivalent) : ""} saving={saving}
      error={error} canSave={valid} onSave={() => void save()} onBack={onBack}>
      <section className="editor-section">
        <div className="field">
          <span>Moneda</span>
          <div className="segmented" role="radiogroup" aria-label="Moneda">
            <button type="button" role="radio" aria-checked={currency === "USD"} onClick={() => setCurrency("USD")}>USD</button>
            <button type="button" role="radio" aria-checked={currency === "ARS"} onClick={() => setCurrency("ARS")}>Pesos</button>
          </div>
        </div>
        <div className="field-pair">
          <Field label={currency === "USD" ? "Monto en dólares" : "Monto en pesos"}>
            <input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} autoFocus />
          </Field>
          {currency === "ARS" && <Field label="Cambio"><input inputMode="decimal" value={rate} onChange={(e) => setRate(e.target.value)} /></Field>}
        </div>
        {hint() && <p className="editor-hint">{hint()}</p>}
        <div className="field-pair">
          <Field label="Fecha"><input type="date" value={paidAt} onChange={(e) => setPaidAt(e.target.value)} /></Field>
          <Field label="Forma de pago"><input list="record-payment-methods" value={method} onChange={(e) => setMethod(e.target.value)} placeholder="Elegir" /></Field>
        </div>
        <datalist id="record-payment-methods">{METHODS.map((option) => <option key={option} value={option} />)}</datalist>
        <Field label="Nota (opcional)"><input value={note} onChange={(e) => setNote(e.target.value)} placeholder="Ej.: comprobante 4421" /></Field>
      </section>
    </PanelEditor>
  );
}
