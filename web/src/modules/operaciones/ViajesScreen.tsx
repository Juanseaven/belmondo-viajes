"use client";

/**
 * La pantalla de Viajes: la primera propia del módulo operaciones.
 *
 * Abre en «Pendientes», la lista de trabajo que arma el sistema, porque la
 * agencia no tiene un hábito de búsqueda que esta pantalla deba respetar: lo
 * que más le sirve es que nada se pase. «Todos los viajes» es la consulta.
 *
 * La pestaña y los filtros viven en la URL, como en el espejo. Toda acción lleva
 * al chat, que es donde se resuelve: la ficha está al lado de la conversación.
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useSocket } from "@/kernel/api/useSocket";
import { operacionesApi } from "./api";
import { today } from "./format";
import { PendingList } from "./PendingList";
import { defaultFilters, filtersFromParams, filtersToParams, hasActiveFilters, toListParams, type TripFilters } from "./tripFilters";
import { TripFiltersBar } from "./TripFiltersBar";
import { TripsTable } from "./TripsTable";
import type { PendingRow, Seller, TripRow, TripSort } from "./types";

const PATH = "/viajes";

type Tab = "pendientes" | "todos";

export function ViajesScreen() {
  const params = useSearchParams();
  const router = useRouter();
  const tab: Tab = params.get("tab") === "todos" ? "todos" : "pendientes";
  const filters = useMemo(() => filtersFromParams(params), [params]);
  const pending = usePending();

  const go = useCallback((nextTab: Tab, nextFilters: TripFilters) => {
    const query = nextTab === "todos" ? filtersToParams(nextFilters) : new URLSearchParams();
    if (nextTab === "todos") {
      const withTab = new URLSearchParams([["tab", "todos"], ...query.entries()]);
      router.replace(`${PATH}?${withTab.toString()}`, { scroll: false });
    } else {
      router.replace(PATH, { scroll: false });
    }
  }, [router]);

  const openChat = (row: TripRow) => {
    if (!row.conversationId) return;
    router.push(`/mirror?c=${encodeURIComponent(row.conversationId)}&viaje=${encodeURIComponent(row.id)}`);
  };

  const asking = pending.rows.filter((row) => row.group !== "salen").length;

  return (
    <section className="viajes-screen">
      <header className="viajes-head"><h1>Viajes</h1></header>
      <div className="viajes-tabs" role="tablist" aria-label="Vistas de viajes">
        <button type="button" role="tab" aria-selected={tab === "pendientes"} onClick={() => go("pendientes", filters)}>
          Pendientes{asking > 0 && <> <span className="viajes-count">{asking}</span></>}
        </button>
        <button type="button" role="tab" aria-selected={tab === "todos"} onClick={() => go("todos", filters)}>
          Todos los viajes
        </button>
      </div>
      <div className="viajes-body">
        {tab === "pendientes"
          ? <PendingView state={pending} onOpen={openChat} />
          : <AllTripsView filters={filters} onFilters={(next) => go("todos", next)} onOpen={openChat} />}
      </div>
    </section>
  );
}

/**
 * Los pendientes se piden siempre, aunque se esté mirando la otra pestaña: el
 * número de la pestaña dice cuántos viajes piden algo. La lista depende del día,
 * así que también se recarga al volver a la pestaña del navegador: una abierta
 * desde ayer mostraría lo de ayer.
 */
function usePending() {
  const [rows, setRows] = useState<PendingRow[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string>();

  const reload = useCallback(async () => {
    try {
      const response = await operacionesApi.pending();
      setRows(response.items);
      setError(undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudieron cargar los pendientes");
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => { void reload(); }, [reload]);
  useSocket("operaciones", () => { void reload(); });
  useEffect(() => {
    const onVisible = () => { if (document.visibilityState === "visible") void reload(); };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [reload]);

  return { rows, loaded, error };
}

function PendingView({ state, onOpen }: { state: ReturnType<typeof usePending>; onOpen: (row: TripRow) => void }) {
  if (state.error) return <p className="viajes-error" role="alert">{state.error}</p>;
  if (!state.loaded) return <p className="viajes-loading">Cargando pendientes…</p>;
  return <PendingList rows={state.rows} onOpen={onOpen} />;
}

function AllTripsView({ filters, onFilters, onOpen }: { filters: TripFilters; onFilters: (next: TripFilters) => void; onOpen: (row: TripRow) => void }) {
  const [rows, setRows] = useState<TripRow[]>([]);
  const [total, setTotal] = useState(0);
  const [cursor, setCursor] = useState<string>();
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string>();
  const [sellers, setSellers] = useState<Seller[]>([]);
  const [version, setVersion] = useState(0);
  const params = useMemo(() => toListParams(filters, today()), [filters]);
  const key = JSON.stringify(params);

  useEffect(() => {
    operacionesApi.sellers().then((response) => setSellers(response.items)).catch(() => setSellers([]));
  }, []);

  // Un evento del módulo recarga la primera página: un viaje cambió y puede haber
  // entrado o salido de los filtros.
  useSocket("operaciones", () => setVersion((value) => value + 1));

  useEffect(() => {
    let current = true;
    // La búsqueda se tipea: se espera un instante para no consultar por letra.
    const timer = setTimeout(() => {
      operacionesApi.listTrips(params)
        .then((page) => {
          if (!current) return;
          setRows(page.items);
          setTotal(page.total);
          setCursor(page.nextCursor);
          setError(undefined);
        })
        .catch((cause) => { if (current) setError(cause instanceof Error ? cause.message : "No se pudieron cargar los viajes"); })
        .finally(() => { if (current) setLoaded(true); });
    }, 200);
    return () => { current = false; clearTimeout(timer); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, version]);

  const loadMore = useCallback(() => {
    if (!cursor) return;
    const next = cursor;
    setCursor(undefined);
    operacionesApi.listTrips({ ...params, cursor: next })
      .then((page) => {
        setRows((current) => [...current, ...page.items]);
        setCursor(page.nextCursor);
      })
      .catch(() => setCursor(next));
  }, [cursor, params]);

  return (
    <>
      <TripFiltersBar value={filters} sellers={sellers} onChange={onFilters} />
      {error && <p className="viajes-error" role="alert">{error}</p>}
      {!loaded ? <p className="viajes-loading">Cargando viajes…</p> : (
        <TripsTable
          rows={rows}
          total={total}
          sort={filters.orden}
          onSort={(orden: TripSort) => onFilters({ ...filters, orden })}
          onOpen={onOpen}
          onClear={() => onFilters({ ...defaultFilters(), orden: filters.orden })}
          hasMore={Boolean(cursor)}
          onMore={loadMore}
        />
      )}
      {loaded && rows.length > 0 && hasActiveFilters(filters) && (
        <button type="button" className="trips-clear" onClick={() => onFilters({ ...defaultFilters(), orden: filters.orden })}>Limpiar filtros</button>
      )}
    </>
  );
}
