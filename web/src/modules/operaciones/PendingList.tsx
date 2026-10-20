"use client";

import { MessageCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { departs, today as currentDay, whenDue } from "./format";
import type { PendingGroup, PendingRow, TripRow } from "./types";

const GROUPS: { group: PendingGroup; title: string }[] = [
  { group: "atrasado", title: "Atrasado" },
  { group: "semana", title: "Esta semana" },
  { group: "salen", title: "Salen en los próximos 14 días" },
];

/**
 * La lista de trabajo de la agencia. Un renglón por viaje: arriba, en grande,
 * qué hay que hacer; abajo, en gris, de quién es. El color solo acompaña a una
 * frase que ya dice lo mismo.
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
        return (
          <section key={group} className={`pending-group is-${group}`} aria-labelledby={id}>
            <h2 id={id}>{title}</h2>
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
      <span className="pending-text">
        {row.task ? (
          <span className="pending-task">
            <strong>{row.task.task}</strong>
            <span className="pending-when">{whenDue(row.task.dueAt, today)}</span>
          </span>
        ) : (
          <span className="pending-task"><strong>Todo listo</strong></span>
        )}
        <small>{tripLine(row, today)}</small>
        {row.more > 0 && <small className="pending-more">y {row.more} {row.more === 1 ? "cosa más" : "cosas más"}</small>}
      </span>
      <OpenChatButton row={row} onOpen={onOpen} primary={row.group !== "salen"} />
    </li>
  );
}

/** «#10482 · Agus Alonso · Cancún · sale el 10 dic», sin los datos que faltan. */
export function tripLine(row: TripRow, today: string) {
  return [`#${row.number}`, row.customerName, row.destination || undefined, row.departAt ? departs(row.departAt, today) : undefined]
    .filter(Boolean)
    .join(" · ");
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
