import { AlertTriangle } from "lucide-react";

import { criticalControls, type Operation } from "./operations";

const money = (amount: number) => new Intl.NumberFormat("es-AR").format(amount);
const SHORT = new Intl.DateTimeFormat("es-AR", { day: "numeric", month: "short" });
const day = (iso: string) => SHORT.format(new Date(`${iso.slice(0, 10)}T12:00:00`));

/**
 * La franja que conecta las dos mitades de la pantalla: mientras se conversa,
 * el viaje que se está negociando queda a la vista. Si hay un control crítico
 * abierto, se dice acá y no solo en el panel.
 */
export function OperationBanner({ operation }: { operation: Operation }) {
  const adults = operation.passengers.filter((passenger) => passenger.kind === "adulto").length;
  const minors = operation.passengers.length - adults;
  const critical = criticalControls(operation)[0];

  return (
    <div className="operation-banner">
      <span className="operation-ref">#{operation.id}</span>
      <span className="operation-facts">
        <strong>{operation.destination}</strong>
        <i aria-hidden>·</i>
        <span className="num">{day(operation.departAt)} – {day(operation.returnAt)}</span>
        <i aria-hidden>·</i>
        <span className="num">{adults} ad{minors > 0 && ` + ${minors} men`}</span>
        <i aria-hidden>·</i>
        <span className="num">USD {money(operation.payment.totalUsd)}</span>
      </span>
      {critical && (
        <span className="operation-critical">
          <AlertTriangle aria-hidden />
          {critical.title}
        </span>
      )}
    </div>
  );
}
