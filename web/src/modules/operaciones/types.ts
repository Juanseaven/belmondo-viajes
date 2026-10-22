export type TripStatus = "consulta" | "cotizado" | "confirmado" | "en viaje" | "cerrado";
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
  /**
   * Alguno de sus contactos tiene un chat individual. Sin chat, los controles
   * que se cierran mandando un mensaje se marcan a mano.
   */
  hasChat?: boolean;
}

export interface Passenger {
  id: string;
  tripId: string;
  firstName: string;
  lastName: string;
  /** Con fecha de nacimiento, lo dice la fecha a la salida; sin ella, se elige. */
  kind: "adulto" | "menor";
  isHolder: boolean;
  birthDate?: string;
  residenceCountry: string;
  /** Con qué documento viaja en este viaje: define la nacionalidad que se evalúa. */
  travelDocumentId?: string;
  /** Lo eligió una persona: el sistema no lo cambia. Si no, lo elige el sistema. */
  travelDocumentManual?: boolean;
  documents: TravelDocument[];
  permits: Permit[];
}

/** Un DNI o pasaporte. La nacionalidad es la del país que lo emitió. */
export interface TravelDocument {
  id: string;
  passengerId: string;
  type: "DNI" | "Pasaporte";
  country: string;
  number?: string;
  issuedAt?: string;
  expiresAt?: string;
}

export type PermitKind = "visa" | "autorizacion_electronica" | "residencia";

/** Una visa, autorización electrónica o residencia que el pasajero ya tiene. */
export interface Permit {
  id: string;
  passengerId: string;
  kind: PermitKind;
  country: string;
  label?: string;
  validFrom?: string;
  expiresAt?: string;
}

/** Un país del itinerario. Sin fechas, la estadía es la del viaje. */
export interface Stop {
  country: string;
  role: "destino" | "escala";
  arriveAt?: string;
  leaveAt?: string;
}

export interface Country { code: string; name: string }

export interface Citation {
  source: string; quote?: string; url?: string; revision?: string;
  /** Día en que se leyó una página oficial (AAAA-MM-DD). */
  readAt?: string;
}

/** Una frase del detalle; la que sale de una fuente la lleva. */
export interface DetailLine { text: string; source?: Citation }

/**
 * Lo que explica un renglón: qué pide el país, el trámite, qué tiene el
 * pasajero y qué hacer. `oficial` dice si el detalle de las páginas oficiales
 * todavía se busca o no se encontró.
 */
export interface RequirementDetail {
  pide?: DetailLine[];
  tramite?: DetailLine[];
  tiene?: string[];
  hacer?: string[];
  oficial?: "buscando" | "sin_datos";
}

/** Una línea del panel de requisitos: qué cumple, qué falta y qué verificar. */
export interface Requirement {
  /** Vacío: es del viaje (falta el itinerario). */
  passengerId?: string;
  /** Vacío: no es de un país (la autorización de menor). */
  country?: string;
  role?: "destino" | "escala" | "";
  key: string;
  /** «aviso» es una recomendación: no es un faltante. */
  status: "cumple" | "falta" | "verificar" | "aviso";
  /** El nombre del requisito; el detalle lo explica. */
  text: string;
  detail?: RequirementDetail;
  /** La regla del país todavía se está buscando. */
  searching?: boolean;
  confirmable: boolean;
  confirmed?: boolean;
  missingData?: boolean;
  sources?: Citation[];
  confirmedByName?: string;
  confirmedAt?: string;
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

/**
 * Lo que pide un país a un pasaporte argentino, mientras el viaje no tiene
 * pasajeros. No dice cumple ni falta: no hay documento contra el cual medir.
 */
export interface PreviewLine {
  country: string;
  role: "destino" | "escala";
  key: string;
  text: string;
  /** No se sabe: hay que verificarlo. */
  verify?: boolean;
  /** La regla todavía se está buscando. */
  searching?: boolean;
  sources?: Citation[];
  /** Lo que suman las páginas oficiales: la regla explicada y el trámite. */
  detail?: RequirementDetail;
}

export interface TripDetail extends TripSummary {
  customer: Customer;
  /** De dónde sale el viaje. No se evalúa como entrada. */
  originCountry: string;
  itinerary: Stop[];
  requirements: Requirement[];
  /** Solo sin pasajeros: lo que piden los países a un pasaporte argentino. */
  preview?: PreviewLine[];
  passengers: Passenger[];
  payment: Payment | null;
  controls: Control[];
}

export interface InboxItem { contactId: string; trip?: TripSummary }

/** Un usuario al que se le puede asignar un viaje. */
export interface Seller { id: string; name: string }

/** Lo que la ficha puede cambiar del viaje. Un vendedor vacío lo desasigna. */
export type TripUpdate = Partial<Pick<TripSummary, "destination" | "departAt" | "returnAt" | "status" | "sellerUserId">>;

export type CustomerUpdate = Partial<Pick<Customer, "name" | "email" | "phoneNumber" | "documentType" | "documentNumber" | "customerSince">>;

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
  customerId?: string;
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

export interface TripPage {
  items: TripRow[];
  total: number;
  /** Cuántos viajes hay en cada estado con los demás filtros aplicados: los números de los chips. */
  byStatus?: Record<string, number>;
  nextCursor?: string;
}

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

/** Un contacto de WhatsApp del cliente, con su chat individual más reciente. */
export interface CustomerContact {
  contactId: string;
  name: string;
  phoneNumber?: string;
  conversationId?: string;
  instanceLabel?: string;
  lastMessageAt?: string;
}

/**
 * Una persona que viajó en algún viaje del cliente, una vez aunque haya viajado
 * en varios: la versión del viaje más reciente.
 */
export interface Companion {
  /** El pasajero de esa versión: es el que se copia al traerlo. */
  passengerId: string;
  firstName: string;
  lastName: string;
  kind: "adulto" | "menor";
  birthDate?: string;
  tripCount: number;
  tripId: string;
  tripNumber: number;
  destination: string;
  /** El documento con que viajó en esa versión. */
  document?: TravelDocument;
  permitCount: number;
  /** Al traer pasajeros: ya está en este viaje. */
  inTrip?: boolean;
  /** Al traer pasajeros: viene marcada, por ser del viaje anterior más reciente. */
  selected?: boolean;
}

/** La página del cliente. */
export interface CustomerDetail extends Customer {
  contacts: CustomerContact[];
  trips: TripRow[];
  companions: Companion[];
}

/** Un cliente en la lista de Clientes. */
export interface CustomerRow {
  id: string;
  name: string;
  customerSince: string;
  email?: string;
  phoneNumber?: string;
  /** El teléfono y el chat del contacto con la conversación individual más reciente. */
  contactPhone?: string;
  conversationId?: string;
  chatCount: number;
  tripCount: number;
  lastDestination?: string;
  /** El viaje abierto más reciente. */
  openTrip?: { id: string; number: number; status: TripStatus; destination: string; departAt?: string };
  lastActivity: string;
}

export type CustomerChip = "todos" | "con-viaje" | "sin-viajes" | "sin-whatsapp";
export type CustomerSort = "movimiento" | "nombre";

export interface CustomerPage {
  items: CustomerRow[];
  total: number;
  /** Cuántos clientes hay en cada chip con la búsqueda aplicada. */
  byChip?: Partial<Record<CustomerChip, number>>;
  nextCursor?: string;
}

export interface CustomerListParams { q?: string; filtro?: CustomerChip; orden?: CustomerSort; cursor?: string }

/** Un cliente que se parece al que se está cargando, o que podría ser el dueño de un chat. */
export interface CustomerMatch {
  id: string;
  name: string;
  phoneNumber?: string;
  email?: string;
  tripCount: number;
  conversationId?: string;
}

/** Un chat que todavía no es de ningún cliente. */
export interface FreeContact {
  contactId: string;
  name: string;
  phoneNumber?: string;
  conversationId: string;
  instanceLabel?: string;
  lastMessageAt?: string;
}
