"use client";

import { useState } from "react";
import Link from "next/link";
import { ChevronDown, Info } from "lucide-react";
import { formatTypedPhone } from "@/lib/phone";
import { ControlRow, type ControlActions } from "./ControlsTab";
import { dayOf, fullDate, money, shortDay, today } from "./format";
import { requirementCounts } from "./RequirementsTab";
import type { TripDetail } from "./types";
import { nextControl } from "./types";

type Tab = "requisitos" | "pagos";

/** El Resumen: tarjetas cortas que se leen, no un formulario. */
export function SummaryTab({ trip, onTab, ...actions }: ControlActions & { trip: TripDetail; onTab: (tab: Tab) => void }) {
  const [showRest, setShowRest] = useState(false);
  const open = trip.controls.filter((control) => control.status === "abierto");
  const next = nextControl(open);
  const rest = open.filter((control) => control.id !== next?.id);
  const counts = requirementCounts(trip.requirements ?? []);
  const total = counts.cumple + counts.falta + counts.verificar;
  const payment = trip.payment;
  const customer = trip.customer;

  return (
    <>
      <section className="panel-card">
        <h3>Lo que sigue</h3>
        {next ? (
          <ul className="control-list">
            <ControlRow control={next} {...actions} />
            {showRest && rest.map((control) => <ControlRow key={control.id} control={control} {...actions} />)}
          </ul>
        ) : <p className="panel-card-text">No hay nada pendiente en este viaje.</p>}
        {next && actions.markByHand && next.closing === "accion" && (
          <p className="panel-hint"><Info aria-hidden />
            <span>{customer.name} no tiene WhatsApp: lo que en otros viajes se cierra al mandar el mensaje, acá se marca a mano y queda quién y cuándo.</span>
          </p>
        )}
        {rest.length > 0 && (
          <button type="button" className="panel-more" aria-expanded={showRest} onClick={() => setShowRest((value) => !value)}>
            <ChevronDown aria-hidden />{showRest ? "Ocultar" : `${rest.length} ${rest.length === 1 ? "control más" : "controles más"}`}
          </button>
        )}
      </section>

      <section className="panel-card">
        <h3>Requisitos{total > 0 && <button type="button" className="link-button" onClick={() => onTab("requisitos")}>Ver detalle</button>}</h3>
        {total === 0 ? (
          <p className="panel-card-text">Con los destinos y los pasajeros cargados, el sistema busca qué pide cada país.</p>
        ) : (
          <div className="req-progress">
            <div className="req-bar" aria-hidden>
              {counts.cumple > 0 && <i className="is-ok" style={{ width: `${(counts.cumple / total) * 100}%` }} />}
              {counts.falta > 0 && <i className="is-late" style={{ width: `${(counts.falta / total) * 100}%` }} />}
              {counts.verificar > 0 && <i className="is-warn" style={{ width: `${(counts.verificar / total) * 100}%` }} />}
            </div>
            <div className="req-legend">
              <span><i className="is-ok" />{counts.cumple} cumple</span>
              <span><i className="is-late" />{counts.falta} falta</span>
              <span><i className="is-warn" />{counts.verificar} a verificar</span>
            </div>
          </div>
        )}
      </section>

      <section className="panel-card">
        <h3>Pago<button type="button" className="link-button" onClick={() => onTab("pagos")}>{payment?.totalUsd !== undefined ? "Editar" : "Cargar"}</button></h3>
        <dl className="panel-kv">
          <dt>Total</dt>
          <dd>{payment?.totalUsd !== undefined ? `USD ${money(payment.totalUsd)}` : <span className="muted">Sin cotizar</span>}</dd>
          <dt>Seña</dt>
          <dd>{depositText(payment)}</dd>
        </dl>
      </section>

      <section className="panel-card">
        <h3>Cliente<Link className="link-button" href={`/clientes/${customer.id}`}>Ver ficha</Link></h3>
        <dl className="panel-kv">
          <dt>Teléfono</dt>
          <dd>{customer.phoneNumber ? formatTypedPhone(customer.phoneNumber) : <span className="muted">Sin cargar</span>}</dd>
          <dt>Mail</dt>
          <dd>{customer.email || <span className="muted">Sin cargar</span>}</dd>
          <dt>Documento</dt>
          <dd>{customer.documentNumber ? `${customer.documentType || "Documento"} ${customer.documentNumber}` : <span className="muted">Sin cargar</span>}</dd>
          <dt>Desde</dt>
          <dd>{fullDate(customer.customerSince)}</dd>
        </dl>
      </section>
    </>
  );
}

function depositText(payment: TripDetail["payment"]) {
  if (!payment?.depositUsd) return <span className="muted">Sin seña</span>;
  const amount = `USD ${money(payment.depositUsd)}`;
  if (payment.depositPaidAt) return <>{amount} · <span className="is-ok">cobrada</span></>;
  if (!payment.depositDueAt) return amount;
  const late = dayOf(payment.depositDueAt) < today();
  return <>{amount} · <span className={late ? "is-late" : "is-warn"}>{late ? "venció" : "vence"} el {shortDay(payment.depositDueAt)}</span></>;
}
