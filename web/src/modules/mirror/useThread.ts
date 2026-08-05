"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { mirrorApi, type Message } from "./api";

/**
 * El hilo de una conversación.
 *
 * El backend devuelve del más reciente al más antiguo (así lo sirve el índice).
 * Acá se invierte una sola vez para pintarlo en orden de lectura, y la paginación
 * hacia atrás agrega POR DELANTE, que es como se comporta el scroll de un chat.
 *
 * `focusId` abre el hilo centrado en un mensaje concreto, que es lo que necesita
 * un resultado de búsqueda: sin eso, hacer clic en un resultado te deja al final
 * del chat.
 */
export function useThread(conversationId: string | null, focusId?: string) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const queryRef = useRef(0);

  useEffect(() => {
    if (!conversationId) {
      setMessages([]);
      setCursor(undefined);
      return;
    }

    const query = ++queryRef.current;
    setLoading(true);
    setError(null);

    mirrorApi
      .messages(conversationId, focusId ? { around: focusId } : {})
      .then((page) => {
        if (query !== queryRef.current) return;
        setMessages([...page.items].reverse());
        setCursor(page.nextCursor);
      })
      .catch((err: unknown) => {
        if (query !== queryRef.current) return;
        setError(err instanceof Error ? err.message : "No se pudo cargar el hilo");
      })
      .finally(() => {
        if (query === queryRef.current) setLoading(false);
      });
  }, [conversationId, focusId]);

  /** Carga los mensajes anteriores. Los antepone, sin tocar los ya pintados. */
  const loadOlder = useCallback(async () => {
    if (!conversationId || !cursor) return;
    const query = queryRef.current;
    try {
      const page = await mirrorApi.messages(conversationId, { before: cursor });
      if (query !== queryRef.current) return;
      setMessages((prev) => {
        const seen = new Set(prev.map((m) => m.id));
        const older = [...page.items].reverse().filter((m) => !seen.has(m.id));
        return [...older, ...prev];
      });
      setCursor(page.nextCursor);
    } catch {
      // Igual que en la bandeja: no se vacía lo cargado por un fallo al paginar.
    }
  }, [conversationId, cursor]);

  /** Añade al final un mensaje llegado en vivo, si es de este hilo. */
  const append = useCallback((msg: Message) => {
    setMessages((prev) =>
      // El dedupe importa: el mismo mensaje puede llegar por websocket y venir en
      // una recarga del hilo casi a la vez.
      prev.some((m) => m.id === msg.id) ? prev : [...prev, msg],
    );
  }, []);

  return { messages, loading, error, hasOlder: Boolean(cursor), loadOlder, append };
}
