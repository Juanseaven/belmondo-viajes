import { AlertTriangle, CalendarDays, Plane, UserRound } from "lucide-react";
import { dayOf, shortDate as date, today } from "./format";
import type { TripDetail } from "./types";

/**
 * La barra del viaje sobre el chat: mientras se conversa, el viaje queda a la
 * vista. La alerta dice cuántos controles importantes están atrasados; cuál es,
 * lo dice el panel, porque el título completo no entra.
 */
export function TripBanner({ trip, from = today() }: { trip: TripDetail; from?: string }) {
  const late = trip.controls.filter((control) =>
    control.status === "abierto" && control.critical && control.dueAt !== undefined && dayOf(control.dueAt) < from).length;
  const people = trip.passengers.length;
  return (
    <div className="trip-bar">
      <span className="trip-bar-ref"><Plane aria-hidden /><span>Viaje #{trip.number} · {trip.destination || "Datos por completar"}</span></span>
      {(trip.departAt || trip.returnAt) && (
        <span className="trip-bar-fact"><CalendarDays aria-hidden />{dates(trip.departAt, trip.returnAt)}</span>
      )}
      <span className="trip-bar-fact"><UserRound aria-hidden />{people === 0 ? "Sin pasajeros" : `${people} ${people === 1 ? "pasajero" : "pasajeros"}`}</span>
      {late > 0 && (
        <span className="trip-bar-alert"><AlertTriangle aria-hidden />{late} {late === 1 ? "control atrasado" : "controles atrasados"}</span>
      )}
    </div>
  );
}

function dates(departAt?: string, returnAt?: string) {
  if (departAt && returnAt) return `${date(departAt)} – ${date(returnAt)}`;
  if (departAt) return `Sale el ${date(departAt)}`;
  return `Vuelve el ${date(returnAt!)}`;
}
