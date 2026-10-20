import type { TripSummary } from "./types";

const STATUS: Record<TripSummary["status"], string> = {
  cotizado: "Cotizado", confirmado: "Confirmado", "en viaje": "Viaje en curso", cerrado: "Cerrado",
};

export function TripChip({ trip }: { trip: TripSummary }) {
  const next = trip.nextControl;
  return (
    <span className={next?.critical ? "trip-chip critical" : "trip-chip"}>
      {next?.title ?? STATUS[trip.status]}
    </span>
  );
}

