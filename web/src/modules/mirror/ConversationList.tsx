"use client";

import { useEffect, useRef } from "react";
import type { useInbox } from "./useInbox";

/** Etiqueta corta para los mensajes que no son texto. */
const TYPE_LABEL: Record<string, string> = {
  image: "📷 Foto",
  video: "🎥 Vídeo",
  audio: "🎤 Audio",
  document: "📎 Documento",
  sticker: "Sticker",
  location: "📍 Ubicación",
  contact: "👤 Contacto",
};

export function ConversationList({
  inbox,
  selectedId,
  onSelect,
}: {
  inbox: ReturnType<typeof useInbox>;
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  const sentinel = useRef<HTMLDivElement>(null);

  // Scroll infinito por observador y no por evento de scroll: no dispara en cada
  // píxel y no hace falta rebotarlo a mano.
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !inbox.hasMore) return;

    const observer = new IntersectionObserver(
      ([entry]) => entry.isIntersecting && void inbox.loadMore(),
      { rootMargin: "200px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [inbox.hasMore, inbox.loadMore]);

  if (inbox.error) {
    return (
      <div className="conversations">
        <p className="error">{inbox.error}</p>
        <button type="button" onClick={() => void inbox.reload()}>
          Reintentar
        </button>
      </div>
    );
  }

  if (inbox.loading && inbox.items.length === 0) {
    return <p className="empty">Cargando conversaciones…</p>;
  }

  if (inbox.items.length === 0) {
    return <p className="empty">No hay conversaciones todavía.</p>;
  }

  return (
    <ul className="conversations">
      {inbox.items.map((c) => (
        <li key={c.id}>
          <button
            type="button"
            className={c.id === selectedId ? "conv active" : "conv"}
            onClick={() => onSelect(c.id)}
          >
            <span className="conv-top">
              <strong>{c.contactName}</strong>
              {c.lastMessageAt && (
                <time dateTime={c.lastMessageAt}>{shortTime(c.lastMessageAt)}</time>
              )}
            </span>
            <span className="conv-bottom">
              <span className="preview">
                {c.preview || TYPE_LABEL[c.lastType ?? ""] || "—"}
              </span>
              {c.unread > 0 && <span className="badge">{c.unread}</span>}
            </span>
            {/* La línea de origen siempre visible: en la bandeja unificada, saber
                por qué número entró el mensaje es la mitad de la información. */}
            <span className="line-tag">{c.instanceLabel}</span>
          </button>
        </li>
      ))}
      <div ref={sentinel} />
    </ul>
  );
}

/** Hora si es de hoy, fecha corta si no. Como cualquier bandeja. */
function shortTime(iso: string): string {
  const date = new Date(iso);
  const today = new Date();
  const sameDay =
    date.getDate() === today.getDate() &&
    date.getMonth() === today.getMonth() &&
    date.getFullYear() === today.getFullYear();

  return sameDay
    ? date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleDateString([], { day: "2-digit", month: "2-digit" });
}
