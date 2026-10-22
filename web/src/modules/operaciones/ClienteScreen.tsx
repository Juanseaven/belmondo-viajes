"use client";

/**
 * La página de un cliente: sus datos, sus viajes, sus chats y con quién viaja.
 *
 * Un viaje de un cliente con WhatsApp se abre en el chat, como desde Viajes:
 * ahí está el panel y ahí se mandan los mensajes que cierran los controles. Uno
 * sin WhatsApp se abre en el panel que entra desde la derecha, y `?viaje=` lo
 * deja en la URL. `?nuevo-viaje=1` abre el formulario de viaje nuevo.
 */

import { useCallback, useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { CalendarDays, ChevronLeft, ChevronRight, IdCard, Mail, MessageCircle, Pencil, Phone, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useSocket } from "@/kernel/api/useSocket";
import { displayName, formatPhone, formatTypedPhone } from "@/lib/phone";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import { conversationAvatarUrl } from "@/modules/mirror/api";
import { operacionesApi } from "./api";
import { companionLine } from "./BringPassengersDialog";
import { clientesListHref } from "./ClientesScreen";
import { CustomerDialog } from "./CustomerDialog";
import { CustomerFiles } from "./CustomerFiles";
import { dayMonth, dayOf, inWords, money, monthName, shortDay, today as currentDay, urgency } from "./format";
import { LinkChatDialog } from "./LinkDialogs";
import { StatusPill } from "./status";
import { TripDrawer } from "./TripDrawer";
import { depositNote } from "./TripsTable";
import type { CustomerDetail, TripRow } from "./types";

export function ClienteScreen() {
  const { id } = useParams<{ id: string }>();
  const params = useSearchParams();
  const router = useRouter();
  const { customer, error, reload } = useCustomer(id);
  const [editing, setEditing] = useState(false);
  const [linking, setLinking] = useState(false);
  const tripId = params.get("viaje") ?? undefined;
  const creating = params.get("nuevo-viaje") === "1";
  const base = `/clientes/${encodeURIComponent(id)}`;

  const chatOf = (detail: CustomerDetail) => detail.contacts.find((contact) => contact.conversationId)?.conversationId;
  const toChat = (conversationId: string, trip: string) =>
    router.push(`/mirror?c=${encodeURIComponent(conversationId)}&viaje=${encodeURIComponent(trip)}`);

  const openTrip = (row: TripRow) => {
    if (row.conversationId) toChat(row.conversationId, row.id);
    else router.replace(`${base}?viaje=${encodeURIComponent(row.id)}`, { scroll: false });
  };

  if (error) {
    return (
      <section className="viajes-screen clientes-screen">
        <div className="viajes-body">
          <Link className="cliente-back" href={clientesListHref()}><ChevronLeft aria-hidden />Clientes</Link>
          <p className="viajes-error" role="alert">{error}</p>
        </div>
      </section>
    );
  }
  if (!customer) return <section className="viajes-screen clientes-screen"><p className="viajes-loading viajes-body">Cargando el cliente…</p></section>;

  return (
    <section className="viajes-screen clientes-screen">
      <header className="cliente-head">
        <ContactAvatar name={customer.name} src={chatOf(customer) ? conversationAvatarUrl(chatOf(customer)!) : undefined} className="avatar-56" />
        <div className="cliente-title">
          <Link className="cliente-back" href={clientesListHref()}><ChevronLeft aria-hidden />Clientes</Link>
          <h1>{customer.name}</h1>
        </div>
        <div className="cliente-actions">
          <Button size="sm" variant="outline" onClick={() => setEditing(true)}><Pencil aria-hidden />Editar</Button>
          <Button size="sm" onClick={() => router.replace(`${base}?nuevo-viaje=1`, { scroll: false })}><Plus aria-hidden />Nuevo viaje</Button>
        </div>
      </header>
      <div className="viajes-body cliente-body">
        <dl className="cliente-facts">
          <Fact icon={<Phone aria-hidden />} label="Teléfono" value={customer.phoneNumber && formatTypedPhone(customer.phoneNumber)} empty="Sin teléfono" />
          <Fact icon={<Mail aria-hidden />} label="Mail" value={customer.email} empty="Sin mail" />
          <Fact icon={<IdCard aria-hidden />} label="Documento"
            value={customer.documentNumber && `${customer.documentType || "Documento"} ${customer.documentNumber}`} empty="Sin documento" />
          <Fact icon={<CalendarDays aria-hidden />} label="Cliente desde" value={monthName(customer.customerSince)} />
        </dl>

        <div className="cliente-grid">
          <div className="cliente-main">
          <section className="panel-card cliente-card">
            <h3>Viajes <small>{customer.trips.length}</small>
              <button type="button" className="link-button" onClick={() => router.replace(`${base}?nuevo-viaje=1`, { scroll: false })}><Plus aria-hidden />Nuevo viaje</button>
            </h3>
            {customer.trips.length === 0 ? (
              <div className="cliente-empty">
                <strong>Todavía no tiene viajes</strong>
                <p>Cargá el recorrido y las fechas; los pasajeros y el pago se completan después.</p>
                <Button size="sm" onClick={() => router.replace(`${base}?nuevo-viaje=1`, { scroll: false })}><Plus aria-hidden />Nuevo viaje</Button>
              </div>
            ) : <CustomerTrips rows={customer.trips} selected={tripId} onOpen={openTrip} />}
          </section>

          <section className="panel-card cliente-card">
            <h3>Archivos</h3>
            <div className="cliente-files">
              <CustomerFiles customerId={customer.id} customerName={customer.name} columns={6} preview={12} />
            </div>
          </section>
          </div>

          <div className="cliente-side">
            <section className="panel-card cliente-card">
              <h3>WhatsApp</h3>
              {customer.contacts.length === 0 ? (
                <div className="cliente-chat is-empty">
                  <MessageCircle aria-hidden />
                  <span>
                    <strong>No tiene chats</strong>
                    {customer.phoneNumber && <small>Si escribe desde {formatTypedPhone(customer.phoneNumber)}, la bandeja va a ofrecer vincularlo.</small>}
                  </span>
                </div>
              ) : (
                <ul className="cliente-list">
                  {customer.contacts.map((contact) => (
                    <li key={contact.contactId} className="cliente-chat">
                      <ContactAvatar name={contact.name} src={contact.conversationId ? conversationAvatarUrl(contact.conversationId) : undefined} className="avatar-36" />
                      <span>
                        <strong>{displayName(contact.name)}</strong>
                        <small>{[contact.phoneNumber && formatPhone(contact.phoneNumber), contact.instanceLabel && `Línea ${contact.instanceLabel}`]
                          .filter(Boolean).join(" · ") || "Sin chat"}</small>
                      </span>
                      {contact.conversationId && (
                        <Button size="sm" variant="outline" onClick={() => router.push(`/mirror?c=${encodeURIComponent(contact.conversationId!)}`)}>Abrir chat</Button>
                      )}
                    </li>
                  ))}
                </ul>
              )}
              <div className="cliente-card-foot">
                <Button size="sm" variant="outline" onClick={() => setLinking(true)}>Vincular un chat</Button>
              </div>
            </section>

            {customer.companions.length > 0 && (
              <section className="panel-card cliente-card">
                <h3>Con quién viaja</h3>
                <ul className="cliente-list">
                  {customer.companions.map((companion) => (
                    <li key={companion.passengerId} className="cliente-chat">
                      <ContactAvatar name={`${companion.firstName} ${companion.lastName}`} className="avatar-36" />
                      <span>
                        <strong>{companion.firstName} {companion.lastName}</strong>
                        <small>{companion.tripCount} {companion.tripCount === 1 ? "viaje" : "viajes"} · {companionLine(companion)}</small>
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
            )}
          </div>
        </div>
      </div>

      <TripDrawer
        customerId={customer.id}
        tripId={creating ? undefined : tripId}
        creating={creating}
        onClose={() => router.replace(base, { scroll: false })}
        onSelectTrip={(next) => router.replace(`${base}?viaje=${encodeURIComponent(next)}`, { scroll: false })}
        onCreated={(trip, detail) => {
          const chat = chatOf(detail);
          if (chat) toChat(chat, trip.id);
          else router.replace(`${base}?viaje=${encodeURIComponent(trip.id)}`, { scroll: false });
        }}
      />
      {editing && (
        <CustomerDialog customer={customer} onClose={() => setEditing(false)}
          onSaved={() => { setEditing(false); void reload(); }} />
      )}
      {linking && (
        <LinkChatDialog customerId={customer.id} customerName={customer.name} onClose={() => setLinking(false)}
          onLinked={() => { setLinking(false); void reload(); }} />
      )}
    </section>
  );
}

function useCustomer(id: string) {
  const [customer, setCustomer] = useState<CustomerDetail>();
  const [error, setError] = useState<string>();

  const reload = useCallback(async () => {
    try {
      setCustomer(await operacionesApi.customer(id));
      setError(undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo cargar el cliente");
    }
  }, [id]);

  useEffect(() => { void reload(); }, [reload]);
  useSocket("operaciones", () => { void reload(); });
  return { customer, error, reload };
}

function Fact({ icon, label, value, empty }: { icon: ReactNode; label: string; value?: string; empty?: string }) {
  return (
    <div className="cliente-fact">
      {icon}
      <dt>{label}</dt>
      <dd>{value || <span className="muted">{empty}</span>}</dd>
    </div>
  );
}

/** Los viajes del cliente, con las columnas de Viajes: abiertos primero. */
function CustomerTrips({ rows, selected, onOpen, today = currentDay() }: {
  rows: TripRow[];
  selected?: string;
  onOpen: (row: TripRow) => void;
  today?: string;
}) {
  return (
    <table className="trips-table cliente-trips">
      <thead>
        <tr>
          <th scope="col" className="col-num">#</th>
          <th scope="col">Destino</th>
          <th scope="col">Salida</th>
          <th scope="col" className="is-right">Total</th>
          <th scope="col">Lo que sigue</th>
          <th scope="col" className="col-chevron"><span className="sr-only">Abrir</span></th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => {
          const deposit = depositNote(row, today);
          const tone = row.next ? urgency(row.next.dueAt, today) : undefined;
          const closed = row.status === "cerrado";
          return (
            <tr key={row.id} className={`is-clickable${row.id === selected ? " is-selected" : ""}`} tabIndex={0}
              onClick={() => onOpen(row)} onKeyDown={(event) => { if (event.key === "Enter") onOpen(row); }}>
              <td className="num col-num" data-label="#">{row.number}</td>
              <td data-label="Destino">
                <span className="trips-dest">{row.destination || <span className="muted">Sin destino</span>}<StatusPill status={row.status} /></span>
              </td>
              <td className="num" data-label="Salida">
                {row.departAt ? <>
                  <span>{shortDay(row.departAt)}</span>
                  {row.returnAt && <small>vuelve el {dayMonth(row.returnAt)}</small>}
                </> : <span className="muted">Sin fecha</span>}
              </td>
              <td className="num is-right" data-label="Total">
                {row.totalUsd !== undefined ? <span>USD {money(row.totalUsd)}</span> : <span className="muted">Sin cotizar</span>}
                {deposit && <small className={deposit.className || undefined}>{deposit.text}</small>}
              </td>
              <td data-label="Lo que sigue">
                {closed ? <span className="muted">—</span> : row.next ? (
                  <span className="trips-next">
                    <i className={`dot is-${tone}`} aria-hidden />
                    <span>
                      <span className={tone === "later" ? undefined : `is-${tone}`}>{row.next.task}</span>
                      <small>{inWords(dayOf(row.next.dueAt), today)}</small>
                    </span>
                  </span>
                ) : <span className="trips-next"><i className="dot is-ok" aria-hidden /><span className="is-ok">Al día</span></span>}
              </td>
              <td className="col-chevron" aria-hidden><ChevronRight /></td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
