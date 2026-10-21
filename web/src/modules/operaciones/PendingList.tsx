"use client";

import { MessageCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { displayName } from "@/lib/phone";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import { conversationAvatarUrl } from "@/modules/mirror/api";
import { dayMonth, inWords, shortDay, today as currentDay, weekdayDate } from "./format";
import type { PendingGroup, PendingRow, TripRow } from "./types";

const GROUPS: { group: PendingGroup; title: string }[] = [
  { group: "atrasado", title: "Atrasado" },
  { group: "semana", title: "Esta semana" },
  { group: "salen", title: "Salen en los próximos 14 días" },
];

/**
 * La lista de trabajo de la agencia. Un renglón por viaje: la cara del cliente,
 * qué hay que hacer en negrita, de quién es debajo, y a la derecha el plazo en
 * palabras. El color de cada grupo acompaña a un título que dice lo mismo.
 */
export function PendingList({ rows, today = currentDay(), onOpen }: {
  rows: PendingRow[];
  today?: string;
  onOpen: (row: TripRow) => void;
}) {
  if (rows.length === 0) {
    return (
      <div className="viajes-empty">
        <strong>No hay nada pendiente.</strong>
        <p>Los viajes que salen en los próximos 14 días van a aparecer acá.</p>
      </div>
    );
  }

  return (
    <div className="pending-list">
      {GROUPS.map(({ group, title }) => {
        const items = rows.filter((row) => row.group === group);
        if (items.length === 0) return null;
        const id = `pending-${group}`;
        const allReady = items.every((row) => !row.task);
        return (
          <section key={group} className={`pending-group is-${group}`} aria-labelledby={id}>
            <h2 id={id}>
              <span className="pending-swatch" aria-hidden />{title}
              <small>{items.length} {items.length === 1 ? "viaje" : "viajes"}{group === "salen" && allReady ? ", todo listo" : ""}</small>
            </h2>
            <ul>
              {items.map((row) => <PendingItemRow key={row.id} row={row} today={today} onOpen={onOpen} />)}
            </ul>
          </section>
        );
      })}
    </div>
  );
}

function PendingItemRow({ row, today, onOpen }: { row: PendingRow; today: string; onOpen: (row: TripRow) => void }) {
  return (
    <li className={`pending-row is-${row.group}`}>
      <ContactAvatar name={row.customerName} src={row.conversationId ? conversationAvatarUrl(row.conversationId) : undefined} className="avatar-36" />
      <span className="pending-text">
        <strong className={row.task ? undefined : "is-ok"}>{row.task ? row.task.task : "Todo listo"}</strong>
        <span className="pending-sub">
          <b>{displayName(row.customerName)}</b>
          <span>{tripLine(row, row.group === "salen")}</span>
          {row.more > 0 && <span className="tag">y {row.more} {row.more === 1 ? "cosa más" : "cosas más"}</span>}
        </span>
      </span>
      <When row={row} today={today} />
      <OpenChatButton row={row} onOpen={onOpen} primary={row.group !== "salen"} />
    </li>
  );
}

/** El plazo en palabras, con la fecha chica debajo. En los que salen, la salida. */
function When({ row, today }: { row: PendingRow; today: string }) {
  const tone = row.group === "atrasado" ? "is-late" : row.group === "semana" ? "is-soon" : "";
  if (row.task) {
    return (
      <span className={`pending-when ${tone}`}>
        {inWords(row.task.dueAt, today)}
        <small>{row.group === "atrasado" ? `era para el ${dayMonth(row.task.dueAt)}` : shortDay(row.task.dueAt)}</small>
      </span>
    );
  }
  if (!row.departAt) return <span className="pending-when" />;
  return <span className="pending-when">{inWords(row.departAt, today)}<small>{shortDay(row.departAt)}</small></span>;
}

/** «#10482 · Cancún · sale el jue 10 dic». Los pasajeros, solo en los que salen y están listos. */
export function tripLine(row: TripRow, withPassengers = false) {
  return [
    `#${row.number}`,
    row.destination || "sin destino",
    row.departAt ? `sale el ${weekdayDate(row.departAt)}` : "sin fecha de salida",
    withPassengers && row.passengerCount > 0 ? `${row.passengerCount} ${row.passengerCount === 1 ? "pasajero" : "pasajeros"}` : undefined,
  ].filter(Boolean).join(" · ");
}

export function OpenChatButton({ row, onOpen, primary }: { row: TripRow; onOpen: (row: TripRow) => void; primary?: boolean }) {
  if (!row.conversationId) {
    return (
      <Button size="sm" variant="outline" disabled title="El cliente no tiene ninguna conversación con las líneas de la agencia">
        Sin chat
      </Button>
    );
  }
  return (
    <Button size="sm" variant={primary ? "default" : "outline"} onClick={(event) => { event.stopPropagation(); onOpen(row); }}>
      <MessageCircle aria-hidden />Abrir chat
    </Button>
  );
}
