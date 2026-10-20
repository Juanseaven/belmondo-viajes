/**
 * Cliente del módulo espejo. Espeja los tipos del backend (`store/read.go`).
 *
 * Todo lo paginado devuelve `Page<T>`: `nextCursor` vacío significa que no hay
 * más. El cursor es opaco a propósito — no se construye ni se interpreta acá.
 */

import { api } from "../../kernel/api/client";

export interface Page<T> {
  items: T[];
  nextCursor?: string;
}

export interface Line {
  id: string;
  name: string;
  label: string;
  displayName?: string;
  phoneNumber?: string;
  status: "created" | "connecting" | "open" | "close";
  isActive: boolean;
  wasConnected?: boolean;
  lastSeenAt?: string;
  unread: number;
}

export interface Conversation {
  id: string;
  instanceId: string;
  instanceLabel: string;
  contactId: string;
  contactName: string;
  phoneNumber?: string;
  avatarUrl?: string;
  isGroup: boolean;
  lastMessageAt?: string;
  preview?: string;
  lastType?: string;
  unread: number;
  needsReply?: boolean;
}

export interface MessageMedia {
  id: string;
  mimeType?: string;
  fileName?: string;
  sizeBytes?: number;
  durationSeconds?: number;
  width?: number;
  height?: number;
  /** El mensaje se muestra antes de que el archivo esté subido. */
  status: "pending" | "processing" | "stored" | "failed";
}

export interface MessageQuote {
  /** UUID interno del mensaje original; sirve para cargar una ventana `around`. */
  id: string;
  direction: "inbound" | "outbound";
  type: string;
  body?: string;
  senderName?: string;
}

export interface Message {
  id: string;
  direction: "inbound" | "outbound";
  /** contact = el cliente; device = el teléfono; panel = esta app. */
  origin: "contact" | "device" | "panel";
  type: string;
  status: string;
  body?: string;
  timestamp: string;
  sentById?: string;
  sentByName?: string;
  senderJid?: string;
  senderName?: string;
  senderPhone?: string;
  quotedId?: string;
  quote?: MessageQuote;
  editedAt?: string;
  deletedAt?: string;
  media?: MessageMedia;

  /**
   * Solo en los mensajes provisionales: los que el panel pinta al enviar, antes
   * de que llegue el eco de WhatsApp. Ausente en todo lo que viene del servidor.
   */
  pending?: {
    key: string;
    fileName?: string;
    waMessageId?: string;
    error?: string;
  };
}

export interface SearchHit {
  messageId: string;
  conversationId: string;
  instanceId: string;
  instanceLabel: string;
  contactName: string;
  snippet: string;
  timestamp: string;
}

export interface SignedMedia {
  url: string;
  mimeType: string;
  expiresIn: number;
}

export interface SendResult {
  id: string;
  idempotencyKey: string;
  /** Id de WhatsApp: la clave para emparejar con el eco del websocket. */
  waMessageId: string;
  status: "pending" | "sent" | "failed";
}

export interface NewLine {
  id: string;
  instance: string;
  status: string;
}

/** Descarta las claves vacías para no mandar `?instance=&q=` en cada petición. */
function qs(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") search.set(key, String(value));
  }
  const out = search.toString();
  return out ? `?${out}` : "";
}

export const mirrorApi = {
  lines: () => api<{ lines: Line[] }>("/api/mirror/lines"),

  conversations: (params: { instance?: string; q?: string; cursor?: string; status?: "unread" }) =>
    api<Page<Conversation>>(`/api/mirror/conversations${qs(params)}`),

  conversation: (id: string) => api<Conversation>(`/api/mirror/conversations/${id}`),

  /** `around` centra la ventana en un mensaje; lo usa el salto desde la búsqueda. */
  messages: (id: string, params: { before?: string; around?: string } = {}) =>
    api<Page<Message>>(`/api/mirror/conversations/${id}/messages${qs(params)}`),

  markRead: (id: string) =>
    api<void>(`/api/mirror/conversations/${id}/read`, { method: "POST" }),

  search: (params: { q: string; instance?: string; cursor?: string }) =>
    api<Page<SearchHit>>(`/api/mirror/search${qs(params)}`),

  /**
   * URL firmada de un adjunto. De vida corta: no se cachea más allá de la
   * pantalla, y quien la pide es la burbuja al entrar en viewport.
   */
  media: (id: string) => api<SignedMedia>(`/api/mirror/media/${id}`),

  /**
   * Envía un mensaje. La `idempotencyKey` la genera el cliente y se reusa en los
   * reintentos: es lo que impide que un doble clic mande el mensaje dos veces.
   *
   * Con adjunto va como multipart; sin él, como JSON.
   */
  send: (
    conversationId: string,
    payload: { body: string; idempotencyKey: string; file?: File; replyToMessageId?: string },
  ) => {
    const path = `/api/mirror/conversations/${conversationId}/messages`;
    if (!payload.file) {
      return api<SendResult>(path, {
        method: "POST",
        body: JSON.stringify({
          body: payload.body,
          idempotencyKey: payload.idempotencyKey,
          replyToMessageId: payload.replyToMessageId,
        }),
      });
    }

    const form = new FormData();
    form.set("body", payload.body);
    form.set("idempotencyKey", payload.idempotencyKey);
    if (payload.replyToMessageId) form.set("replyToMessageId", payload.replyToMessageId);
    form.set("file", payload.file);
    // Sin Content-Type a propósito: el navegador tiene que ponerlo él para
    // incluir el boundary del multipart.
    return api<SendResult>(path, { method: "POST", body: form, headers: {} });
  },

  createLine: (payload: { ownerLabel: string; displayName?: string }) =>
    api<NewLine>("/api/mirror/lines", { method: "POST", body: JSON.stringify(payload) }),

  deleteLine: (id: string) => api<void>(`/api/mirror/lines/${id}`, { method: "DELETE" }),

  lineQR: (id: string) => api<{ instance: string; qr: string }>(`/api/mirror/lines/${id}/qr`),
};
