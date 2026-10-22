"use client";

import { useEffect, useState } from "react";
import { operacionesApi, type NewPassenger } from "./api";
import type { TripPanelProps } from "./TripPanel";
import type { Companion, TripDetail } from "./types";

type TripActions = Omit<TripPanelProps, "trip" | "countries" | "trips" | "sellers" | "collapsed" | "onClose" | "onSelectTrip" | "onNewTrip" | "onRunControl">;

/**
 * Lo que el panel del viaje puede escribir, conectado a la API. Lo usan el
 * panel al lado del chat y el que se abre sobre Clientes y Viajes, para que
 * los dos hagan lo mismo.
 *
 * Cada escritura recarga al terminar. El websocket también lo haría, pero quien
 * guardó no tiene por qué esperar a su propio eco.
 */
export function useTripActions(active: TripDetail | undefined, reload: () => Promise<void>): TripActions | undefined {
  const companions = useCompanions(active);

  if (!active) return undefined;
  const after = async <T,>(write: Promise<T>) => {
    const result = await write;
    await reload();
    return result;
  };

  return {
    onUpdateTrip: async (input) => { await after(operacionesApi.updateTrip(active.id, input)); },
    onCloseControl: async (control) => { await after(operacionesApi.closeControl(control.id)); },
    onReopenControl: async (control) => { await after(operacionesApi.reopenControl(control.id)); },
    onAddPassenger: async (input: NewPassenger) => { await after(operacionesApi.addPassenger(active.id, input)); },
    onUpdatePassenger: async (id, input) => { await after(operacionesApi.updatePassenger(id, input)); },
    onDeletePassenger: async (passenger) => { await after(operacionesApi.deletePassenger(passenger.id)); },
    onSavePayment: async (input) => { await after(operacionesApi.putPayment(active.id, input)); },
    onAddPaymentRecord: async (input) => { await after(operacionesApi.addPaymentRecord(active.id, input)); },
    onDeletePaymentRecord: async (record) => { await after(operacionesApi.deletePaymentRecord(record.id)); },
    onUpdateCustomer: async (input) => { await after(operacionesApi.updateCustomer(active.customer.id, input)); },
    onSaveItinerary: async (input) => { await after(operacionesApi.putItinerary(active.id, input)); },
    onAddDocument: async (passengerId, input) => { await after(operacionesApi.addDocument(passengerId, input)); },
    onUpdateDocument: async (id, input) => { await after(operacionesApi.updateDocument(id, input)); },
    onDeleteDocument: async (document) => { await after(operacionesApi.deleteDocument(document.id)); },
    onAddPermit: async (passengerId, input) => { await after(operacionesApi.addPermit(passengerId, input)); },
    onDeletePermit: async (permit) => { await after(operacionesApi.deletePermit(permit.id)); },
    onConfirmRequirement: async (requirement) => { await after(operacionesApi.confirmRequirement(active.id, requirement)); },
    companions: companions.items,
    onBringPassengers: async (ids) => { await after(operacionesApi.bringPassengers(active.id, ids)); },
  };
}

/**
 * Las personas de los otros viajes del cliente, para traerlas a este. Se
 * vuelven a pedir cuando cambian el viaje o sus pasajeros.
 */
function useCompanions(active: TripDetail | undefined) {
  const [items, setItems] = useState<Companion[]>([]);
  const tripId = active?.id;
  const passengersKey = active?.passengers.map((passenger) => passenger.id).join(",") ?? "";

  useEffect(() => {
    let current = true;
    if (!tripId) {
      setItems([]);
      return;
    }
    Promise.resolve(operacionesApi.tripCompanions(tripId))
      .then((response) => { if (current) setItems(response?.items ?? []); })
      .catch(() => { if (current) setItems([]); });
    return () => { current = false; };
  }, [tripId, passengersKey]);

  return { items };
}
