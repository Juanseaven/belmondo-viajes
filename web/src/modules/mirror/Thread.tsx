"use client";

import { useEffect, useRef } from "react";
import type { Conversation } from "./api";
import { MessageBubble } from "./MessageBubble";
import { useThread } from "./useThread";

/**
 * El hilo de una conversación, con scroll invertido: se abre abajo y se pagina
 * hacia arriba.
 */
export function Thread({
  conversation,
  focusId,
}: {
  conversation: Conversation | null;
  focusId?: string;
}) {
  const { messages, loading, error, hasOlder, loadOlder } = useThread(
    conversation?.id ?? null,
    focusId,
  );

  const scroller = useRef<HTMLDivElement>(null);
  const topSentinel = useRef<HTMLDivElement>(null);
  const anchorRef = useRef<HTMLDivElement>(null);

  // Al pedir mensajes anteriores, el navegador mantiene el scrollTop mientras el
  // contenido crece por arriba: el hilo salta. Se corrige guardando la altura
  // antes de cargar y restaurando la diferencia después.
  const prevHeight = useRef(0);

  useEffect(() => {
    const node = topSentinel.current;
    const box = scroller.current;
    if (!node || !box || !hasOlder) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry.isIntersecting) return;
        prevHeight.current = box.scrollHeight;
        void loadOlder();
      },
      { root: box, rootMargin: "300px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasOlder, loadOlder]);

  useEffect(() => {
    const box = scroller.current;
    if (!box) return;

    if (prevHeight.current > 0) {
      box.scrollTop = box.scrollHeight - prevHeight.current;
      prevHeight.current = 0;
      return;
    }
    // En modo búsqueda se centra el mensaje buscado; si no, al final del chat.
    if (focusId && anchorRef.current) {
      anchorRef.current.scrollIntoView({ block: "center" });
    } else {
      box.scrollTop = box.scrollHeight;
    }
  }, [messages, focusId]);

  if (!conversation) {
    return (
      <div className="thread empty">
        <p>Elegí una conversación para verla.</p>
      </div>
    );
  }

  return (
    <div className="thread">
      <header className="thread-header">
        <div>
          <h2>{conversation.contactName}</h2>
          <span className="line-tag">{conversation.instanceLabel}</span>
        </div>
        {conversation.phoneNumber && <span className="phone">{conversation.phoneNumber}</span>}
      </header>

      <div className="messages" ref={scroller}>
        <div ref={topSentinel} />
        {hasOlder && <p className="loading-older">Cargando anteriores…</p>}

        {loading && messages.length === 0 && <p className="empty">Cargando…</p>}
        {error && <p className="error">{error}</p>}
        {!loading && !error && messages.length === 0 && (
          <p className="empty">Esta conversación no tiene mensajes.</p>
        )}

        {messages.map((m) => (
          <div key={m.id} ref={m.id === focusId ? anchorRef : undefined}
               className={m.id === focusId ? "highlight" : undefined}>
            <MessageBubble message={m} />
          </div>
        ))}
      </div>
    </div>
  );
}
