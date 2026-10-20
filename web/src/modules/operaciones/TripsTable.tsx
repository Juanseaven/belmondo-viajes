"use client";

import { useEffect, useRef } from "react";
import { ArrowDown, ArrowUp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { dayOf, money, shortDate, today as currentDay, whenDue } from "./format";
import type { TripRow, TripSort, TripStatus } from "./types";

export const STATUS_LABEL: Record<TripStatus, string> = {
  cotizado: "Cotizado", confirmado: "Confirmado", "en viaje": "En viaje", cerrado: "Cerrado",
};

const MISSING = "—";

/** Tono de un plazo: rojo si pasó, naranja si es esta semana. */
function tone(dueAt: string, today: string) {
  const due = dayOf(dueAt);
  if (due < today) return "is-late";
  const weekEnd = new Date(`${today}T12:00:00Z`);
  weekEnd.setUTCDate(weekEnd.getUTCDate() + 7);
  return due <= weekEnd.toISOString().slice(0, 10) ? "is-soon" : "";
}

function depositNote(row: TripRow, today: string) {
  if (row.depositPaidAt) return { text: "seña cobrada", className: "" };
  if (!row.depositDueAt) return undefined;
  const className = tone(row.depositDueAt, today);
  return { text: className === "is-late" ? "seña vencida" : `seña ${whenDue(row.depositDueAt, today)}`, className };
}

interface TripsTableProps {
  rows: TripRow[];
  total: number;
  sort: TripSort;
  onSort: (sort: TripSort) => void;
  onOpen: (row: TripRow) => void;
  onClear: () => void;
  hasMore?: boolean;
  onMore?: () => void;
  today?: string;
}

/**
 * La planilla de viajes. Pocas columnas y en dos líneas: el dato arriba y el
 * detalle en gris abajo, para que entre sin scroll horizontal. Lo que falta es
 * una raya: en cuarenta filas, «Sin completar» repetido se vuelve ruido.
 */
export function TripsTable({ rows, total, sort, onSort, onOpen, onClear, hasMore, onMore, today = currentDay() }: TripsTableProps) {
  const sentinel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const node = sentinel.current;
    if (!node || !hasMore || !onMore || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(([entry]) => entry.isIntersecting && onMore(), { rootMargin: "200px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasMore, onMore]);

  if (rows.length === 0) {
    return (
      <div className="viajes-empty">
        <strong>Ningún viaje coincide</strong>
        <p>Probá con otra búsqueda o quitá algún filtro.</p>
        <Button size="sm" variant="outline" onClick={onClear}>Limpiar filtros</Button>
      </div>
    );
  }

  const header = (label: string, key?: TripSort) => {
    if (!key) return <th scope="col">{label}</th>;
    const active = sort === key;
    const ascending = key === "salida";
    return (
      <th scope="col" aria-sort={active ? (ascending ? "ascending" : "descending") : undefined}>
        <button type="button" className={active ? "is-active" : undefined} onClick={() => onSort(key)}>
          {label}{active && (ascending ? <ArrowUp aria-hidden /> : <ArrowDown aria-hidden />)}
        </button>
      </th>
    );
  };

  return (
    <div className="trips-table-wrap">
      <p className="trips-count">{total} {total === 1 ? "viaje" : "viajes"}</p>
      <table className="trips-table">
        <thead>
          <tr>
            {header("#", "numero")}
            {header("Cliente")}
            {header("Destino")}
            {header("Salida", "salida")}
            {header("Total", "total")}
            {header("Lo que sigue")}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const deposit = depositNote(row, today);
            const people = [row.passengerCount > 0 ? `${row.passengerCount} pax` : undefined, row.sellerName].filter(Boolean).join(" · ");
            const open = row.conversationId ? () => onOpen(row) : undefined;
            return (
              <tr
                key={row.id}
                className={open ? "is-clickable" : undefined}
                onClick={open}
                onKeyDown={open ? (event) => { if (event.key === "Enter") open(); } : undefined}
                tabIndex={open ? 0 : undefined}
                title={open ? undefined : "El cliente no tiene ninguna conversación con las líneas de la agencia"}
              >
                <td className="num" data-label="#">{row.number}</td>
                <td data-label="Cliente"><strong>{row.customerName}</strong>{people && <small>{people}</small>}</td>
                <td data-label="Destino"><span>{row.destination || MISSING}</span><small>{STATUS_LABEL[row.status]}</small></td>
                <td className="num" data-label="Salida">
                  <span>{row.departAt ? shortDate(row.departAt) : MISSING}</span>
                  {row.returnAt && <small>al {shortDate(row.returnAt)}</small>}
                </td>
                <td className="num" data-label="Total">
                  <span>{row.totalUsd !== undefined ? `USD ${money(row.totalUsd)}` : MISSING}</span>
                  {deposit && <small className={deposit.className || undefined}>{deposit.text}</small>}
                </td>
                <td data-label="Lo que sigue">
                  {row.next ? <>
                    <span className={tone(row.next.dueAt, today) || undefined}>{row.next.task}</span>
                    <small>{whenDue(row.next.dueAt, today)}</small>
                  </> : <span className="is-ok">Al día</span>}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {hasMore && onMore && (
        <div ref={sentinel} className="trips-more">
          <Button size="sm" variant="ghost" onClick={onMore}>Ver más viajes</Button>
        </div>
      )}
    </div>
  );
}
