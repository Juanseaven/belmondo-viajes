import { CHECKLIST_TEMPLATE } from "./checklist";
import type { Control, ControlEvidence, Operation } from "./operations";

/**
 * Operaciones de la demo comercial.
 *
 * Tres expedientes completos, uno por cada estado que vale la pena mostrar: una
 * cotizada sin seña, un grupo con documentación a medias y una confirmada con
 * un comprobante sin conciliar. El resto de las conversaciones no tiene
 * operación todavía, que es lo que pasa con una consulta recién entrada.
 *
 * La referencia temporal es el 11 de agosto de 2026, igual que los mensajes.
 */

/** Lo que cada operación aporta sobre un renglón de la plantilla. */
interface RowState {
  done?: ControlEvidence;
  critical?: boolean;
  dueAt?: string;
  owner?: string;
}

/**
 * El checklist sale de la plantilla y la operación solo dice cómo está cada
 * renglón. Escribir treinta y un renglones por viaje sería copiar el proceso
 * tres veces y arriesgarse a que las copias se separen.
 */
function checklist(holder: string, state: Record<string, RowState>): Control[] {
  return CHECKLIST_TEMPLATE.map((item) => ({
    ...item,
    message: item.message?.replaceAll("{pax}", holder),
    ...state[item.id],
  }));
}

/** Azúcar para los renglones tildados a mano, que son la mayoría. */
const firmado = (by: string, at: string): RowState => ({ done: { by, at } });

const IGUAZU: Operation = {
  id: "10482",
  status: "cotizada",
  conversationId: "demo-1",
  destination: "Iguazú",
  departAt: "2026-09-12",
  returnAt: "2026-09-15",
  seller: "Juan Pérez",
  createdAt: "2026-08-11T11:39:00-03:00",
  createdBy: "Juan Pérez",
  passengers: [
    {
      id: "p-10482-1", firstName: "Ana", lastName: "Pérez", kind: "adulto", isHolder: true,
      document: { type: "DNI", number: "30.111.222", expiresAt: "2031-04-02" },
    },
    {
      id: "p-10482-2", firstName: "Diego", lastName: "Pérez", kind: "adulto", isHolder: false,
      document: { type: "DNI", number: "29.884.310", expiresAt: "2029-11-18" },
    },
    {
      id: "p-10482-3", firstName: "Sofía", lastName: "Pérez", kind: "menor", isHolder: false,
      document: { type: "DNI", number: "55.987.101", expiresAt: "2026-09-02" },
    },
  ],
  services: [
    {
      id: "s-10482-1", kind: "aereo", detail: "Aeroparque – Iguazú, ida y vuelta",
      provider: "Aerolíneas Argentinas", dates: "12 y 15 sep", baggage: "23 kg en bodega",
      status: "cotizado", amountUsd: 1090,
    },
    {
      id: "s-10482-2", kind: "hotel", detail: "Hotel Saint George, 3 noches",
      provider: "Saint George", dates: "12 – 15 sep", board: "Desayuno",
      status: "cotizado", amountUsd: 620,
    },
    {
      id: "s-10482-3", kind: "traslado", detail: "Aeropuerto – hotel – aeropuerto",
      provider: "Iguazú Transfers", status: "cotizado", amountUsd: 130,
    },
  ],
  payment: {
    totalUsd: 1840,
    rate: 1171,
    method: "Transferencia en pesos",
    depositUsd: 500,
    depositDueAt: "2026-08-12",
    depositPaid: false,
    nonRefundableUsd: 500,
  },
  documents: [
    {
      id: "d-10482-1", name: "DNI de Ana Pérez", kind: "documento",
      source: { via: "upload", by: "Juan Pérez", at: "2026-08-11T11:47:00-03:00" },
    },
    {
      id: "d-10482-2", name: "DNI de Diego Pérez", kind: "documento",
      source: { via: "upload", by: "Juan Pérez", at: "2026-08-11T11:48:00-03:00" },
    },
  ],
  controls: checklist("Ana", {
    "carga-nombres": firmado("Juan Pérez", "2026-08-11T11:40:00-03:00"),
    "carga-apellidos": firmado("Juan Pérez", "2026-08-11T11:40:00-03:00"),
    "carga-destino": firmado("Juan Pérez", "2026-08-11T11:40:00-03:00"),
    "carga-salida": firmado("Juan Pérez", "2026-08-11T11:41:00-03:00"),
    "carga-llegada": firmado("Juan Pérez", "2026-08-11T11:41:00-03:00"),
    "carga-contacto": firmado("Juan Pérez", "2026-08-11T11:41:00-03:00"),
    "carga-usd": firmado("Juan Pérez", "2026-08-11T11:43:00-03:00"),
    "carga-pesos": firmado("Juan Pérez", "2026-08-11T11:43:00-03:00"),
    "carga-cambio": firmado("Juan Pérez", "2026-08-11T11:43:00-03:00"),
    "carga-metodo": firmado("Juan Pérez", "2026-08-11T11:43:00-03:00"),
    "carga-acepto": { owner: "Juan Pérez", dueAt: "2026-08-12" },
    "control-destino": firmado("Juan Pérez", "2026-08-11T11:43:00-03:00"),
    "control-fechas-vuelo": firmado("Juan Pérez", "2026-08-11T11:43:00-03:00"),
    // El DNI de Sofía vence el 2 de septiembre y el viaje vuelve el 15.
    "control-vto-doc": { critical: true, owner: "Juan Pérez", dueAt: "2026-08-14" },
  }),
};

const BARILOCHE: Operation = {
  id: "10475",
  status: "cotizada",
  conversationId: "demo-2",
  destination: "Bariloche",
  departAt: "2026-10-03",
  returnAt: "2026-10-08",
  seller: "María López",
  createdAt: "2026-08-06T10:15:00-03:00",
  createdBy: "María López",
  passengers: [
    {
      id: "p-10475-1", firstName: "Lucas", lastName: "García", kind: "adulto", isHolder: true,
      document: { type: "DNI", number: "27.660.914", expiresAt: "2030-06-21" },
    },
    {
      id: "p-10475-2", firstName: "Paula", lastName: "García", kind: "adulto", isHolder: false,
      document: { type: "DNI", number: "28.114.550", expiresAt: "2031-01-09" },
    },
    {
      id: "p-10475-3", firstName: "Martín", lastName: "López", kind: "adulto", isHolder: false,
      document: { type: "DNI", number: "30.402.887", expiresAt: "2028-08-30" },
    },
    {
      id: "p-10475-4", firstName: "Laura", lastName: "López", kind: "adulto", isHolder: false,
      document: { type: "DNI", number: "31.775.203", expiresAt: "2029-02-14" },
    },
    { id: "p-10475-5", firstName: "Diego", lastName: "Rodríguez", kind: "adulto", isHolder: false, document: { type: "DNI", number: "" } },
    { id: "p-10475-6", firstName: "Carla", lastName: "Rodríguez", kind: "adulto", isHolder: false, document: { type: "DNI", number: "" } },
    { id: "p-10475-7", firstName: "Pablo", lastName: "Sánchez", kind: "adulto", isHolder: false, document: { type: "DNI", number: "" } },
    { id: "p-10475-8", firstName: "Ana", lastName: "Sánchez", kind: "adulto", isHolder: false, document: { type: "DNI", number: "" } },
  ],
  services: [
    {
      id: "s-10475-1", kind: "aereo", detail: "Aeroparque – Bariloche, ida y vuelta, 8 pasajeros",
      provider: "Aerolíneas Argentinas", dates: "3 y 8 oct", baggage: "15 kg en bodega",
      status: "cotizado", amountUsd: 2960,
    },
    {
      id: "s-10475-2", kind: "hotel", detail: "Hotel Panamericano, 5 noches, 4 dobles",
      provider: "Panamericano", dates: "3 – 8 oct", board: "Media pensión",
      status: "cotizado", amountUsd: 1680,
    },
    {
      id: "s-10475-3", kind: "traslado", detail: "Micro privado aeropuerto – hotel – aeropuerto",
      provider: "Andes Bus", status: "cotizado", amountUsd: 320,
    },
  ],
  payment: {
    totalUsd: 4960,
    rate: 1171,
    method: "Transferencia en pesos",
    depositUsd: 1200,
    depositDueAt: "2026-08-20",
    depositPaid: false,
  },
  documents: [],
  controls: checklist("Lucas", {
    "carga-nombres": firmado("María López", "2026-08-06T10:20:00-03:00"),
    "carga-apellidos": firmado("María López", "2026-08-06T10:20:00-03:00"),
    "carga-destino": firmado("María López", "2026-08-06T10:20:00-03:00"),
    "carga-salida": firmado("María López", "2026-08-06T10:21:00-03:00"),
    "carga-llegada": firmado("María López", "2026-08-06T10:21:00-03:00"),
    "carga-contacto": firmado("María López", "2026-08-06T10:22:00-03:00"),
    "carga-usd": firmado("María López", "2026-08-11T11:26:00-03:00"),
    "carga-pesos": firmado("María López", "2026-08-11T11:26:00-03:00"),
    "carga-cambio": firmado("María López", "2026-08-11T11:26:00-03:00"),
    "carga-acepto": { owner: "María López", dueAt: "2026-08-20" },
    "control-destino": firmado("María López", "2026-08-06T10:22:00-03:00"),
    "control-fechas-vuelo": firmado("María López", "2026-08-06T10:22:00-03:00"),
    "control-traslados": { owner: "María López" },
  }),
};

const CANCUN: Operation = {
  id: "10471",
  status: "confirmada",
  conversationId: "demo-3",
  destination: "Cancún",
  departAt: "2026-11-20",
  returnAt: "2026-11-27",
  seller: "María López",
  createdAt: "2026-08-08T15:02:00-03:00",
  createdBy: "María López",
  passengers: [
    {
      id: "p-10471-1", firstName: "Martín", lastName: "Gómez", kind: "adulto", isHolder: true,
      document: { type: "Pasaporte", number: "AAF302118", expiresAt: "2027-03-15" },
    },
    {
      id: "p-10471-2", firstName: "Julia", lastName: "Gómez", kind: "adulto", isHolder: false,
      document: { type: "Pasaporte", number: "AAD771904", expiresAt: "2026-10-30" },
    },
  ],
  services: [
    {
      id: "s-10471-1", kind: "aereo", detail: "Ezeiza – Cancún, ida y vuelta",
      provider: "Copa Airlines", dates: "20 y 27 nov", baggage: "23 kg en bodega",
      status: "reservado", amountUsd: 2100,
    },
    {
      id: "s-10471-2", kind: "hotel", detail: "Riu Palace, 7 noches",
      provider: "Riu", dates: "20 – 27 nov", board: "All inclusive",
      status: "reservado", amountUsd: 1050,
    },
    {
      id: "s-10471-3", kind: "asistencia", detail: "Asistencia al viajero, 8 días",
      provider: "Assist Card", status: "cotizado", amountUsd: 250,
    },
  ],
  payment: {
    totalUsd: 3400,
    rate: 1171,
    method: "Transferencia en pesos",
    depositUsd: 600,
    depositDueAt: "2026-08-15",
    depositPaid: false,
    nonRefundableUsd: 900,
  },
  documents: [
    {
      id: "d-10471-1", name: "Comprobante de transferencia", kind: "comprobante",
      source: { via: "whatsapp", messageId: "demo-3-1", at: "2026-08-11T10:52:00-03:00" },
    },
    {
      id: "d-10471-2", name: "Pasaporte de Martín Gómez", kind: "documento",
      source: { via: "upload", by: "María López", at: "2026-08-09T09:30:00-03:00" },
    },
  ],
  controls: checklist("Martín", {
    "carga-nombres": firmado("María López", "2026-08-08T15:05:00-03:00"),
    "carga-apellidos": firmado("María López", "2026-08-08T15:05:00-03:00"),
    "carga-destino": firmado("María López", "2026-08-08T15:05:00-03:00"),
    "carga-salida": firmado("María López", "2026-08-08T15:06:00-03:00"),
    "carga-llegada": firmado("María López", "2026-08-08T15:06:00-03:00"),
    "carga-contacto": firmado("María López", "2026-08-08T15:07:00-03:00"),
    "carga-usd": firmado("María López", "2026-08-08T15:10:00-03:00"),
    "carga-pesos": firmado("María López", "2026-08-08T15:10:00-03:00"),
    "carga-cambio": firmado("María López", "2026-08-08T15:10:00-03:00"),
    "carga-metodo": firmado("María López", "2026-08-08T15:10:00-03:00"),
    // El comprobante llegó por WhatsApp: el renglón se cerró con el mensaje.
    "carga-comprobante": {
      done: { by: "María López", at: "2026-08-11T10:53:00-03:00", messageId: "demo-3-1" },
    },
    "carga-acepto": {
      done: { by: "María López", at: "2026-08-09T11:20:00-03:00", messageId: "demo-3-2" },
    },
    "control-titular": firmado("María López", "2026-08-09T09:35:00-03:00"),
    "control-orden": firmado("María López", "2026-08-09T09:35:00-03:00"),
    "control-fechas-vuelo": firmado("María López", "2026-08-09T09:36:00-03:00"),
    "control-destino": firmado("María López", "2026-08-09T09:36:00-03:00"),
    "control-equipaje": firmado("María López", "2026-08-09T09:37:00-03:00"),
    "control-visa": firmado("María López", "2026-08-08T15:12:00-03:00"),
    "control-hotel-liquidacion": firmado("María López", "2026-08-09T09:40:00-03:00"),
    "control-hotel-datos": firmado("María López", "2026-08-09T09:40:00-03:00"),
    "control-comidas": firmado("María López", "2026-08-09T09:41:00-03:00"),
    // El pasaporte de Julia vence el 30 de octubre y el viaje sale el 20 de noviembre.
    "control-vto-doc": { critical: true, owner: "María López", dueAt: "2026-08-18" },
    "control-asistencia": { owner: "María López" },
  }),
};

export const DEMO_OPERATIONS: Operation[] = [IGUAZU, BARILOCHE, CANCUN];

export const operationFor = (conversationId: string): Operation | undefined =>
  DEMO_OPERATIONS.find((operation) => operation.conversationId === conversationId);
