import { api } from "@/kernel/api/client";
import type { Customer, InboxItem, Passenger, Payment, TripDetail, TripSummary } from "./types";

export interface NewTrip { contactId: string; destination?: string; departAt?: string; returnAt?: string }
export interface NewPassenger { firstName: string; lastName: string; kind?: "adulto" | "menor"; isHolder?: boolean; documentType?: string; documentNumber?: string; documentExpiresAt?: string }
export type PaymentInput = Omit<Payment, "tripId" | "depositPaidAt"> & { depositPaid?: boolean };

export const operacionesApi = {
  inbox: (contactIds: string[]) => api<{ items: InboxItem[] }>(`/api/operaciones/bandeja?contactos=${encodeURIComponent(contactIds.join(","))}`),
  trips: (contactId: string) => api<{ items: TripSummary[] }>(`/api/operaciones/viajes?contacto=${encodeURIComponent(contactId)}`),
  trip: (id: string) => api<TripDetail>(`/api/operaciones/viajes/${id}`),
  createTrip: (input: NewTrip) => api<TripSummary>("/api/operaciones/viajes", { method: "POST", body: JSON.stringify(input) }),
  updateTrip: (id: string, input: Partial<Pick<TripSummary, "destination" | "departAt" | "returnAt" | "status" | "sellerUserId">>) =>
    api<TripSummary>(`/api/operaciones/viajes/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  addPassenger: (tripId: string, input: NewPassenger) => api<Passenger>(`/api/operaciones/viajes/${tripId}/pasajeros`, { method: "POST", body: JSON.stringify(input) }),
  updatePassenger: (id: string, input: Partial<NewPassenger>) => api<Passenger>(`/api/operaciones/pasajeros/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  deletePassenger: (id: string) => api<void>(`/api/operaciones/pasajeros/${id}`, { method: "DELETE" }),
  putPayment: (tripId: string, input: PaymentInput) => api<Payment>(`/api/operaciones/viajes/${tripId}/pago`, { method: "PUT", body: JSON.stringify(input) }),
  closeControl: (id: string, messageId?: string) => api<void>(`/api/operaciones/controles/${id}/cerrar`, { method: "POST", body: JSON.stringify({ messageId }) }),
  reopenControl: (id: string) => api<void>(`/api/operaciones/controles/${id}/reabrir`, { method: "POST", body: "{}" }),
  customer: (id: string) => api<Customer>(`/api/operaciones/clientes/${id}`),
};
