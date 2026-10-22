"use client";

import { useEffect, useState, type ReactNode } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ContactDetails } from "@/modules/mirror/ContactDetails";
import { ContactCustomer } from "./ContactCustomer";
import type { Conversation } from "@/modules/mirror/api";
import { operacionesApi } from "./api";
import { NewTripPanel, type TripSeed } from "./NewTripPanel";
import { StatusPill } from "./status";
import { TripPanel } from "./TripPanel";
import type { Control, Country, Seller } from "./types";
import { useTripActions } from "./useTripActions";
import type { useTrips } from "./useTrips";

interface TripSidebarProps {
  conversation: Pick<Conversation, "contactId" | "contactName" | "avatarUrl" | "phoneNumber" | "instanceLabel" | "isGroup">;
  trips: ReturnType<typeof useTrips>;
  collapsed?: boolean;
  onClose?: () => void;
  /** Un control por acción no envía: precarga el composer del hilo. */
  onRunControl: (control: Control) => void;
}

/**
 * La ficha del viaje en la pantalla del espejo. Es la única pieza que habla con
 * la API de operaciones: la pantalla le pasa la conversación abierta y recibe
 * el pedido de precargar el composer, nada más.
 *
 * Cada escritura recarga la ficha al terminar. El websocket también lo haría,
 * pero quien guardó no tiene por qué esperar a su propio eco.
 */
export function TripSidebar({ conversation, trips, collapsed, onClose, onRunControl }: TripSidebarProps) {
  const { contactId } = conversation;
  const [sellers, setSellers] = useState<Seller[]>([]);
  const [countries, setCountries] = useState<Country[]>([]);
  const [creating, setCreating] = useState(false);
  const active = trips.active;

  useEffect(() => {
    let current = true;
    operacionesApi.sellers()
      .then((response) => { if (current) setSellers(response.items); })
      .catch(() => { if (current) setSellers([]); });
    return () => { current = false; };
  }, []);

  useEffect(() => {
    let current = true;
    Promise.resolve(operacionesApi.countries())
      .then((response) => { if (current && response) setCountries(response.items); })
      .catch(() => { if (current) setCountries([]); });
    return () => { current = false; };
  }, []);

  useEffect(() => { setCreating(false); }, [contactId]);

  const actions = useTripActions(active, trips.reload);

  const create = async (seed: TripSeed) => {
    const trip = await operacionesApi.createTrip({ contactId, ...seed });
    trips.select(trip.id);
    await trips.reload();
    setCreating(false);
  };

  const shell = (children: ReactNode) => (
    <aside className={`trip-panel${collapsed ? " is-collapsed" : ""}`} aria-label="Viaje" aria-hidden={collapsed || undefined} inert={collapsed || undefined}>
      {children}
    </aside>
  );

  if (creating) {
    return shell(
      <NewTripPanel contactName={conversation.contactName} avatarUrl={conversation.avatarUrl} countries={countries}
        onCancel={() => setCreating(false)} onCreate={create} />,
    );
  }

  if (!active || !actions) {
    const previous = trips.contactTrips;
    return shell(
      <ContactDetails conversation={conversation} onClose={onClose}>
        {!conversation.isGroup && <ContactCustomer contactId={contactId} contactName={conversation.contactName} onLinked={() => void trips.reload()} />}
        <div className="panel-empty">
          <strong>{previous.length > 0 ? "Este contacto no tiene viajes abiertos" : "Este contacto no tiene viajes"}</strong>
          <p>Cargá el recorrido y las fechas; los pasajeros y el pago se completan después.</p>
          <Button size="sm" onClick={() => setCreating(true)}><Plus aria-hidden />Nuevo viaje</Button>
        </div>
        {previous.length > 0 && <>
          <h3 className="panel-group-title">Viajes anteriores</h3>
          <ul className="panel-card trip-history">
            {previous.map((trip) => (
              <li key={trip.id}>
                <button type="button" onClick={() => trips.select(trip.id)}>
                  <span><strong>Viaje #{trip.number}</strong><small>{trip.destination || "Destino sin cargar"}</small></span>
                  <StatusPill status={trip.status} />
                </button>
              </li>
            ))}
          </ul>
        </>}
      </ContactDetails>,
    );
  }

  return (
    <TripPanel
      trip={active}
      trips={trips.contactTrips}
      sellers={sellers}
      countries={countries}
      collapsed={collapsed}
      onClose={onClose}
      onSelectTrip={trips.select}
      onNewTrip={() => setCreating(true)}
      onRunControl={onRunControl}
      {...actions}
    />
  );
}
