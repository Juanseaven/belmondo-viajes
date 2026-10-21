"use client";

import { useEffect, useState, type ReactNode } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ContactDetails } from "@/modules/mirror/ContactDetails";
import type { Conversation } from "@/modules/mirror/api";
import { operacionesApi, type NewPassenger } from "./api";
import { NewTripPanel, type TripSeed } from "./NewTripPanel";
import { StatusPill } from "./status";
import { TripPanel } from "./TripPanel";
import type { Control, Country, Seller } from "./types";
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

  const after = async <T,>(write: Promise<T>) => {
    const result = await write;
    await trips.reload();
    return result;
  };

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

  if (!active) {
    const previous = trips.contactTrips;
    return shell(
      <ContactDetails conversation={conversation} onClose={onClose}>
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
      onUpdateTrip={async (input) => { await after(operacionesApi.updateTrip(active.id, input)); }}
      onRunControl={onRunControl}
      onCloseControl={async (control) => { await after(operacionesApi.closeControl(control.id)); }}
      onReopenControl={async (control) => { await after(operacionesApi.reopenControl(control.id)); }}
      onAddPassenger={async (input: NewPassenger) => { await after(operacionesApi.addPassenger(active.id, input)); }}
      onUpdatePassenger={async (id, input) => { await after(operacionesApi.updatePassenger(id, input)); }}
      onDeletePassenger={async (passenger) => { await after(operacionesApi.deletePassenger(passenger.id)); }}
      onSavePayment={async (input) => { await after(operacionesApi.putPayment(active.id, input)); }}
      onUpdateCustomer={async (input) => { await after(operacionesApi.updateCustomer(active.customer.id, input)); }}
      onSaveItinerary={async (input) => { await after(operacionesApi.putItinerary(active.id, input)); }}
      onAddDocument={async (passengerId, input) => { await after(operacionesApi.addDocument(passengerId, input)); }}
      onUpdateDocument={async (id, input) => { await after(operacionesApi.updateDocument(id, input)); }}
      onDeleteDocument={async (document) => { await after(operacionesApi.deleteDocument(document.id)); }}
      onAddPermit={async (passengerId, input) => { await after(operacionesApi.addPermit(passengerId, input)); }}
      onDeletePermit={async (permit) => { await after(operacionesApi.deletePermit(permit.id)); }}
      onConfirmRequirement={async (requirement) => { await after(operacionesApi.confirmRequirement(active.id, requirement)); }}
    />
  );
}
