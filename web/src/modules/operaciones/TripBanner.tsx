import { AlertTriangle } from "lucide-react";
import { money, shortDate as date } from "./format";
import type { TripDetail } from "./types";
import { nextControl } from "./types";

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
