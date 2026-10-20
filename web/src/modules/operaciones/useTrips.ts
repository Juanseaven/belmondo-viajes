"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useSocket } from "@/kernel/api/useSocket";
import { operacionesApi } from "./api";
import type { TripDetail, TripSummary } from "./types";

export function useTrips(contactIds: string[], activeContactId?: string) {
  const contactKey = useMemo(() => [...new Set(contactIds)].join(","), [contactIds.join(",")]);
  const [byContact, setByContact] = useState<Record<string, TripSummary | undefined>>({});
  const [active, setActive] = useState<TripDetail>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  const reload = useCallback(async () => {
    const ids = contactKey ? contactKey.split(",") : [];
    setLoading(true);
    try {
      const response = await operacionesApi.inbox(ids);
      const next: Record<string, TripSummary | undefined> = {};
      for (const item of response.items) next[item.contactId] = item.trip;
      setByContact(next);
      const summary = activeContactId ? next[activeContactId] : undefined;
      setActive(summary ? await operacionesApi.trip(summary.id) : undefined);
      setError(undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudieron cargar los viajes");
    } finally {
      setLoading(false);
    }
  }, [activeContactId, contactKey]);

  useEffect(() => { void reload(); }, [reload]);
  useSocket("operaciones", () => { void reload(); });

  return { byContact, active, loading, error, reload };
}
