export type TripStatus = "cotizado" | "confirmado" | "en viaje" | "cerrado";
export type Closing = "derivado" | "accion" | "manual";

export interface ControlSummary {
  id: string;
  itemId: string;
  title: string;
  critical: boolean;
  dueAt?: string;
  closing: Closing;
}

export interface TripSummary {
  id: string;
  number: number;
  customerId: string;
  status: TripStatus;
  destination: string;
  departAt?: string;
  returnAt?: string;
  sellerUserId?: string;
  sellerName?: string;
  createdAt: string;
  passengerCount: number;
  totalUsd?: number;
  nextControl?: ControlSummary;
}

export interface Customer {
  id: string;
  name: string;
  email?: string;
  phoneNumber?: string;
  documentType?: string;
  documentNumber?: string;
  customerSince: string;
  contactIds: string[];
}

export interface Passenger {
  id: string;
  tripId: string;
  firstName: string;
  lastName: string;
  kind: "adulto" | "menor";
  isHolder: boolean;
  documentType?: "DNI" | "Pasaporte";
  documentNumber?: string;
  documentExpiresAt?: string;
}

export interface Payment {
  tripId: string;
  totalUsd?: number;
  rate?: number;
  totalArs?: number;
  method?: string;
  depositUsd?: number;
  depositDueAt?: string;
  depositPaidAt?: string;
  nonRefundableUsd?: number;
}

export interface Evidence {
  id: string;
  closedBy?: string;
  closedByName?: string;
  closedAt: string;
  messageId?: string;
}

export interface Control extends ControlSummary {
  stage: "carga" | "control";
  prompt?: string;
  actionLabel?: string;
  message?: string;
  status: "abierto" | "cerrado";
  evidence: Evidence[];
}

export interface TripDetail extends TripSummary {
  customer: Customer;
  passengers: Passenger[];
  payment: Payment | null;
  controls: Control[];
}

export interface InboxItem { contactId: string; trip?: TripSummary }

/** Un usuario al que se le puede asignar un viaje. */
export interface Seller { id: string; name: string }

/** Lo que la ficha puede cambiar del viaje. Un vendedor vacío lo desasigna. */
export type TripUpdate = Partial<Pick<TripSummary, "destination" | "departAt" | "returnAt" | "status" | "sellerUserId">>;

export type CustomerUpdate = Partial<Pick<Customer, "name" | "email" | "phoneNumber" | "documentType" | "documentNumber">>;

export const nextControl = (controls: Control[]): Control | undefined =>
  [...controls]
    .filter((control) => control.status === "abierto")
    .sort((a, b) => {
      if (a.critical !== b.critical) return a.critical ? -1 : 1;
      if (a.dueAt && b.dueAt) return a.dueAt.localeCompare(b.dueAt);
      if (a.dueAt) return -1;
      if (b.dueAt) return 1;
      return 0;
    })[0];

/** Algo de un viaje que tiene plazo: un control abierto o la seña sin cobrar. */
export interface PendingItem {
  kind: "control" | "sena";
  id?: string;
  itemId?: string;
  /** La tarea dicha como orden: «Agendar el check in del vuelo de ida». */
  task: string;
  dueAt: string;
  critical: boolean;
}

/** Un viaje como se ve en las listas de la pantalla de Viajes. */
export interface TripRow {
  id: string;
  number: number;
  status: TripStatus;
  destination: string;
  departAt?: string;
  returnAt?: string;
  customerName: string;
  passengerCount: number;
  sellerName?: string;
  totalUsd?: number;
  depositUsd?: number;
  depositDueAt?: string;
  depositPaidAt?: string;
  /** Lo más urgente del viaje, venza cuando venza. Sin esto, está al día. */
  next?: PendingItem;
  /** La conversación más reciente del cliente. Sin esto, no hay chat que abrir. */
  conversationId?: string;
}

export type PendingGroup = "atrasado" | "semana" | "salen";

export interface PendingRow extends TripRow {
  group: PendingGroup;
  /** Encabeza el renglón. Sin esto, el viaje sale pronto y está al día. */
  task?: PendingItem;
  more: number;
}

export interface TripPage { items: TripRow[]; total: number; nextCursor?: string }

export type TripSort = "salida" | "numero" | "total";

/** Los parámetros de «Todos los viajes», como los espera el backend. */
export interface TripListParams {
  q?: string;
  estado?: string;
  salidaDesde?: string;
  salidaHasta?: string;
  vendedor?: string;
  atrasados?: boolean;
  orden?: TripSort;
  cursor?: string;
}
