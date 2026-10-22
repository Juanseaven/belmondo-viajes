"use client";

import { useCallback, useEffect, useState } from "react";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useSocket } from "@/kernel/api/useSocket";
import { operacionesApi } from "./api";
import { NewTripPanel, type TripSeed } from "./NewTripPanel";
import { TripPanel } from "./TripPanel";
import type { Country, CustomerDetail, Seller, TripDetail, TripSummary } from "./types";
import { useTripActions } from "./useTripActions";

/**
 * El panel del viaje fuera del chat: entra desde la derecha sobre la página
 * del cliente, o sobre Viajes, para los viajes de clientes sin WhatsApp. Es el
 * mismo panel de la bandeja, con las mismas pestañas; lo que allá se cierra
 * mandando un mensaje, acá se marca a mano.
 *
 * Con `creating`, muestra el formulario de viaje nuevo para ese cliente.
 */
export function TripDrawer({ customerId, tripId, creating, onClose, onSelectTrip, onCreated }: {
  customerId: string;
  tripId?: string;
  creating?: boolean;
  onClose: () => void;
  onSelectTrip: (tripId: string) => void;
  onCreated?: (trip: TripSummary, customer: CustomerDetail) => void;
}) {
  const open = Boolean(tripId || creating);
  const { customer, trip, reload, error } = useCustomerTrip(customerId, open ? tripId : undefined);
  const { sellers, countries } = useCatalogs(open);
  const actions = useTripActions(trip, reload);

  const create = async (seed: TripSeed) => {
    const created = await operacionesApi.createTrip({ customerId, ...seed });
    if (customer) onCreated?.(created, customer);
  };

  let body = null;
  if (creating && customer) {
    body = (
      <aside className="trip-panel" aria-label="Viaje">
        <NewTripPanel contactName={customer.name} countries={countries} onCancel={onClose} onCreate={create}
          clientNote="Los pasajeros y el pago se cargan después, en el viaje." />
      </aside>
    );
  } else if (trip && actions) {
    body = (
      <TripPanel trip={trip} trips={customer?.trips} sellers={sellers} countries={countries}
        onClose={onClose} onSelectTrip={onSelectTrip} {...actions} />
    );
  } else if (error) {
    body = <aside className="trip-panel" aria-label="Viaje"><p className="panel-error" role="alert">{error}</p></aside>;
  }

  return (
    <Sheet open={open} onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent className="details-sheet trip-sheet" showCloseButton={false}>
        <SheetHeader className="sr-only">
          <SheetTitle>{creating ? "Nuevo viaje" : trip ? `Viaje #${trip.number}` : "Viaje"}</SheetTitle>
          <SheetDescription>{customer?.name ?? ""}</SheetDescription>
        </SheetHeader>
        {body ?? <aside className="trip-panel" aria-label="Viaje"><p className="panel-note trip-sheet-loading">Cargando el viaje…</p></aside>}
      </SheetContent>
    </Sheet>
  );
}

/** El cliente (para el selector de viajes y el nombre) y el viaje elegido. */
function useCustomerTrip(customerId: string, tripId?: string) {
  const [customer, setCustomer] = useState<CustomerDetail>();
  const [trip, setTrip] = useState<TripDetail>();
  const [error, setError] = useState<string>();

  const reload = useCallback(async () => {
    try {
      const [nextCustomer, nextTrip] = await Promise.all([
        operacionesApi.customer(customerId),
        tripId ? operacionesApi.trip(tripId) : Promise.resolve(undefined),
      ]);
      setCustomer(nextCustomer);
      setTrip(nextTrip);
      setError(undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo cargar el viaje");
    }
  }, [customerId, tripId]);

  useEffect(() => {
    setTrip(undefined);
    void reload();
  }, [reload]);
  useSocket("operaciones", () => { void reload(); });

  return { customer, trip, reload, error };
}

function useCatalogs(open: boolean) {
  const [sellers, setSellers] = useState<Seller[]>([]);
  const [countries, setCountries] = useState<Country[]>([]);
  useEffect(() => {
    if (!open) return;
    let current = true;
    operacionesApi.sellers()
      .then((response) => { if (current) setSellers(response.items); })
      .catch(() => { if (current) setSellers([]); });
    Promise.resolve(operacionesApi.countries())
      .then((response) => { if (current && response) setCountries(response.items); })
      .catch(() => { if (current) setCountries([]); });
    return () => { current = false; };
  }, [open]);
  return { sellers, countries };
}
