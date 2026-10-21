"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { mirrorApi, type Message, type MessageQuote } from "./api";

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
  // De qué conversación son los mensajes cargados: al cambiar de chat, los del
  // anterior siguen en pantalla hasta que llega la página nueva.
  const [loadedFor, setLoadedFor] = useState<string | null>(null);

  const queryRef = useRef(0);
  const earlyEchoes = useRef(new Map<string, Message>());

  useEffect(() => {
    earlyEchoes.current.clear();
    if (!conversationId) {
      setMessages([]);
      setCursor(undefined);
      setLoadedFor(null);
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
        setLoadedFor(conversationId);
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

  /**
   * Añade al final un mensaje llegado en vivo.
   *
   * `waMessageId` es la clave de reconciliación: si el mensaje que llega es el
   * eco de algo que enviamos nosotros, reemplaza a la burbuja provisional en vez
   * de duplicarla.
  */
  const append = useCallback((msg: Message, waMessageId?: string) => {
    if (waMessageId && msg.direction === "outbound" && msg.origin === "panel") {
      earlyEchoes.current.set(waMessageId, msg);
      if (earlyEchoes.current.size > 100) {
        const oldest = earlyEchoes.current.keys().next().value;
        if (oldest) earlyEchoes.current.delete(oldest);
      }
    }
    setMessages((prev) => {
      if (prev.some((m) => m.id === msg.id)) return prev; // ya estaba
      if (waMessageId) {
        const i = prev.findIndex((m) => m.pending?.waMessageId === waMessageId);
        if (i >= 0) {
          earlyEchoes.current.delete(waMessageId);
          const next = [...prev];
          next[i] = msg;
          return next;
        }
      }
      return [...prev, msg];
    });
  }, []);

  /** Pinta la burbuja provisional apenas se aprieta enviar. */
  const addOptimistic = useCallback(
    (key: string, body: string, fileName?: string, quote?: MessageQuote) => {
      setMessages((prev) => [
        ...prev,
        {
          id: `pendiente:${key}`,
          direction: "outbound",
          origin: "panel",
          type: fileName ? "document" : "text",
          status: "pending",
          body,
          quotedId: quote ? "pending" : undefined,
          quote,
          timestamp: new Date().toISOString(),
          pending: { key, fileName },
        },
      ]);
    },
    [],
  );

  /**
   * Resuelve una burbuja provisional.
   *
   * Con `waMessageId` la deja esperando el eco, que llegará por websocket y la
   * reemplazará por el mensaje real. Con error, la marca en rojo para reintentar.
   *
   * No se sustituye acá directamente por el mensaje real porque todavía no lo
   * tenemos: la fila en `messages` la crea el webhook uno o dos segundos después.
   */
  const settleOptimistic = useCallback(
    (key: string, waMessageId: string | null, error?: string) => {
      const earlyEcho = waMessageId ? earlyEchoes.current.get(waMessageId) : undefined;
      if (waMessageId && earlyEcho) earlyEchoes.current.delete(waMessageId);
      setMessages((prev) => {
        if (earlyEcho) {
          const withoutOptimistic = prev.filter((m) => m.pending?.key !== key);
          return withoutOptimistic.some((m) => m.id === earlyEcho.id)
            ? withoutOptimistic
            : [...withoutOptimistic, earlyEcho];
        }
        return prev.map((m) =>
          m.pending?.key === key
            ? {
                ...m,
                status: error ? "failed" : "sent",
                pending: { ...m.pending, waMessageId: waMessageId ?? undefined, error },
            }
            : m,
        );
      });
    },
    [],
  );

  return {
    messages,
    loadedFor,
    loading,
    error,
    hasOlder: Boolean(cursor),
    loadOlder,
    append,
    addOptimistic,
    settleOptimistic,
  };
}
