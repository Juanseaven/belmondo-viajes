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
import { ArrowLeft, Info, Plus, Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { useSession } from "../../kernel/auth/session";
import { useSocket, type ServerEvent } from "../../kernel/api/useSocket";
import { mirrorApi, type Conversation, type Line, type Message } from "./api";
import { ConversationList } from "./ConversationList";
import { LineDialog } from "./LineDialog";
import { LineFilter } from "./LineFilter";
import { SearchResults } from "./SearchResults";
import { Thread } from "./Thread";
import { useInbox } from "./useInbox";
import type { useThread } from "./useThread";
import { mirror } from "./module";
import { operacionesApi } from "../operaciones/api";
import { TripBanner } from "../operaciones/TripBanner";
import { TripSidebar } from "../operaciones/TripSidebar";
import type { Control } from "../operaciones/types";
import { useTrips } from "../operaciones/useTrips";

/** Lo que el hilo abierto expone hacia arriba para recibir eventos en vivo. */
type ThreadHandle = ReturnType<typeof useThread>;
type InboxTab = "all" | "unread";

interface NewMessageEvent {
  messageId: string;
  conversationId: string;
  instanceId: string;
  /** Clave para emparejar el eco con la burbuja provisional que pintó el envío. */
  waMessageId?: string;
  body?: string;
  type: string;
  direction: Message["direction"];
  origin: Message["origin"];
  timestamp: string;
  quotedId?: string;
  senderJid?: string;
  senderName?: string;
  senderPhone?: string;
}

export function MirrorScreen() {
  const params = useSearchParams();
  return <LiveMirrorScreen params={params} />;
}

function LiveMirrorScreen({ params }: { params: ReturnType<typeof useSearchParams> }) {
  const router = useRouter();
  const session = useSession();

  const instance = params.get("instance") ?? "";
  const openId = params.get("c") ?? "";
  const focusId = params.get("m") ?? undefined;
  const query = params.get("q") ?? "";
  // El viaje con el que se llegó desde la pantalla de Viajes.
  const preferredTripId = params.get("viaje") ?? undefined;

  const [lines, setLines] = useState<Line[]>([]);
  const [open, setOpen] = useState<Conversation | null>(null);
  const [searchValue, setSearchValue] = useState(query);
  const [altaAbierta, setAltaAbierta] = useState(false);
  const [lineaParaVincular, setLineaParaVincular] = useState<Line | null>(null);
  const [ultimoQR, setUltimoQR] = useState<{ instance: string; qr: string } | null>(null);
  const [tab, setTab] = useState<InboxTab>("all");
  const [detailsOpen, setDetailsOpen] = useState(true);
  const [actionDraft, setActionDraft] = useState<{ controlId: string; body: string; waMessageId?: string }>();

  const inbox = useInbox(instance, query ? "" : searchValue, tab === "unread");
  const unreadChats = instance
    ? lines.find((line) => line.id === instance)?.unread ?? 0
    : lines.reduce((total, line) => total + line.unread, 0);
  const contactIds = useMemo(() => inbox.items.map((item) => item.contactId), [inbox.items]);
  const trips = useTrips(contactIds, open?.contactId, preferredTripId);
  const runControl = (control: Control) => setActionDraft({ controlId: control.id, body: control.message || "" });

  useEffect(() => {
    setSearchValue(query);
  }, [query]);

  useEffect(() => { setActionDraft(undefined); }, [openId]);

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

  const disconnectLine = async (line: Line) => {
    await mirrorApi.deleteLine(line.id);
    setLines((current) => current.filter((item) => item.id !== line.id));

    const deletedConversationWasOpen = open?.instanceId === line.id;
    if (instance === line.id || deletedConversationWasOpen) {
      setOpen(null);
      setParams({
        instance: instance === line.id ? undefined : instance || undefined,
        c: undefined,
        m: undefined,
      });
    }
    await inbox.reload();
  };

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
  const qrPrompted = useRef(new Set<string>());

  // El hilo abierto expone sus acciones acá para que el websocket pueda
  // inyectarle el mensaje que acaba de llegar sin recargarlo entero.
  const threadApi = useRef<ThreadHandle | null>(null);
  const recentMessageIDs = useRef(new Map<string, string>());

  useSocket(
    mirror.name,
    (ev: ServerEvent) => {
      if (ev.type === "message.new") {
        const p = ev.payload as NewMessageEvent;
        if (p.waMessageId) {
          recentMessageIDs.current.set(p.waMessageId, p.messageId);
          if (recentMessageIDs.current.size > 100) {
            const oldest = recentMessageIDs.current.keys().next().value;
            if (oldest) recentMessageIDs.current.delete(oldest);
          }
        }
        inbox.applyNewMessage(p.conversationId, p.body ?? "", p.type, p.timestamp, p.direction);
        linesDirty.current = true;

        // Si el mensaje es del hilo abierto, se agrega en vivo. El waMessageId
        // hace que el eco de lo que enviamos reemplace a su burbuja provisional
        // en vez de duplicarla.
        if (p.conversationId === openId) {
          const liveMessage: Message = {
            id: p.messageId,
            direction: p.direction,
            origin: p.origin,
            type: p.type,
            status: "sent",
            body: p.body,
            timestamp: p.timestamp,
            quotedId: p.quotedId,
            senderJid: p.senderJid,
            senderName: p.senderName,
            senderPhone: p.senderPhone,
          };
          if (p.quotedId) {
            mirrorApi.messages(openId, { around: p.messageId })
              .then((page) => {
                const enriched = page.items.find((message) => message.id === p.messageId);
                threadApi.current?.append(enriched ?? liveMessage, p.waMessageId);
              })
              .catch(() => threadApi.current?.append(liveMessage, p.waMessageId));
          } else {
            threadApi.current?.append(liveMessage, p.waMessageId);
          }
        }

        // El POST de envío devuelve una fila de outbound_messages. La evidencia,
        // en cambio, apunta a messages, que nace con el eco del webhook. Se
        // emparejan por waMessageId y recién entonces se cierra el control.
        if (actionDraft?.waMessageId && p.waMessageId === actionDraft.waMessageId) {
          const controlID = actionDraft.controlId;
          setActionDraft(undefined);
          void operacionesApi.closeControl(controlID, p.messageId).then(trips.reload);
        }
      }
      if (ev.type === "conversation.read") {
        const p = ev.payload as { conversationId: string };
        inbox.applyRead(p.conversationId);
        linesDirty.current = true;
      }
      if (ev.type === "line.qr") {
        const qrEvent = ev.payload as { instance: string; qr: string };
        setUltimoQR(qrEvent);
        const existing = lines.find((line) => line.name === qrEvent.instance && line.status !== "open");
        if (session.user?.role === "admin" && existing?.wasConnected && !qrPrompted.current.has(existing.name)) {
          qrPrompted.current.add(existing.name);
          setLineaParaVincular(existing);
        }
      }
      if (ev.type === "instance.status") {
        linesDirty.current = true;
        // Una línea que se vincula cierra el modal de alta.
        if ((ev.payload as { state?: string }).state === "open") {
          const connected = ev.payload as { instance?: string };
          setAltaAbierta(false);
          setLineaParaVincular(null);
          if (connected.instance) qrPrompted.current.delete(connected.instance);
          loadLines();
        }
      }
      if (ev.type === "history.sync") {
        const p = ev.payload as { instance?: string };
        if (!instance || p.instance === instance) inbox.scheduleReload();
        linesDirty.current = true;
      }
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
    <section className="mirror-workspace">
      <div className={`${open ? "mirror-body has-details" : "mirror-body"}${detailsOpen ? "" : " operation-collapsed"}`}>
        <aside className="inbox-pane">
          <div className="inbox-lines">
            <LineFilter
              lines={lines}
              selected={instance}
              onSelect={(id) => setParams({ instance: id || undefined, c: undefined, m: undefined })}
              canManage={session.user?.role === "admin"}
              onDisconnect={disconnectLine}
            />
            {session.user?.role === "admin" && (
              <Button
                type="button"
                variant="ghost"
                className="inbox-connect"
                onClick={() => { setLineaParaVincular(null); setAltaAbierta(true); }}
              >
                <Plus aria-hidden />Conectar línea
              </Button>
            )}
          </div>

          <div className="inbox-tabs" role="tablist" aria-label="Filtrar conversaciones">
            <button type="button" role="tab" aria-selected={tab === "all"} onClick={() => setTab("all")}>
              Todos
            </button>
            <button type="button" role="tab" aria-selected={tab === "unread"} onClick={() => setTab("unread")}>
              No leídos <span>{unreadChats}</span>
            </button>
          </div>

          <div className="inbox-search">
            <div>
              <Search aria-hidden />
              <Input
                type="search"
                aria-label="Buscar contacto o mensaje"
                placeholder="Buscar contacto o mensaje…"
                value={searchValue}
                onChange={(event) => {
                  setSearchValue(event.target.value);
                  if (query) setParams({ q: undefined, c: undefined, m: undefined });
                }}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    setParams({ q: searchValue || undefined, c: undefined, m: undefined });
                  }
                }}
              />
            </div>
          </div>

          {query ? (
            <SearchResults
              query={query}
              instance={instance}
              onOpen={(hit) => setParams({ c: hit.conversationId, m: hit.messageId, viaje: undefined })}
              onClear={() => {
                setSearchValue("");
                setParams({ q: undefined, m: undefined });
              }}
            />
          ) : (
            <ConversationList
              inbox={inbox}
              selectedId={openId}
              onSelect={(id) => setParams({ c: id, m: undefined, viaje: undefined })}
            />
          )}
        </aside>

        <div className="thread-pane">
          {open && (
            <>
              <Button
                className="mobile-back-trigger"
                variant="ghost"
                size="icon"
                aria-label="Volver a conversaciones"
                onClick={() => setParams({ c: undefined, m: undefined })}
              >
                <ArrowLeft aria-hidden />
              </Button>
              <Sheet>
                <SheetTrigger asChild>
                  <Button className="mobile-details-trigger" variant="outline" size="icon" aria-label="Ver detalles del contacto">
                    <Info aria-hidden />
                  </Button>
                </SheetTrigger>
                <SheetContent className="details-sheet">
                  <SheetHeader className="sr-only">
                    <SheetTitle>Detalles del contacto</SheetTitle>
                  </SheetHeader>
                  <TripSidebar contactId={open.contactId} contactName={open.contactName} trips={trips} onRunControl={runControl} />
                </SheetContent>
              </Sheet>
            </>
          )}
          <Thread
            conversation={open}
            focusId={focusId}
            lineStatus={lines.find((line) => line.id === open?.instanceId)?.status}
            detailsHidden={!detailsOpen}
            onShowDetails={() => setDetailsOpen(true)}
            threadRef={(thread) => { threadApi.current = thread; }}
            banner={trips.active ? <TripBanner trip={trips.active} /> : undefined}
            draft={actionDraft?.body}
            onMessageSent={(message) => {
              if (!actionDraft) return;
              const definitiveID = recentMessageIDs.current.get(message.waMessageId);
              if (definitiveID) {
                const controlID = actionDraft.controlId;
                setActionDraft(undefined);
                void operacionesApi.closeControl(controlID, definitiveID).then(trips.reload);
                return;
              }
              setActionDraft({ ...actionDraft, waMessageId: message.waMessageId });
            }}
          />
        </div>
        {open && (
          <TripSidebar
            contactId={open.contactId}
            contactName={open.contactName}
            trips={trips}
            collapsed={!detailsOpen}
            onClose={() => setDetailsOpen(false)}
            onRunControl={runControl}
          />
        )}
      </div>

      {(altaAbierta || lineaParaVincular) && (
        <LineDialog
          existingLine={lineaParaVincular ?? undefined}
          qrEvents={ultimoQR}
          onClose={() => { setAltaAbierta(false); setLineaParaVincular(null); }}
          onConnected={() => { setAltaAbierta(false); setLineaParaVincular(null); loadLines(); }}
        />
      )}
    </section>
  );
}
