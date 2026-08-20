/**
 * La operación: el expediente de un viaje vendido.
 *
 * Reemplaza dos planillas en papel de la agencia —la de carga de reserva y la
 * de control— sin convertirlas en una lista de casilleros. La regla que ordena
 * todo está en `Control.closing`: cuando la acción se puede ejecutar desde acá,
 * el control se cierra al ejecutarla y queda la prueba; cuando no, se confirma
 * a mano pero registrando quién y cuándo.
 *
 * Un tilde suelto sería una declaración. Acá el estado es una consecuencia.
 */

export type OperationStatus = "cotizada" | "confirmada" | "en viaje" | "cerrada";

/**
 * El pasajero, no el conteo. La planilla controla nombre y apellido del titular,
 * el orden entre ellos y el vencimiento del documento: nada de eso entra en
 * «2 ad + 1 men».
 */
export interface Passenger {
  id: string;
  /** Tal como figura en el documento. */
  firstName: string;
  lastName: string;
  kind: "adulto" | "menor";
  isHolder: boolean;
  document: { type: "DNI" | "Pasaporte"; number: string; expiresAt?: string };
}

export type ServiceKind = "aereo" | "hotel" | "traslado" | "asistencia" | "excursion";

/**
 * Lo que el pasajero compró. `amountUsd` es lo que la segunda tabla de la
 * planilla de control anotaba a mano por separado.
 */
export interface Service {
  id: string;
  kind: ServiceKind;
  detail: string;
  provider?: string;
  dates?: string;
  /** Régimen de comidas: la planilla lo controla renglón por renglón. */
  board?: string;
  baggage?: string;
  status: "cotizado" | "reservado" | "emitido";
  amountUsd: number;
}

export interface Payment {
  totalUsd: number;
  /** Cambio del día, como lo pide la planilla de carga. */
  rate: number;
  method: string;
  depositUsd: number;
  depositDueAt?: string;
  depositPaid: boolean;
  /** Monto no reembolsable en caso de cancelación. */
  nonRefundableUsd?: number;
}

export type DocumentSource =
  | { via: "whatsapp"; messageId: string; at: string }
  | { via: "upload"; by: string; at: string };

export interface OperationDocument {
  id: string;
  name: string;
  kind: "documento" | "comprobante" | "voucher";
  /** De dónde salió. Sin procedencia un documento no prueba nada. */
  source: DocumentSource;
}

/** `carga` es la planilla de venta; `control`, la de auditoría posterior. */
export type ControlStage = "carga" | "control";

export interface ControlEvidence {
  at: string;
  by: string;
  /** La prueba. Los controles manuales no la tienen: solo firma y fecha. */
  messageId?: string;
  documentId?: string;
}

export interface Control {
  id: string;
  stage: ControlStage;
  /** Sigue la redacción de la planilla; también se usa como chip en la bandeja. */
  title: string;
  critical?: boolean;
  dueAt?: string;
  owner?: string;
  closing: "accion" | "manual";
  /** Qué dice el botón cuando se cierra por acción. */
  actionLabel?: string;
  /** Lo que sale por la conversación al ejecutar la acción: la prueba misma. */
  message?: string;
  done?: ControlEvidence;
}

export interface Operation {
  id: string;
  status: OperationStatus;
  conversationId: string;
  destination: string;
  departAt: string;
  returnAt: string;
  /** La persona que vendió, no la línea de WhatsApp por la que entró. */
  seller: string;
  createdAt: string;
  createdBy: string;
  passengers: Passenger[];
  services: Service[];
  payment: Payment;
  documents: OperationDocument[];
  controls: Control[];
}

export const openControls = (operation: Operation): Control[] =>
  operation.controls.filter((control) => !control.done);

export const criticalControls = (operation: Operation): Control[] =>
  openControls(operation).filter((control) => control.critical);

/**
 * El control que hay que mirar primero: crítico antes que común, y dentro de
 * cada grupo el que vence antes. Lo que no vence va al final.
 */
export function nextControl(operation: Operation): Control | undefined {
  return [...openControls(operation)].sort((a, b) => {
    if (Boolean(a.critical) !== Boolean(b.critical)) return a.critical ? -1 : 1;
    if (a.dueAt && b.dueAt) return a.dueAt.localeCompare(b.dueAt);
    if (a.dueAt) return -1;
    if (b.dueAt) return 1;
    return 0;
  })[0];
}

export interface OperationRowState {
  label: string;
  tone?: "warn" | "late";
}

/**
 * Lo que ve el supervisor sin abrir la conversación. Es la columna que la
 * planilla de control quería ser, así que no se escribe a mano: sale de los
 * controles abiertos y, si no queda ninguno, del estado de la operación.
 */
export function operationRowState(operation: Operation): OperationRowState {
  const next = nextControl(operation);
  if (!next) return { label: STATUS_LABEL[operation.status] };
  return { label: next.title, tone: next.critical ? "late" : "warn" };
}

const STATUS_LABEL: Record<OperationStatus, string> = {
  cotizada: "Cotizada",
  confirmada: "Confirmada",
  "en viaje": "Viaje en curso",
  cerrada: "Cerrada",
};

export const statusLabel = (status: OperationStatus): string => STATUS_LABEL[status];

export const servicesTotal = (operation: Operation): number =>
  operation.services.reduce((sum, service) => sum + service.amountUsd, 0);

export const balanceUsd = (operation: Operation): number =>
  operation.payment.totalUsd - (operation.payment.depositPaid ? operation.payment.depositUsd : 0);

export const totalArs = (operation: Operation): number =>
  Math.round(operation.payment.totalUsd * operation.payment.rate);

/**
 * El «control de liquidación» de la planilla: que los montos cotizados
 * coincidan. Acá no se tilda, se calcula.
 */
export const liquidationMatches = (operation: Operation): boolean =>
  servicesTotal(operation) === operation.payment.totalUsd;

/**
 * El documento tiene que seguir vigente cuando el pasajero vuelve, no cuando
 * sale. Es el error que la planilla intenta atajar con «VTO DNI/PASAPORTE».
 */
export const documentExpires = (passenger: Passenger, returnAt: string): boolean =>
  passenger.document.expiresAt !== undefined && passenger.document.expiresAt < returnAt;

export const expiringPassengers = (operation: Operation): Passenger[] =>
  operation.passengers.filter((passenger) => documentExpires(passenger, operation.returnAt));
