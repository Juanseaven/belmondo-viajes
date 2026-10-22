"use client";

import { useEffect, useRef } from "react";
import { ArrowDown, ArrowUp, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { displayName } from "@/lib/phone";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import { conversationAvatarUrl } from "@/modules/mirror/api";
import { dayMonth, dayOf, inWords, money, shortDay, today as currentDay, urgency, whenDue } from "./format";
import { StatusPill } from "./status";
import type { TripRow, TripSort } from "./types";

const SORT_LABEL: Record<TripSort, string> = { salida: "salida", numero: "número", total: "total" };

export function depositNote(row: TripRow, today: string) {
  if (row.depositPaidAt) return { text: "seña cobrada", className: "is-ok" };
  if (!row.depositDueAt) return undefined;
  const tone = urgency(row.depositDueAt, today);
  if (tone === "late") return { text: "seña vencida", className: "is-late" };
  return { text: `seña ${whenDue(row.depositDueAt, today)}`, className: tone === "soon" ? "is-soon" : "" };
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
 * detalle en gris abajo, para que entre sin scroll horizontal. Lo que falta
 * dice qué falta («Sin cotizar», «Sin fecha»): una raya no dice si es cero, si
 * no se sabe o si no aplica.
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

  const header = (label: string, key?: TripSort, className?: string) => {
    if (!key) return <th scope="col" className={className}>{label}</th>;
    const active = sort === key;
    const ascending = key === "salida";
    return (
      <th scope="col" className={className} aria-sort={active ? (ascending ? "ascending" : "descending") : undefined}>
        <button type="button" className={active ? "is-active" : undefined} onClick={() => onSort(key)}>
          {label}{active && (ascending ? <ArrowUp aria-hidden /> : <ArrowDown aria-hidden />)}
        </button>
      </th>
    );
  };

  return (
    <div className="trips-table-wrap">
      <p className="trips-count">{total} {total === 1 ? "viaje" : "viajes"} · ordenados por {SORT_LABEL[sort]}</p>
      <table className="trips-table">
        <thead>
          <tr>
            {header("#", "numero", "col-num")}
            {header("Cliente")}
            {header("Destino")}
            {header("Salida", "salida")}
            {header("Total", "total", "is-right")}
            {header("Lo que sigue")}
            <th scope="col" className="col-chevron"><span className="sr-only">Abrir</span></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const deposit = depositNote(row, today);
            const people = row.passengerCount > 0 ? `${row.passengerCount} ${row.passengerCount === 1 ? "pasajero" : "pasajeros"}` : "Sin pasajeros";
            // Sin chat, se abre el viaje en el panel; sin cliente (un viaje leído
            // de una versión vieja del servidor), no hay qué abrir.
            const open = row.conversationId || row.customerId ? () => onOpen(row) : undefined;
            const tone = row.next ? urgency(row.next.dueAt, today) : undefined;
            return (
              <tr
                key={row.id}
                className={open ? "is-clickable" : undefined}
                onClick={open}
                onKeyDown={open ? (event) => { if (event.key === "Enter") open(); } : undefined}
                tabIndex={open ? 0 : undefined}
                title={row.conversationId ? undefined : "El cliente no tiene WhatsApp: el viaje se abre acá"}
              >
                <td className="num col-num" data-label="#">{row.number}</td>
                <td data-label="Cliente">
                  <span className="trips-client">
                    <ContactAvatar name={row.customerName} src={row.conversationId ? conversationAvatarUrl(row.conversationId) : undefined} className="avatar-32" />
                    <span>
                      <strong>{displayName(row.customerName)}</strong>
                      <small>{people} · {row.sellerName || "Sin asignar"}</small>
                    </span>
                  </span>
                </td>
                <td data-label="Destino">
                  <span className="trips-dest">
                    {row.destination || <span className="muted">Sin destino</span>}
                    <StatusPill status={row.status} />
                  </span>
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
                  {row.next ? (
                    <span className="trips-next">
                      <i className={`dot is-${tone}`} aria-hidden />
                      <span>
                        <span className={tone === "later" ? undefined : `is-${tone}`}>{row.next.task}</span>
                        <small>{inWords(dayOf(row.next.dueAt), today)}</small>
                      </span>
                    </span>
                  ) : (
                    <span className="trips-next"><i className="dot is-ok" aria-hidden /><span className="is-ok">Al día</span></span>
                  )}
                </td>
                <td className="col-chevron" aria-hidden>{open && <ChevronRight />}</td>
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
