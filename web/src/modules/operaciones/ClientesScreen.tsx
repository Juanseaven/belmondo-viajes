"use client";

/**
 * La pantalla de Clientes: quien compra los viajes, con o sin WhatsApp.
 *
 * Tiene el mismo esquema que «Todos los viajes»: buscador, chips con cuántos
 * hay y la tabla. La búsqueda, el chip y el orden viven en la URL, para que una
 * recarga o un enlace pasado a otra persona muestren lo mismo.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowDown, ArrowUp, ChevronRight, Mail, MessageCircle, Phone, Plus, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useSocket } from "@/kernel/api/useSocket";
import { formatPhone, formatTypedPhone } from "@/lib/phone";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import { conversationAvatarUrl } from "@/modules/mirror/api";
import { operacionesApi } from "./api";
import { CustomerDialog } from "./CustomerDialog";
import { monthName, weekdayDate } from "./format";
import { StatusPill } from "./status";
import type { CustomerChip, CustomerPage, CustomerRow, CustomerSort } from "./types";

const PATH = "/clientes";

const CHIPS: { value: CustomerChip; label: string }[] = [
  { value: "todos", label: "Todos" },
  { value: "con-viaje", label: "Con viaje abierto" },
  { value: "sin-viajes", label: "Sin viajes" },
  { value: "sin-whatsapp", label: "Sin WhatsApp" },
];

/**
 * La búsqueda de la lista tal como quedó, para que «‹ Clientes» en la página de
 * un cliente vuelva a lo mismo.
 */
let lastListQuery = "";
export const clientesListHref = () => `${PATH}${lastListQuery}`;

interface Filters { q: string; filtro: CustomerChip; orden: CustomerSort }

function filtersFrom(params: URLSearchParams): Filters {
  const filtro = CHIPS.some((chip) => chip.value === params.get("filtro")) ? params.get("filtro") as CustomerChip : "todos";
  return { q: params.get("q") ?? "", filtro, orden: params.get("orden") === "nombre" ? "nombre" : "movimiento" };
}

function toQuery(filters: Filters) {
  const query = new URLSearchParams();
  if (filters.q) query.set("q", filters.q);
  if (filters.filtro !== "todos") query.set("filtro", filters.filtro);
  if (filters.orden !== "movimiento") query.set("orden", filters.orden);
  const text = query.toString();
  return text ? `?${text}` : "";
}

export function ClientesScreen() {
  const params = useSearchParams();
  const router = useRouter();
  const filters = useMemo(() => filtersFrom(new URLSearchParams(params.toString())), [params]);
  const [creating, setCreating] = useState(false);
  const list = useCustomers(filters);

  useEffect(() => { lastListQuery = toQuery(filters); }, [filters]);

  const go = (next: Filters) => router.replace(`${PATH}${toQuery(next)}`, { scroll: false });
  const open = (id: string, query = "") => router.push(`${PATH}/${encodeURIComponent(id)}${query}`);

  return (
    <section className="viajes-screen clientes-screen">
      <header className="clientes-head">
        <h1>Clientes</h1>
        <Button size="sm" onClick={() => setCreating(true)}><Plus aria-hidden />Nuevo cliente</Button>
      </header>
      <div className="viajes-body">
        <div className="trips-filters">
          <div className="trips-filters-row">
            <div className="trips-search">
              <Search aria-hidden />
              <Input type="search" aria-label="Buscar clientes" placeholder="Buscar por nombre, teléfono, mail, documento o pasajero"
                value={filters.q} onChange={(event) => go({ ...filters, q: event.target.value })} />
            </div>
          </div>
          <div className="trips-filters-row" role="group" aria-label="Filtro">
            {CHIPS.map((chip) => {
              const count = list.page?.byChip?.[chip.value];
              return (
                <button key={chip.value} type="button" className="trips-chip" aria-pressed={filters.filtro === chip.value}
                  onClick={() => go({ ...filters, filtro: chip.value })}>
                  {chip.label}{count !== undefined && <> <em>{count}</em></>}
                </button>
              );
            })}
          </div>
        </div>
        {list.error && <p className="viajes-error" role="alert">{list.error}</p>}
        {!list.page ? <p className="viajes-loading">Cargando clientes…</p> : (
          <CustomersTable
            rows={list.rows}
            total={list.page.total}
            sort={filters.orden}
            searching={Boolean(filters.q) || filters.filtro !== "todos"}
            onSort={(orden) => go({ ...filters, orden })}
            onOpen={(row) => open(row.id)}
            onClear={() => go({ q: "", filtro: "todos", orden: filters.orden })}
            onCreate={() => setCreating(true)}
            hasMore={Boolean(list.cursor)}
            onMore={list.loadMore}
          />
        )}
      </div>
      {creating && (
        <CustomerDialog
          onClose={() => setCreating(false)}
          onOpenCustomer={(id) => { setCreating(false); open(id); }}
          onCreated={(customer, withTrip) => { setCreating(false); open(customer.id, withTrip ? "?nuevo-viaje=1" : ""); }}
        />
      )}
    </section>
  );
}

/**
 * La lista: primera página al cambiar los filtros o ante cualquier evento del
 * módulo, y las siguientes por cursor. También se recarga al volver a la
 * pestaña del navegador.
 */
function useCustomers(filters: Filters) {
  const [page, setPage] = useState<CustomerPage>();
  const [rows, setRows] = useState<CustomerRow[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [error, setError] = useState<string>();
  const [version, setVersion] = useState(0);
  const params = useMemo(() => ({ q: filters.q.trim() || undefined, filtro: filters.filtro, orden: filters.orden }), [filters]);
  const key = JSON.stringify(params);

  useSocket("operaciones", () => setVersion((value) => value + 1));
  useEffect(() => {
    const onVisible = () => { if (document.visibilityState === "visible") setVersion((value) => value + 1); };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, []);

  useEffect(() => {
    let current = true;
    const timer = setTimeout(() => {
      operacionesApi.customers(params)
        .then((next) => {
          if (!current) return;
          setPage(next);
          setRows(next.items);
          setCursor(next.nextCursor);
          setError(undefined);
        })
        .catch((cause) => { if (current) setError(cause instanceof Error ? cause.message : "No se pudieron cargar los clientes"); });
    }, 200);
    return () => { current = false; clearTimeout(timer); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, version]);

  const loadMore = useCallback(() => {
    if (!cursor) return;
    const next = cursor;
    setCursor(undefined);
    operacionesApi.customers({ ...params, cursor: next })
      .then((more) => {
        setRows((current) => [...current, ...more.items]);
        setCursor(more.nextCursor);
      })
      .catch(() => setCursor(next));
  }, [cursor, params]);

  return { page, rows, cursor, error, loadMore };
}

const SORT_LABEL: Record<CustomerSort, string> = { movimiento: "el último movimiento", nombre: "nombre" };

/**
 * La tabla de clientes. El dato arriba y el detalle en gris abajo, como en
 * Viajes. Toda la fila abre la página del cliente.
 */
export function CustomersTable({ rows, total, sort, searching, onSort, onOpen, onClear, onCreate, hasMore, onMore }: {
  rows: CustomerRow[];
  total: number;
  sort: CustomerSort;
  searching: boolean;
  onSort: (sort: CustomerSort) => void;
  onOpen: (row: CustomerRow) => void;
  onClear: () => void;
  onCreate: () => void;
  hasMore?: boolean;
  onMore?: () => void;
}) {
  const sentinel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const node = sentinel.current;
    if (!node || !hasMore || !onMore || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(([entry]) => entry.isIntersecting && onMore(), { rootMargin: "200px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasMore, onMore]);

  if (rows.length === 0) {
    return searching ? (
      <div className="viajes-empty">
        <strong>Ningún cliente coincide</strong>
        <p>Probá con otra búsqueda o quitá el filtro.</p>
        <Button size="sm" variant="outline" onClick={onClear}>Limpiar búsqueda</Button>
      </div>
    ) : (
      <div className="viajes-empty">
        <strong>Todavía no hay clientes</strong>
        <p>Los clientes nacen al abrir un viaje desde un chat, o se cargan a mano.</p>
        <Button size="sm" onClick={onCreate}><Plus aria-hidden />Nuevo cliente</Button>
      </div>
    );
  }

  const header = (label: string, key?: CustomerSort) => {
    if (!key) return <th scope="col">{label}</th>;
    const active = sort === key;
    return (
      <th scope="col" aria-sort={active ? (key === "nombre" ? "ascending" : "descending") : undefined}>
        <button type="button" className={active ? "is-active" : undefined} onClick={() => onSort(key)}>
          {label}{active && (key === "nombre" ? <ArrowUp aria-hidden /> : <ArrowDown aria-hidden />)}
        </button>
      </th>
    );
  };

  return (
    <div className="trips-table-wrap">
      <p className="trips-count">
        {total} {total === 1 ? "cliente" : "clientes"} · ordenados por {SORT_LABEL[sort]}
        {sort === "nombre" && <> · <button type="button" className="link-button" onClick={() => onSort("movimiento")}>ordenar por movimiento</button></>}
      </p>
      <table className="trips-table customers-table">
        <thead>
          <tr>
            {header("Cliente", "nombre")}
            {header("Contacto")}
            {header("Viajes")}
            {header("Viaje abierto")}
            <th scope="col" className="col-chevron"><span className="sr-only">Abrir</span></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id} className="is-clickable" tabIndex={0} onClick={() => onOpen(row)}
              onKeyDown={(event) => { if (event.key === "Enter") onOpen(row); }}>
              <td data-label="Cliente">
                <span className="trips-client">
                  <ContactAvatar name={row.name} src={row.conversationId ? conversationAvatarUrl(row.conversationId) : undefined} className="avatar-32" />
                  <span>
                    <strong>{row.name}</strong>
                    <small>cliente desde {monthName(row.customerSince)}</small>
                  </span>
                </span>
              </td>
              <td data-label="Contacto"><ContactCell row={row} /></td>
              <td className="num" data-label="Viajes">
                {row.tripCount === 0 ? <span className="muted">Sin viajes</span> : <>
                  <span>{row.tripCount}</span>
                  {row.tripCount > 1 && row.lastDestination && <small>último: {row.lastDestination}</small>}
                </>}
              </td>
              <td data-label="Viaje abierto">
                {row.openTrip ? (
                  <span className="trips-dest">
                    <span>#{row.openTrip.number} {row.openTrip.destination || "sin destino"}{row.openTrip.departAt && ` · sale ${weekdayDate(row.openTrip.departAt)}`}</span>
                    <StatusPill status={row.openTrip.status} />
                  </span>
                ) : <span className="muted">—</span>}
              </td>
              <td className="col-chevron" aria-hidden><ChevronRight /></td>
            </tr>
          ))}
        </tbody>
      </table>
      {hasMore && onMore && (
        <div ref={sentinel} className="trips-more">
          <Button size="sm" variant="ghost" onClick={onMore}>Ver más clientes</Button>
        </div>
      )}
    </div>
  );
}

/**
 * Con WhatsApp, el globo verde y el teléfono del chat más reciente. Sin
 * WhatsApp, el teléfono (o el mail) en gris.
 */
function ContactCell({ row }: { row: CustomerRow }) {
  if (row.chatCount > 0) {
    return <>
      <span className="customer-contact"><MessageCircle aria-label="Con WhatsApp" className="is-wa" />{row.contactPhone ? formatPhone(row.contactPhone) : "WhatsApp"}</span>
      {row.chatCount > 1 && <small>{row.chatCount} chats</small>}
    </>;
  }
  if (!row.phoneNumber && !row.email) return <span className="muted">Sin contacto</span>;
  return <>
    <span className="customer-contact">
      {row.phoneNumber ? <><Phone aria-hidden />{formatTypedPhone(row.phoneNumber)}</> : <><Mail aria-hidden />{row.email}</>}
    </span>
    <small>{row.phoneNumber && row.email ? `${row.email} · sin WhatsApp` : "sin WhatsApp"}</small>
  </>;
}
