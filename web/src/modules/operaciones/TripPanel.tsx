"use client";

import { useEffect, useState } from "react";
import { ArrowRight, CalendarDays, ChevronDown, MapPin, Pencil, Plus, UserRound, UsersRound, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { ItineraryInput, NewPassenger, PaymentInput } from "./api";
import { countryName } from "./countries";
import { ControlsTab } from "./ControlsTab";
import { longDay } from "./format";
import { PassengersTab, type DocumentActions, type PassengerPatch } from "./PassengersTab";
import { PaymentEditor, PaymentTab } from "./PaymentTab";
import { RequirementsTab, requirementCounts } from "./RequirementsTab";
import { StatusMenu, StatusPill } from "./status";
import { SummaryTab } from "./SummaryTab";
import { TripEditor } from "./TripEditor";
import type { Companion, Control, Country, CustomerUpdate, Passenger, Requirement, Seller, TripDetail, TripStatus, TripSummary, TripUpdate } from "./types";

/** Lo que alcanza para cambiar de viaje desde el número. */
type TripChoice = Pick<TripSummary, "id" | "number" | "destination" | "status">;

type Tab = "resumen" | "pasajeros" | "requisitos" | "pagos" | "controles";

export interface TripPanelProps extends DocumentActions {
  trip: TripDetail;
  /** El catálogo de países, para elegirlos por nombre. */
  countries?: Country[];
  /** Los viajes del contacto. Con más de uno, el número del viaje deja cambiar. */
  trips?: TripChoice[];
  sellers?: Seller[];
  collapsed?: boolean;
  onClose?: () => void;
  onSelectTrip?: (tripId: string) => void;
  onNewTrip?: () => void;
  onUpdateTrip?: (input: TripUpdate) => Promise<void>;
  /** Precarga el mensaje del control en el chat. Sin chat no hay: se marca a mano. */
  onRunControl?: (control: Control) => void;
  onCloseControl: (control: Control) => void;
  onReopenControl: (control: Control) => void;
  onAddPassenger: (input: NewPassenger) => Promise<void> | void;
  onUpdatePassenger?: (id: string, input: PassengerPatch) => Promise<void>;
  onDeletePassenger?: (passenger: Passenger) => Promise<void>;
  onSavePayment: (input: PaymentInput) => Promise<void> | void;
  onUpdateCustomer?: (input: CustomerUpdate) => Promise<void>;
  onSaveItinerary?: (input: ItineraryInput) => Promise<void>;
  onConfirmRequirement?: (requirement: Requirement) => Promise<void>;
  /** Las personas de los otros viajes del cliente, para traerlas a este. */
  companions?: Companion[];
  onBringPassengers?: (passengerIds: string[]) => Promise<void>;
}

/**
 * El panel del viaje, al lado del chat. Se lee primero y se edita cuando hace
 * falta: los datos se muestran como datos, y los formularios aparecen al tocar
 * «Editar», reemplazando el contenido del panel.
 */
export function TripPanel({
  trip, trips, sellers, countries = [], collapsed, onClose, onSelectTrip, onNewTrip, onUpdateTrip,
  onRunControl, onCloseControl, onReopenControl, onAddPassenger, onUpdatePassenger,
  onDeletePassenger, onSavePayment, onUpdateCustomer, onSaveItinerary, onConfirmRequirement, companions,
  onBringPassengers, ...documentActions
}: TripPanelProps) {
  const [tab, setTab] = useState<Tab>("resumen");
  const [editor, setEditor] = useState<"viaje" | "pago">();
  // «Cargar visa» desde Requisitos: lleva a Pasajeros con ese pasajero abierto.
  const [focus, setFocus] = useState<{ passengerId: string; addPermit?: boolean }>();
  const [error, setError] = useState<string>();

  useEffect(() => { setEditor(undefined); setError(undefined); }, [trip.id]);

  // Un cliente sin chat no puede recibir el mensaje que prueba el control: se
  // marca a mano. Sin el dato (un viaje leído antes de existir), se asume chat.
  const markByHand = trip.customer.hasChat === false;
  const controlActions = { onRun: onRunControl ?? (() => undefined), onClose: onCloseControl, onReopen: onReopenControl, markByHand };
  const missing = requirementCounts(trip.requirements ?? []).falta;
  const openControls = trip.controls.filter((control) => control.status === "abierto").length;

  const changeStatus = async (status: TripStatus) => {
    if (!onUpdateTrip || status === trip.status) return;
    setError(undefined);
    try {
      await onUpdateTrip({ status });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo cambiar el estado");
    }
  };

  const panelClass = `trip-panel${collapsed ? " is-collapsed" : ""}`;
  const panelProps = { "aria-label": "Viaje", "aria-hidden": collapsed || undefined, inert: collapsed || undefined } as const;

  if (editor === "viaje") {
    return (
      <aside className={panelClass} {...panelProps}>
        <TripEditor trip={trip} countries={countries} sellers={sellers} onBack={() => setEditor(undefined)}
          onUpdateTrip={onUpdateTrip} onSaveItinerary={onSaveItinerary} onUpdateCustomer={onUpdateCustomer} />
      </aside>
    );
  }
  if (editor === "pago") {
    return (
      <aside className={panelClass} {...panelProps}>
        <PaymentEditor tripNumber={trip.number} payment={trip.payment} onBack={() => setEditor(undefined)}
          onSave={async (input) => { await onSavePayment(input); setEditor(undefined); }} />
      </aside>
    );
  }

  const tabs: { value: Tab; label: string; count?: number; bad?: boolean }[] = [
    { value: "resumen", label: "Resumen" },
    { value: "pasajeros", label: "Pasajeros", count: trip.passengers.length || undefined },
    { value: "requisitos", label: "Requisitos", count: missing || undefined, bad: true },
    { value: "pagos", label: "Pagos" },
    { value: "controles", label: "Controles", count: openControls || undefined },
  ];

  return (
    <aside className={panelClass} {...panelProps}>
      <header className="panel-head">
        <div className="panel-title">
          <div>
            {trips && trips.length > 1 && onSelectTrip
              ? <TripSwitcher trip={trip} trips={trips} onSelect={onSelectTrip} />
              : <strong>Viaje #{trip.number}</strong>}
            <StatusMenu status={trip.status} disabled={!onUpdateTrip} onChange={(status) => void changeStatus(status)} />
          </div>
          <small>{trip.destination || "Destino sin cargar"} · {trip.sellerName ? `vende ${trip.sellerName}` : "sin vendedor"}</small>
        </div>
        {onNewTrip && <Button variant="ghost" size="icon" className="icon-button" aria-label="Nuevo viaje" title="Nuevo viaje" onClick={onNewTrip}><Plus /></Button>}
        {onClose && <Button variant="ghost" size="icon" className="icon-button" aria-label="Ocultar viaje" title="Ocultar viaje" onClick={onClose}><X /></Button>}
      </header>
      {error && <p className="panel-error" role="alert">{error}</p>}

      {tab === "resumen" && <>
        <TripFacts trip={trip} countries={countries} />
        <div className="panel-edit-link">
          <button type="button" className="link-button" onClick={() => setEditor("viaje")}><Pencil aria-hidden />Editar viaje y cliente</button>
        </div>
      </>}

      <div className="panel-tabs" role="tablist" aria-label="Secciones del viaje">
        {tabs.map(({ value, label, count, bad }) => (
          <button key={value} type="button" role="tab" aria-selected={tab === value} onClick={() => setTab(value)}>
            {label}{count !== undefined && <> <span className={bad ? "tab-count is-bad" : "tab-count"}>{count}</span></>}
          </button>
        ))}
      </div>

      <div className="panel-body">
        {tab === "resumen" && <SummaryTab trip={trip} onTab={setTab} {...controlActions} />}
        {tab === "pasajeros" && (
          <PassengersTab trip={trip} countries={countries} focus={focus} onFocused={() => setFocus(undefined)}
            onAddPassenger={onAddPassenger} onUpdatePassenger={onUpdatePassenger} onDeletePassenger={onDeletePassenger}
            companions={companions} onBringPassengers={onBringPassengers} {...documentActions} />
        )}
        {tab === "requisitos" && (
          <RequirementsTab trip={trip} countries={countries} onConfirm={onConfirmRequirement}
            onAddPermit={documentActions.onAddPermit ? (passengerId) => { setFocus({ passengerId, addPermit: true }); setTab("pasajeros"); } : undefined} />
        )}
        {tab === "pagos" && <PaymentTab payment={trip.payment} onEdit={() => setEditor("pago")} />}
        {tab === "controles" && <ControlsTab controls={trip.controls} {...controlActions} />}
      </div>
    </aside>
  );
}

/** El número del viaje, que abre la lista de viajes del contacto para cambiar. */
function TripSwitcher({ trip, trips, onSelect }: { trip: TripDetail; trips: TripChoice[]; onSelect: (id: string) => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" className="trip-switcher" aria-label={`Viaje #${trip.number}, cambiar de viaje`}>
          Viaje #{trip.number}<ChevronDown aria-hidden />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" sideOffset={6} className="trip-switcher-menu">
        <DropdownMenuRadioGroup value={trip.id} onValueChange={onSelect}>
          {trips.map((item) => (
            <DropdownMenuRadioItem key={item.id} value={item.id}>
              <span className="trip-switcher-item">
                <strong>Viaje #{item.number}</strong>
                <small>{item.destination || "Destino sin cargar"}</small>
              </span>
              <StatusPill status={item.status} />
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Los datos del viaje a la vista: valor en negrita, rótulo chico arriba, ícono a la izquierda. */
function TripFacts({ trip, countries }: { trip: TripDetail; countries: Country[] }) {
  const route = [trip.originCountry || "AR", ...(trip.itinerary ?? []).map((stop) => stop.country)];
  const adults = trip.passengers.filter((p) => p.kind === "adulto").length;
  const minors = trip.passengers.length - adults;
  const people = trip.passengers.length === 0
    ? undefined
    : `${trip.passengers.length} · ${[adults ? `${adults} ${adults === 1 ? "adulto" : "adultos"}` : "", minors ? `${minors} ${minors === 1 ? "menor" : "menores"}` : ""].filter(Boolean).join(", ")}`;

  return (
    <dl className="trip-facts">
      <div className="trip-fact is-wide">
        <MapPin aria-hidden />
        <dt>Recorrido</dt>
        <dd className="trip-route-line">
          {route.length === 1
            ? <span className="muted">Sin destinos</span>
            : route.map((code, index) => (
              <span key={`${code}-${index}`}>{index > 0 && <ArrowRight aria-hidden />}{countryName(countries, code)}</span>
            ))}
        </dd>
      </div>
      <div className="trip-fact"><CalendarDays aria-hidden /><dt>Salida</dt><dd>{trip.departAt ? longDay(trip.departAt) : <span className="muted">Sin fecha</span>}</dd></div>
      <div className="trip-fact"><CalendarDays aria-hidden /><dt>Regreso</dt><dd>{trip.returnAt ? longDay(trip.returnAt) : <span className="muted">Sin fecha</span>}</dd></div>
      <div className="trip-fact"><UserRound aria-hidden /><dt>Cliente</dt><dd>{trip.customer.name}</dd></div>
      <div className="trip-fact"><UsersRound aria-hidden /><dt>Pasajeros</dt><dd>{people ?? <span className="muted">Sin cargar</span>}</dd></div>
    </dl>
  );
}
