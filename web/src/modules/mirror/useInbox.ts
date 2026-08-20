"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { mirrorApi, type Conversation } from "./api";

/**
 * La bandeja: página inicial, scroll infinito y reacción al tiempo real.
 *
 * Lo delicado acá es que la lista se reordena sola. Cada mensaje que entra mueve
 * una conversación al principio, así que hay dos casos distintos:
 *
 *   - Conversación ya visible: se parcha en memoria y se sube al principio. No
 *     hace falta ir al servidor.
 *   - Conversación desconocida: se recarga la primera página. El evento del
 *     websocket no trae el nombre del contacto ni la foto, y engordarlo para
 *     cubrir este caso acoplaría el sobre del socket a la forma de esta pantalla.
 *     Recargar es autocurativo y pasa una vez por chat nuevo.
 */
export function useInbox(instance: string, search: string, unanswered = false) {
  const [items, setItems] = useState<Conversation[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Identifica la consulta vigente. Una respuesta que llega tarde, después de
  // que el usuario cambió de filtro, se descarta comparando contra esto.
  const queryRef = useRef(0);

  const reload = useCallback(async () => {
    const query = ++queryRef.current;
    setLoading(true);
    setError(null);
    try {
      const page = await mirrorApi.conversations({
        instance,
        q: search,
        status: unanswered ? "unanswered" : undefined,
      });
      if (query !== queryRef.current) return;
      setItems(page.items);
      setCursor(page.nextCursor);
    } catch (err) {
      if (query !== queryRef.current) return;
      setError(err instanceof Error ? err.message : "No se pudo cargar la bandeja");
    } finally {
      if (query === queryRef.current) setLoading(false);
    }
  }, [instance, search, unanswered]);

  useEffect(() => {
    // Rebote en la búsqueda: sin esto cada tecla dispara una consulta con ILIKE.
    const delay = search ? 250 : 0;
    const timer = setTimeout(reload, delay);
    return () => clearTimeout(timer);
  }, [reload, search]);

  const loadMore = useCallback(async () => {
    if (!cursor) return;
    const query = queryRef.current;
    try {
      const page = await mirrorApi.conversations({
        instance,
        q: search,
        cursor,
        status: unanswered ? "unanswered" : undefined,
      });
      if (query !== queryRef.current) return;
      // Se filtran los ids ya presentes: si entró un mensaje mientras se pedía la
      // página, una conversación pudo saltar hacia arriba y aparecer dos veces.
      setItems((prev) => {
        const seen = new Set(prev.map((c) => c.id));
        return [...prev, ...page.items.filter((c) => !seen.has(c.id))];
      });
      setCursor(page.nextCursor);
    } catch {
      // Un fallo al paginar no debe vaciar lo ya cargado; el usuario reintenta
      // scrolleando.
    }
  }, [cursor, instance, search, unanswered]);

  /** Aplica un mensaje nuevo llegado por websocket. */
  const applyNewMessage = useCallback(
    (
      conversationId: string,
      preview: string,
      type: string,
      at: string,
      direction: "inbound" | "outbound",
    ) => {
      let known = false;
      setItems((prev) => {
        const found = prev.find((c) => c.id === conversationId);
        if (!found) return prev;
        known = true;
        if (unanswered && direction === "outbound") {
          return prev.filter((conversation) => conversation.id !== conversationId);
        }
        const updated: Conversation = {
          ...found,
          preview,
          lastType: type,
          lastMessageAt: at,
          unread: direction === "inbound" ? found.unread + 1 : found.unread,
          needsReply: direction === "inbound",
        };
        return [updated, ...prev.filter((c) => c.id !== conversationId)];
      });
      if (!known && (!unanswered || direction === "inbound")) void reload();
    },
    [reload, unanswered],
  );

  const applyRead = useCallback((conversationId: string) => {
    setItems((prev) =>
      prev.map((c) => (c.id === conversationId ? { ...c, unread: 0 } : c)),
    );
  }, []);

  return { items, loading, error, hasMore: Boolean(cursor), loadMore, reload, applyNewMessage, applyRead };
}
