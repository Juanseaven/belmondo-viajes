import { api } from "@/kernel/api/client";
import type { Companion, Country, CustomerFile, FilePage, PaymentRecord, Customer, CustomerDetail, CustomerListParams, CustomerMatch, CustomerPage, CustomerUpdate, FreeContact, InboxItem, Passenger, Payment, PendingRow, Permit, Requirement, Seller, Stop, TravelDocument, TripDetail, TripListParams, TripPage, TripSummary, TripUpdate } from "./types";

function listQuery(params: TripListParams | CustomerListParams | Record<string, string | undefined>): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "" || value === false) continue;
    query.set(key, value === true ? "1" : String(value));
  }
  const text = query.toString();
  return text ? `?${text}` : "";
}

/** Un viaje es de un chat (contactId) o de un cliente cargado a mano (customerId): uno de los dos. */
export interface NewTrip {
  contactId?: string; customerId?: string; destination?: string; departAt?: string; returnAt?: string;
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
export interface NewCustomer { name: string; email?: string; phoneNumber?: string; documentType?: string; documentNumber?: string }
/** Los datos de un cliente que se está cargando, para avisar si ya existe. */
export interface SimilarInput { nombre?: string; telefono?: string; mail?: string; documento?: string }
export type PaymentInput = Omit<Payment, "tripId" | "depositPaidAt" | "totalArs" | "depositPaidBefore">;
export interface NewPaymentRecord { amount: number; currency: "USD" | "ARS"; rate?: number; paidAt?: string; method?: string; note?: string }

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
  customer: (id: string) => api<CustomerDetail>(`/api/operaciones/clientes/${id}`),
  customers: (params: CustomerListParams) => api<CustomerPage>(`/api/operaciones/clientes${listQuery(params)}`),
  createCustomer: (input: NewCustomer) => api<Customer>("/api/operaciones/clientes", { method: "POST", body: JSON.stringify(input) }),
  similarCustomers: (input: SimilarInput) => api<{ items: CustomerMatch[] }>(`/api/operaciones/clientes/parecidos${listQuery({ ...input })}`),
  linkContact: (customerId: string, contactId: string) =>
    api<void>(`/api/operaciones/clientes/${customerId}/contactos`, { method: "POST", body: JSON.stringify({ contactId }) }),
  /** Los chats sin cliente; con cliente, primero los que coinciden con su teléfono. */
  freeContacts: (q: string, customerId?: string) => api<{ items: FreeContact[] }>(`/api/operaciones/contactos${listQuery({ q, cliente: customerId })}`),
  /** El cliente del contacto, si tiene; si no, los que podrían serlo por el teléfono. */
  contactCustomers: (contactId: string) =>
    api<{ customerId?: string; items: CustomerMatch[] }>(`/api/operaciones/contactos/${contactId}/clientes`),
  tripCompanions: (tripId: string) => api<{ items: Companion[] }>(`/api/operaciones/viajes/${tripId}/companeros`),
  bringPassengers: (tripId: string, passengerIds: string[]) =>
    api<{ items: Passenger[] }>(`/api/operaciones/viajes/${tripId}/pasajeros/traer`, { method: "POST", body: JSON.stringify({ passengerIds }) }),
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
  addPaymentRecord: (tripId: string, input: NewPaymentRecord) =>
    api<PaymentRecord>(`/api/operaciones/viajes/${tripId}/pagos`, { method: "POST", body: JSON.stringify(input) }),
  deletePaymentRecord: (id: string) => api<void>(`/api/operaciones/pagos/${id}`, { method: "DELETE" }),
  /** Una de las dos listas de Archivos: fotos y videos, o documentos. */
  customerFiles: (customerId: string, params: { tipo: "media" | "documentos"; favoritos?: boolean; cursor?: string; limit?: number }) =>
    api<FilePage>(`/api/operaciones/clientes/${customerId}/archivos${listQuery({
      tipo: params.tipo, favoritos: params.favoritos ? "1" : undefined, cursor: params.cursor, limit: params.limit?.toString(),
    })}`),
  setFavorite: (file: Pick<CustomerFile, "messageId">, on: boolean) =>
    api<void>(`/api/operaciones/archivos/${file.messageId}/favorito`, { method: on ? "PUT" : "DELETE" }),
  /** El catálogo de países con nombres en español. Lo sirve el módulo requisitos. */
  countries: () => api<{ items: Country[] }>("/api/requisitos/paises"),
};
