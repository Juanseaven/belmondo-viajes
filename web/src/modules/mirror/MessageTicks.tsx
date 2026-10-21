import { AlertCircle, Check, CheckCheck, Clock3 } from "lucide-react";

/**
 * Los tildes de WhatsApp: uno gris enviado, dos grises entregado, dos azules
 * leído. Solo para lo que mandó la agencia; lo que recibe no lleva.
 */
export function MessageTicks({ status, className }: { status?: string; className?: string }) {
  const cls = (extra: string) => `ticks ${extra}${className ? ` ${className}` : ""}`;
  switch (status) {
    case "read":
      return <CheckCheck className={cls("is-read")} aria-label="Leído" />;
    case "delivered":
      return <CheckCheck className={cls("")} aria-label="Entregado" />;
    case "pending":
      return <Clock3 className={cls("")} aria-label="Enviando" />;
    case "failed":
      return <AlertCircle className={cls("is-failed")} aria-label="No se envió" />;
    default:
      return <Check className={cls("")} aria-label="Enviado" />;
  }
}
