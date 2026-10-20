import { api } from "@/kernel/api/client";
import type { Country, Customer, CustomerUpdate, InboxItem, Passenger, Payment, PendingRow, Permit, Requirement, Seller, Stop, TravelDocument, TripDetail, TripListParams, TripPage, TripSummary, TripUpdate } from "./types";

function listQuery(params: TripListParams): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "" || value === false) continue;
    query.set(key, value === true ? "1" : String(value));
  }
  const text = query.toString();
  return text ? `?${text}` : "";
}

export interface NewTrip {
  contactId: string; destination?: string; departAt?: string; returnAt?: string;
  /** Vacío: sale de Argentina. */
  originCountry?: string; itinerary?: Stop[];
}
/** El origen y los países se guardan juntos: el origen no puede estar entre ellos. */
export interface ItineraryInput { originCountry: string; itinerary: Stop[] }
export interface NewPassenger {
  firstName: string; lastName: string; kind?: "adulto" | "menor"; isHolder?: boolean;
  birthDate?: string; residenceCountry?: string; travelDocumentId?: string;
  /** El alta puede traer su primer documento, que queda como el de viaje. */
  documentType?: string; documentCountry?: string; documentNumber?: string; documentExpiresAt?: string;
}
export type DocumentInput = Omit<TravelDocument, "id" | "passengerId">;
export type PermitInput = Omit<Permit, "id" | "passengerId">;
/** El pago se guarda entero: lo que no viaja queda vacío en la base. */
export type PaymentInput = Omit<Payment, "tripId" | "depositPaidAt"> & { depositPaid?: boolean };

export const operacionesApi = {
  inbox: (contactIds: string[]) => api<{ items: InboxItem[] }>(`/api/operaciones/bandeja?contactos=${encodeURIComponent(contactIds.join(","))}`),
  trips: (contactId: string) => api<{ items: TripSummary[] }>(`/api/operaciones/viajes?contacto=${encodeURIComponent(contactId)}`),
  trip: (id: string) => api<TripDetail>(`/api/operaciones/viajes/${id}`),
  createTrip: (input: NewTrip) => api<TripSummary>("/api/operaciones/viajes", { method: "POST", body: JSON.stringify(input) }),
  updateTrip: (id: string, input: TripUpdate) =>
    api<TripSummary>(`/api/operaciones/viajes/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  addPassenger: (tripId: string, input: NewPassenger) => api<Passenger>(`/api/operaciones/viajes/${tripId}/pasajeros`, { method: "POST", body: JSON.stringify(input) }),
  updatePassenger: (id: string, input: Partial<NewPassenger>) => api<Passenger>(`/api/operaciones/pasajeros/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  deletePassenger: (id: string) => api<void>(`/api/operaciones/pasajeros/${id}`, { method: "DELETE" }),
  putPayment: (tripId: string, input: PaymentInput) => api<Payment>(`/api/operaciones/viajes/${tripId}/pago`, { method: "PUT", body: JSON.stringify(input) }),
  closeControl: (id: string, messageId?: string) => api<void>(`/api/operaciones/controles/${id}/cerrar`, { method: "POST", body: JSON.stringify({ messageId }) }),
  reopenControl: (id: string) => api<void>(`/api/operaciones/controles/${id}/reabrir`, { method: "POST", body: "{}" }),
  customer: (id: string) => api<Customer>(`/api/operaciones/clientes/${id}`),
  updateCustomer: (id: string, input: CustomerUpdate) => api<Customer>(`/api/operaciones/clientes/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  sellers: () => api<{ items: Seller[] }>("/api/operaciones/vendedores"),
  pending: () => api<{ items: PendingRow[] }>("/api/operaciones/pendientes"),
  listTrips: (params: TripListParams) => api<TripPage>(`/api/operaciones/viajes${listQuery(params)}`),
  putItinerary: (tripId: string, input: ItineraryInput) =>
    api<void>(`/api/operaciones/viajes/${tripId}/itinerario`, { method: "PUT", body: JSON.stringify(input) }),
  addDocument: (passengerId: string, input: DocumentInput) =>
    api<TravelDocument>(`/api/operaciones/pasajeros/${passengerId}/documentos`, { method: "POST", body: JSON.stringify(input) }),
  updateDocument: (id: string, input: Partial<DocumentInput>) =>
    api<TravelDocument>(`/api/operaciones/documentos/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  deleteDocument: (id: string) => api<void>(`/api/operaciones/documentos/${id}`, { method: "DELETE" }),
  addPermit: (passengerId: string, input: PermitInput) =>
    api<Permit>(`/api/operaciones/pasajeros/${passengerId}/permisos`, { method: "POST", body: JSON.stringify(input) }),
  deletePermit: (id: string) => api<void>(`/api/operaciones/permisos/${id}`, { method: "DELETE" }),
  confirmRequirement: (tripId: string, requirement: Requirement) =>
    api<void>(`/api/operaciones/viajes/${tripId}/requisitos/confirmar`, { method: "POST", body: JSON.stringify({
      passengerId: requirement.passengerId, country: requirement.country ?? "", key: requirement.key,
    }) }),
  /** El catálogo de países con nombres en español. Lo sirve el módulo requisitos. */
  countries: () => api<{ items: Country[] }>("/api/requisitos/paises"),
};
