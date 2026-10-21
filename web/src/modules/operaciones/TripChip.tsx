import { Plane } from "lucide-react";
import { dayOf, today } from "./format";
import type { TripSummary } from "./types";

/**
 * El viaje abierto del contacto, en su fila de la bandeja: el número y el
 * destino, y «Atrasado» si el próximo control ya se pasó. La bandeja trae solo
 * el próximo control, no cuántos hay atrasados: por eso la etiqueta no lleva número.
 */
export function TripChip({ trip, from = today() }: { trip: TripSummary; from?: string }) {
  const due = trip.nextControl?.dueAt;
  const late = due !== undefined && dayOf(due) < from;
  return (
    <>
      <span className="tag"><Plane aria-hidden />#{trip.number}{trip.destination ? ` ${trip.destination}` : ""}</span>
      {late && <span className="tag is-late">Atrasado</span>}
    </>
  );
}
