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
