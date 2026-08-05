"use client";

/**
 * Pantalla del módulo espejo: todas las líneas de la agencia en una bandeja.
 *
 * El estado que importa (línea seleccionada, conversación abierta, búsqueda) vive
 * en la URL, no en useState. Así una pestaña sobrevive a la recarga, se puede
 * dejar fija en una línea y el enlace a una conversación se le puede pasar a otra
 * persona.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { useSocket, type ServerEvent } from "../../kernel/api/useSocket";
import { mirrorApi, type Conversation, type Line } from "./api";
import { ConversationList } from "./ConversationList";
import { LineFilter } from "./LineFilter";
import { SearchResults } from "./SearchResults";
import { Thread } from "./Thread";
import { useInbox } from "./useInbox";
import { mirror } from "./module";

interface NewMessageEvent {
  conversationId: string;
  instanceId: string;
  body?: string;
  type: string;
  timestamp: string;
}

export function MirrorScreen() {
  const router = useRouter();
  const params = useSearchParams();

  const instance = params.get("instance") ?? "";
  const openId = params.get("c") ?? "";
  const focusId = params.get("m") ?? undefined;
  const query = params.get("q") ?? "";

  const [lines, setLines] = useState<Line[]>([]);
  const [open, setOpen] = useState<Conversation | null>(null);
  const [contactFilter, setContactFilter] = useState("");

  const inbox = useInbox(instance, contactFilter);

  /** Escribe en la URL sin apilar historial: esto es filtrar, no navegar. */
  const setParams = useCallback(
    (patch: Record<string, string | undefined>) => {
      const next = new URLSearchParams(params.toString());
      for (const [key, value] of Object.entries(patch)) {
        if (value) next.set(key, value);
        else next.delete(key);
      }
      router.replace(`${mirror.path}?${next.toString()}`, { scroll: false });
    },
    [params, router],
  );

  const loadLines = useCallback(() => {
    mirrorApi
      .lines()
      .then((res) => setLines(res.lines))
      .catch(() => setLines([]));
  }, []);

  useEffect(loadLines, [loadLines]);

  // La conversación abierta se resuelve desde la URL y no desde el clic: así un
  // enlace pegado en el navegador abre el hilo correcto.
  useEffect(() => {
    if (!openId) {
      setOpen(null);
      return;
    }
    let vigente = true;
    mirrorApi
      .conversation(openId)
      .then((conv) => {
        if (!vigente) return;
        setOpen(conv);
        // Abrir marca revisado. El contador es global: deja de figurar como
        // pendiente para todo el equipo.
        if (conv.unread > 0) {
          void mirrorApi.markRead(conv.id).then(() => {
            inbox.applyRead(conv.id);
            loadLines();
          });
        }
      })
      .catch(() => {
        if (vigente) setOpen(null);
      });
    return () => {
      vigente = false;
    };
    // inbox.applyRead es estable (useCallback sin deps cambiantes); incluirlo
    // volvería a disparar el efecto en cada render de la bandeja.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openId, loadLines]);

  // Suscripción estrechada a la línea filtrada: mirando un número, el tráfico de
  // los demás no viaja por la red.
  const subscribed = useMemo(() => (instance ? [instance] : []), [instance]);

  const linesDirty = useRef(false);

  useSocket(
    mirror.name,
    (ev: ServerEvent) => {
      if (ev.type === "message.new") {
        const p = ev.payload as NewMessageEvent;
        inbox.applyNewMessage(p.conversationId, p.body ?? "", p.type, p.timestamp);
        linesDirty.current = true;
      }
      if (ev.type === "conversation.read") {
        const p = ev.payload as { conversationId: string };
        inbox.applyRead(p.conversationId);
        linesDirty.current = true;
      }
      if (ev.type === "instance.status") linesDirty.current = true;
    },
    subscribed,
  );

  // Los contadores por línea se refrescan a lo sumo cada 5 s. Recargarlos en cada
  // mensaje convertiría una ráfaga de WhatsApp en una ráfaga de consultas.
  useEffect(() => {
    const timer = setInterval(() => {
      if (!linesDirty.current) return;
      linesDirty.current = false;
      loadLines();
    }, 5000);
    return () => clearInterval(timer);
  }, [loadLines]);

  return (
    <section className="mirror">
      <header className="page-header">
        <h1>WhatsApp</h1>
        <p>{mirror.description}</p>
      </header>

      <LineFilter
        lines={lines}
        selected={instance}
        onSelect={(id) => setParams({ instance: id || undefined, c: undefined, m: undefined })}
      />

      <div className="mirror-body">
        {/* No es `.sidebar`: esa clase la usa el shell para el menú de módulos. */}
        <aside className="inbox-pane">
          <div className="searches">
            <input
              type="search"
              placeholder="Buscar contacto…"
              value={contactFilter}
              onChange={(e) => setContactFilter(e.target.value)}
            />
            <input
              type="search"
              placeholder="Buscar en los mensajes…"
              defaultValue={query}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  setParams({ q: e.currentTarget.value || undefined, c: undefined, m: undefined });
                }
              }}
            />
          </div>

          {query ? (
            <SearchResults
              query={query}
              instance={instance}
              onOpen={(hit) => setParams({ c: hit.conversationId, m: hit.messageId })}
              onClear={() => setParams({ q: undefined, m: undefined })}
            />
          ) : (
            <ConversationList
              inbox={inbox}
              selectedId={openId}
              onSelect={(id) => setParams({ c: id, m: undefined })}
            />
          )}
        </aside>

        <Thread conversation={open} focusId={focusId} />
      </div>
    </section>
  );
}
