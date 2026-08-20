import type { Conversation, Line, Message } from "./api";

/**
 * Contenido de la demo comercial (/mirror?demo=1).
 *
 * Todo lo de acá se ve en la pantalla que se le muestra al cliente, así que se
 * escribe como si fuera un martes cualquiera de una agencia real: nombres,
 * destinos, montos y estados de venta. "Cliente 1" y "Ventas 1" no le permiten
 * a nadie imaginarse su propio negocio adentro del producto.
 */

/** Estado comercial que se pinta en la fila de la bandeja. */
export interface ConversationState {
  label: string;
  tone?: "warn" | "late";
}

/**
 * La ficha del contacto. El viaje no vive acá: vive en la operación, porque un
 * contacto puede tener varias a lo largo del tiempo.
 *
 * Sin etiquetas ni ciudad. Las tres etiquetas que había eran campos disfrazados
 * —recurrente se deduce del historial, el paquete es el producto y el pago en
 * pesos es el método— y la ciudad no la da WhatsApp ni la usa ningún paso del
 * proceso administrativo.
 */
export interface DemoContactProfile {
  kind: string;
  /** Medio proceso administrativo sale por mail: avisos, vouchers, no reembolsable. */
  email: string;
  since: string;
  note: { body: string[]; time: string };
  activity: Array<{ title: string; detail: string; time: string; mark?: boolean }>;
}

export const DEMO_LINES: Line[] = [
  { id: "line-nacional", name: "ventas-nacional", label: "Ventas Nacional", displayName: "Ventas Nacional", phoneNumber: "+54 9 11 5510-2201", status: "open", isActive: true, unread: 3 },
  { id: "line-exterior", name: "ventas-exterior", label: "Ventas Exterior", displayName: "Ventas Exterior", phoneNumber: "+54 9 11 5510-2202", status: "open", isActive: true, unread: 4 },
  { id: "line-corporativo", name: "corporativo", label: "Corporativo", displayName: "Corporativo", phoneNumber: "+54 9 11 5510-2203", status: "open", isActive: true, unread: 1 },
  { id: "line-postventa", name: "post-venta", label: "Post-venta", displayName: "Post-venta", phoneNumber: "+54 9 11 5510-2204", status: "connecting", isActive: true, unread: 0 },
];

interface DemoRow {
  name: string;
  phone: string;
  line: string;
  preview: string;
  at: string;
  unread?: number;
  group?: boolean;
  state?: ConversationState;
  mention?: boolean;
}

const ROWS: DemoRow[] = [
  {
    name: "Ana Pérez", phone: "+54 9 11 5512-8834", line: "line-nacional",
    preview: "Para los tres sale USD 1.840. Incluye vuelos, hotel, traslados y Cataratas.",
    at: "2026-08-11T11:43:00-03:00",
    state: { label: "Cotización enviada" },
  },
  {
    name: "Grupo Bariloche", phone: "+54 9 11 5540-1190", line: "line-nacional", group: true,
    preview: "Lucas: Somos 8, ¿vamos todos en el mismo micro?",
    at: "2026-08-11T11:31:00-03:00", unread: 1, mention: true,
    state: { label: "8 pasajeros" },
  },
  {
    name: "Martín Gómez", phone: "+54 9 11 3388-0472", line: "line-exterior",
    preview: "Te mandé el comprobante",
    at: "2026-08-11T10:52:00-03:00", unread: 1,
    state: { label: "Seña a confirmar", tone: "warn" },
  },
  {
    name: "Laura Díaz", phone: "+54 9 341 214-7765", line: "line-exterior",
    preview: "¿Puedo pagar en 6 cuotas?",
    at: "2026-08-11T09:47:00-03:00", unread: 3,
    state: { label: "Sin responder 4 h", tone: "late" },
  },
  {
    name: "Empresa Rosario", phone: "+54 341 425-9100", line: "line-corporativo",
    preview: "Hola, necesitamos 12 pasajes a Santiago para octubre",
    at: "2026-08-11T09:28:00-03:00", mention: true,
    state: { label: "Cotización pedida" },
  },
  {
    name: "Familia García", phone: "+54 9 261 550-3318", line: "line-postventa",
    preview: "Llegamos bien, el hotel muy lindo. Gracias!",
    at: "2026-08-11T08:44:00-03:00",
    state: { label: "Viaje en curso" },
  },
  {
    name: "Pablo Rodríguez", phone: "+54 9 11 6072-4415", line: "line-nacional",
    preview: "Ahí van los DNI que faltaban",
    at: "2026-08-11T08:12:00-03:00",
    state: { label: "Falta documentación", tone: "warn" },
  },
  {
    name: "Carla López", phone: "+54 9 223 411-8890", line: "line-exterior",
    preview: "Dale, espero el voucher",
    at: "2026-08-10T19:05:00-03:00",
    state: { label: "Sale en 9 días", tone: "warn" },
  },
  {
    name: "Diego Martínez", phone: "+54 9 11 4477-2093", line: "line-exterior",
    preview: "Por ahora se nos va de presupuesto, gracias",
    at: "2026-08-10T17:40:00-03:00",
    state: { label: "Cerrada" },
  },
  {
    name: "Julieta Fernández", phone: "+54 9 351 660-2288", line: "line-nacional",
    preview: "Hola, quería ver opciones para octubre",
    at: "2026-08-10T16:22:00-03:00",
    state: { label: "Cotizada hace 2 días", tone: "late" },
  },
];

const lineLabel = (id: string) => DEMO_LINES.find((line) => line.id === id)?.displayName ?? "";

export const DEMO_CONVERSATIONS: Conversation[] = ROWS.map((row, index) => ({
  id: `demo-${index + 1}`,
  instanceId: row.line,
  instanceLabel: lineLabel(row.line),
  contactId: `contact-${index + 1}`,
  contactName: row.name,
  phoneNumber: row.phone,
  isGroup: Boolean(row.group),
  lastMessageAt: row.at,
  preview: row.preview,
  lastType: "text",
  unread: row.unread ?? 0,
}));

export const DEMO_STATES: Record<string, ConversationState> = Object.fromEntries(
  ROWS.map((row, index) => [`demo-${index + 1}`, row.state ?? { label: "" }]).filter(
    ([, state]) => Boolean((state as ConversationState).label),
  ),
);

export const MENTIONS = new Set(ROWS.map((row, index) => (row.mention ? `demo-${index + 1}` : "")).filter(Boolean));
export const UNANSWERED = new Set(
  ROWS.map((row, index) => ((row.unread ?? 0) > 0 ? `demo-${index + 1}` : "")).filter(Boolean),
);

/* Fichas */

export const DEMO_PROFILES: Record<string, DemoContactProfile> = {
  "demo-1": {
    kind: "Contacto de WhatsApp",
    email: "ana.perez@gmail.com",
    since: "mar 2024",
    note: {
      body: [
        "Ya viajó con nosotros en 2024.",
        "Viaja con una nena de 9 años.",
      ],
      time: "hoy 11:32",
    },
    activity: [
      { title: "Cotización enviada", detail: "Iguazú 12–15 sep · USD 1.840", time: "hoy 11:43", mark: true },
      { title: "Asignada a Ventas Nacional", detail: "Por regla de reparto de la línea", time: "hoy 11:39" },
      { title: "Conversación abierta", detail: "Primer mensaje entrante", time: "hoy 11:38" },
      { title: "Viaje cerrado", detail: "Salta 4 noches · USD 1.120", time: "18 jul 2024" },
    ],
  },
  "demo-2": {
    kind: "Grupo de WhatsApp · 8 participantes",
    email: "lucas.garcia@outlook.com",
    since: "jun 2026",
    note: {
      body: ["Lucas organiza el viaje del grupo."],
      time: "ayer 18:10",
    },
    activity: [
      { title: "Cotización enviada", detail: "Bariloche 3–8 oct · 8 pax", time: "ayer 17:55", mark: true },
      { title: "Grupo creado", detail: "Lo abrió el contacto", time: "6 ago" },
    ],
  },
  "demo-3": {
    kind: "Contacto de WhatsApp",
    email: "martin.gomez@gmail.com",
    since: "ago 2026",
    note: {
      body: ["Mandó el comprobante. Falta revisar el pago."],
      time: "hoy 10:55",
    },
    activity: [
      { title: "Comprobante recibido", detail: "Pendiente de conciliar", time: "hoy 10:52", mark: true },
      { title: "Cotización enviada", detail: "Cancún 20–27 nov · USD 3.400", time: "8 ago" },
    ],
  },
};

/* Hilos */

const ana: Message[] = [
  { id: "m1", direction: "inbound", origin: "contact", type: "text", status: "read", body: "Hola! Quería consultar por el viaje a Iguazú de septiembre. ¿Hay lugar?", timestamp: "2026-08-11T11:38:00-03:00" },
  { id: "m2", direction: "outbound", origin: "panel", type: "text", status: "read", body: "Hola Ana, sí, queda lugar del 12 al 15. ¿Cuántos viajarían?", timestamp: "2026-08-11T11:39:00-03:00", sentByName: "Ventas Nacional" },
  { id: "m3", direction: "inbound", origin: "contact", type: "text", status: "read", body: "Somos 2 adultos y una nena de 9.", timestamp: "2026-08-11T11:41:00-03:00" },
  { id: "m4", direction: "outbound", origin: "panel", type: "text", status: "read", body: "Para los tres sale USD 1.840. Incluye vuelos, 3 noches con desayuno, traslados y Cataratas del lado argentino.", timestamp: "2026-08-11T11:43:00-03:00", sentByName: "Ventas Nacional" },
];

const grupoBariloche: Message[] = [
  { id: "g1", direction: "inbound", origin: "contact", type: "text", status: "read", body: "Hola, ¿qué pasó con lo de Bariloche de octubre?", timestamp: "2026-08-11T11:20:00-03:00", senderName: "Lucas" },
  { id: "g2", direction: "outbound", origin: "panel", type: "text", status: "read", body: "Hola! Tengo opción del 3 al 8. Sale USD 620 por persona con media pensión.", timestamp: "2026-08-11T11:26:00-03:00", sentByName: "Ventas Nacional" },
  { id: "g3", direction: "inbound", origin: "contact", type: "text", status: "read", body: "Somos 8, ¿vamos todos en el mismo micro?", timestamp: "2026-08-11T11:31:00-03:00", senderName: "Lucas" },
];

const generic = (conversation: Conversation, preview: string): Message[] => [
  { id: `${conversation.id}-1`, direction: "inbound", origin: "contact", type: "text", status: "read", body: preview, timestamp: conversation.lastMessageAt ?? "2026-08-11T10:00:00-03:00", senderName: conversation.isGroup ? "Lucas" : undefined },
  { id: `${conversation.id}-2`, direction: "outbound", origin: "panel", type: "text", status: "delivered", body: "Dale, lo reviso y te digo.", timestamp: "2026-08-11T11:52:00-03:00", sentByName: conversation.instanceLabel },
];

export const DEMO_MESSAGES: Record<string, Message[]> = Object.fromEntries(
  DEMO_CONVERSATIONS.map((conversation, index) => [
    conversation.id,
    conversation.id === "demo-1" ? ana
      : conversation.id === "demo-2" ? grupoBariloche
      : generic(conversation, ROWS[index].preview),
  ]),
);

export function demoProfile(conversation: Conversation): DemoContactProfile {
  const existing = DEMO_PROFILES[conversation.id];
  if (existing) return existing;

  return {
    kind: conversation.isGroup ? "Grupo de WhatsApp" : "Contacto de WhatsApp",
    email: "",
    since: "ago 2026",
    note: { body: ["Todavía sin notas para este contacto."], time: "" },
    activity: [
      { title: "Conversación abierta", detail: `Ingresó por ${conversation.instanceLabel}`, time: "hoy", mark: true },
    ],
  };
}
