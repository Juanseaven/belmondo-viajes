"use client";

import { useEffect, useState } from "react";
import { operacionesApi, type NewPassenger } from "./api";
import { NewTripPanel, type TripSeed } from "./NewTripPanel";
import { TripPanel } from "./TripPanel";
import type { Control, Country, Seller } from "./types";
import type { useTrips } from "./useTrips";

interface TripSidebarProps {
  contactId: string;
  contactName: string;
  trips: ReturnType<typeof useTrips>;
  collapsed?: boolean;
  onClose?: () => void;
  /** Un control por acción no envía: precarga el composer del hilo. */
  onRunControl: (control: Control) => void;
}

/**
 * La ficha del viaje en la pantalla del espejo. Es la única pieza que habla con
 * la API de operaciones: la pantalla le pasa el contacto abierto y recibe el
 * pedido de precargar el composer, nada más.
 *
 * Cada escritura recarga la ficha al terminar. El websocket también lo haría,
 * pero quien guardó no tiene por qué esperar a su propio eco.
 */
export function TripSidebar({ contactId, contactName, trips, collapsed, onClose, onRunControl }: TripSidebarProps) {
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

  // El primer dato guardado crea el viaje: abrir la conversación no escribe.
  const createDraft = async (seed: TripSeed) => {
    const trip = await operacionesApi.createTrip({ contactId, ...seed });
    trips.select(trip.id);
    setCreating(false);
    return trip;
  };

  if (!active || creating) {
    return (
      <NewTripPanel
        contactName={contactName}
        countries={countries}
        collapsed={collapsed}
        onClose={onClose}
        onCancel={active ? () => setCreating(false) : undefined}
        onCreate={async (seed) => { await after(createDraft(seed)); }}
        onAddPassenger={async (input) => {
          const trip = await createDraft({});
          await after(operacionesApi.addPassenger(trip.id, input));
        }}
        onSavePayment={async (input) => {
          const trip = await createDraft({});
          await after(operacionesApi.putPayment(trip.id, input));
        }}
      />
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
