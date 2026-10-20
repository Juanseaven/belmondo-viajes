"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useSocket } from "@/kernel/api/useSocket";
import { operacionesApi } from "./api";
import type { TripDetail, TripSummary } from "./types";

/**
 * preferredTripId es el viaje con el que se llegó al chat desde la pantalla de
 * Viajes: manda sobre el que elegiría la bandeja hasta que se cambie de contacto
 * o de viaje.
 */
export function useTrips(contactIds: string[], activeContactId?: string, preferredTripId?: string) {
  const contactKey = useMemo(() => [...new Set(contactIds)].join(","), [contactIds.join(",")]);
  const [byContact, setByContact] = useState<Record<string, TripSummary | undefined>>({});
  const [contactTrips, setContactTrips] = useState<TripSummary[]>([]);
  // El viaje que eligió quien atiende. Sin elección, manda el que la bandeja
  // considera activo: el más reciente sin cerrar.
  const [selectedId, setSelectedId] = useState<string | undefined>(preferredTripId);
  const [active, setActive] = useState<TripDetail>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => { setSelectedId(preferredTripId); }, [activeContactId, preferredTripId]);

  const reload = useCallback(async () => {
    const ids = contactKey ? contactKey.split(",") : [];
    setLoading(true);
    try {
      const [response, own] = await Promise.all([
        operacionesApi.inbox(ids),
        activeContactId ? operacionesApi.trips(activeContactId) : Promise.resolve({ items: [] as TripSummary[] }),
      ]);
      const next: Record<string, TripSummary | undefined> = {};
      for (const item of response.items) next[item.contactId] = item.trip;
      setByContact(next);
      setContactTrips(own.items);
      // Una elección que ya no existe (otro usuario borró el viaje) no se sostiene.
      const chosen = selectedId && own.items.some((trip) => trip.id === selectedId) ? selectedId : undefined;
      const tripId = chosen ?? (activeContactId ? next[activeContactId]?.id : undefined);
      setActive(tripId ? await operacionesApi.trip(tripId) : undefined);
      setError(undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudieron cargar los viajes");
    } finally {
      setLoading(false);
    }
  }, [activeContactId, contactKey, selectedId]);

  useEffect(() => { void reload(); }, [reload]);
  useSocket("operaciones", () => { void reload(); });

  return { byContact, contactTrips, active, loading, error, reload, select: setSelectedId };
}
