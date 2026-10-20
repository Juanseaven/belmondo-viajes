import { AlertTriangle } from "lucide-react";
import type { TripDetail } from "./types";
import { nextControl } from "./types";

const date = (value: string) => new Intl.DateTimeFormat("es-AR", { day: "numeric", month: "short" }).format(new Date(`${value.slice(0, 10)}T12:00:00`));
const money = (value: number) => new Intl.NumberFormat("es-AR").format(value);

export function TripBanner({ trip }: { trip: TripDetail }) {
  const critical = nextControl(trip.controls.filter((control) => control.critical));
  return (
    <div className="operation-banner trip-banner">
      <span className="operation-ref">#{trip.number}</span>
      <span className="operation-facts">
        <strong>{trip.destination || "Datos por completar"}</strong>
        {trip.departAt && trip.returnAt && <><i>·</i><span>{date(trip.departAt)} – {date(trip.returnAt)}</span></>}
        {trip.departAt && !trip.returnAt && <><i>·</i><span>Salida {date(trip.departAt)}</span></>}
        {!trip.departAt && trip.returnAt && <><i>·</i><span>Regreso {date(trip.returnAt)}</span></>}
        <i>·</i><span>{trip.passengers.length} pax</span>
        {trip.payment?.totalUsd !== undefined && <><i>·</i><span>USD {money(trip.payment.totalUsd)}</span></>}
      </span>
      {critical && <span className="operation-critical"><AlertTriangle aria-hidden />{critical.title}</span>}
    </div>
  );
}
