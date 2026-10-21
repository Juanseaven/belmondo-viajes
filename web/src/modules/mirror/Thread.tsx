"use client";

import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { MessageCircleMore, PanelRight, Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { displayName, formatPhone, isPhoneLike } from "@/lib/phone";
import type { Conversation, Message, MessageQuote, SendResult } from "./api";
import { Composer } from "./Composer";
import { ContactAvatar } from "./ContactAvatar";
import { MessageBubble } from "./MessageBubble";
import { useThread } from "./useThread";

/** Dónde va el separador «N mensajes sin leer»: se fija al abrir el chat. */
type UnreadMark = { conversationId: string; messageId: string; count: number };

export function Thread({
  conversation,
  focusId,
  lineStatus,
  detailsOpen,
  onToggleDetails,
  onSearch,
  threadRef,
  banner,
  draft,
  onMessageSent,
}: {
  conversation: Conversation | null;
  focusId?: string;
  lineStatus?: string;
  /** Con viaje al lado: el botón de la cabecera lo muestra u oculta. */
  detailsOpen?: boolean;
  onToggleDetails?: () => void;
  onSearch?: () => void;
  threadRef?: (thread: ReturnType<typeof useThread>) => void;
  banner?: ReactNode;
  draft?: string;
  onMessageSent?: (message: SendResult) => void;
}) {
  const [replyingTo, setReplyingTo] = useState<MessageQuote>();
  const [aroundId, setAroundId] = useState(focusId);
  const [highlightedId, setHighlightedId] = useState(focusId);
  const [unreadMark, setUnreadMark] = useState<UnreadMark>();
  const thread = useThread(conversation?.id ?? null, aroundId);
  const { messages, loading, error, hasOlder, loadOlder } = thread;

  threadRef?.(thread);

  const scroller = useRef<HTMLDivElement>(null);
  const topSentinel = useRef<HTMLDivElement>(null);
  const messageRefs = useRef(new Map<string, HTMLDivElement>());
  const prevHeight = useRef(0);

  useEffect(() => {
    setReplyingTo(undefined);
    setAroundId(focusId);
    setHighlightedId(focusId);
  }, [conversation?.id, focusId]);

  // El separador se calcula una vez, con los no leídos que tenía el chat al
  // abrirlo: contando hacia atrás los mensajes recibidos. Lo que llega después
  // no lo mueve.
  useEffect(() => {
    if (!conversation || thread.loadedFor !== conversation.id) return;
    if (unreadMark?.conversationId === conversation.id) return;
    let messageId = "";
    if (conversation.unread > 0) {
      let seen = 0;
      for (let i = messages.length - 1; i >= 0 && seen < conversation.unread; i--) {
        if (messages[i].direction === "inbound") {
          seen++;
          messageId = messages[i].id;
        }
      }
    }
    setUnreadMark({ conversationId: conversation.id, messageId, count: conversation.unread });
  }, [conversation, messages, thread.loadedFor, unreadMark]);
  const unreadAt = unreadMark?.conversationId === conversation?.id ? unreadMark : undefined;

  // Como WhatsApp: al abrir un chat con mensajes sin leer, la vista arranca en
  // el separador y no al final.
  const unreadDivider = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (unreadAt?.messageId) unreadDivider.current?.scrollIntoView?.({ block: "center" });
  }, [unreadAt?.messageId]);

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
    const anchor = highlightedId ? messageRefs.current.get(highlightedId) : undefined;
    if (anchor) {
      anchor.scrollIntoView({ block: "center", behavior: "smooth" });
    } else {
      box.scrollTop = box.scrollHeight;
    }
  }, [messages, highlightedId]);

  const selectReply = (message: Message) => {
    setReplyingTo({
      id: message.id,
      direction: message.direction,
      type: message.type,
      body: message.deletedAt ? undefined : message.body,
      senderName: message.direction === "outbound"
        ? message.sentByName || "Vos"
        : conversation?.isGroup
          ? message.senderName || message.senderPhone || message.senderJid || "Contacto"
          : conversation?.contactName || "Contacto",
    });
  };

  const openQuote = (messageId: string) => {
    setHighlightedId(messageId);
    if (!messages.some((message) => message.id === messageId)) {
      setAroundId(messageId);
    }
  };

  if (!conversation) {
    return (
      <div className="thread-empty">
        <span><MessageCircleMore aria-hidden /></span>
        <strong>Elegí una conversación</strong>
        <p>Seleccioná un contacto de la bandeja para ver el historial y responder.</p>
      </div>
    );
  }

  const phone = conversation.phoneNumber && !isPhoneLike(conversation.contactName)
    ? formatPhone(conversation.phoneNumber)
    : undefined;

  return (
    <div className="thread">
      <header className="thread-header">
        <ContactAvatar name={conversation.contactName} src={conversation.avatarUrl} className="avatar-40" />
        <div className="thread-contact">
          <h2>{displayName(conversation.contactName)}</h2>
          <span>
            {phone && <>{phone}<i aria-hidden>·</i></>}
            <span className={`line-status-dot ${lineStatus ?? "open"}`} aria-hidden />
            Línea {conversation.instanceLabel}
          </span>
        </div>
        {onSearch && (
          <Button type="button" variant="ghost" size="icon" className="icon-button" onClick={onSearch} aria-label="Buscar" title="Buscar">
            <Search aria-hidden />
          </Button>
        )}
        {onToggleDetails && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={detailsOpen ? "icon-button is-on" : "icon-button"}
            onClick={onToggleDetails}
            aria-label={detailsOpen ? "Ocultar viaje" : "Mostrar viaje"}
            title={detailsOpen ? "Ocultar viaje" : "Mostrar viaje"}
            aria-pressed={detailsOpen}
          >
            <PanelRight aria-hidden />
          </Button>
        )}
      </header>
      {banner}

      <div className="messages" ref={scroller}>
        <div ref={topSentinel} />
        {hasOlder && <p className="loading-older">Cargando mensajes anteriores…</p>}
        {loading && messages.length === 0 && <p className="thread-state">Cargando conversación…</p>}
        {error && <p className="thread-state error">{error}</p>}
        {!loading && !error && messages.length === 0 && (
          <p className="thread-state">Esta conversación todavía no tiene mensajes.</p>
        )}
        {messages.map((message, index) => {
          const label = dayLabel(message.timestamp);
          const previous = messages[index - 1];
          const startsDay = !previous || dayKey(previous.timestamp) !== dayKey(message.timestamp);
          const startsUnread = unreadAt?.messageId === message.id;
          const first = startsDay || startsUnread || authorKey(previous) !== authorKey(message);
          return (
            <Fragment key={message.id}>
              {startsDay && (
                <div className="day-divider" role="separator" aria-label={label}>
                  <span>{label}</span>
                </div>
              )}
              {startsUnread && (
                <div className="unread-divider" role="separator" ref={unreadDivider}>
                  <span>{unreadAt.count} {unreadAt.count === 1 ? "mensaje sin leer" : "mensajes sin leer"}</span>
                </div>
              )}
              <div
                ref={(node) => {
                  if (node) messageRefs.current.set(message.id, node);
                  else messageRefs.current.delete(message.id);
                }}
                data-message-id={message.id}
                aria-current={message.id === highlightedId ? "true" : undefined}
                className={message.id === highlightedId ? "highlight" : undefined}
              >
                <MessageBubble
                  message={message}
                  isGroup={conversation.isGroup}
                  first={first}
                  onReply={selectReply}
                  onOpenQuote={openQuote}
                />
              </div>
            </Fragment>
          );
        })}
      </div>

      <Composer
        conversationId={conversation.id}
        disabled={lineStatus !== undefined && lineStatus !== "open"}
        disabledReason="La línea está desconectada. Volvé a vincularla para poder escribir."
        onOptimistic={(key, body, fileName, quote) => {
          // Un envío nuevo invalida cualquier compensación pendiente de la
          // paginación hacia atrás: desde este momento el ancla es el final.
          prevHeight.current = 0;
          thread.addOptimistic(key, body, fileName, quote);
        }}
        onSettled={thread.settleOptimistic}
        replyTo={replyingTo}
        onCancelReply={() => setReplyingTo(undefined)}
        initialBody={draft}
        onSent={(message) => { setReplyingTo(undefined); onMessageSent?.(message); }}
      />
    </div>
  );
}

function dayKey(value: string): string {
  const date = new Date(value);
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
}

/** Quién firma una burbuja: dos seguidas del mismo autor forman una tanda. */
function authorKey(message: Message): string {
  if (message.direction === "inbound") return `in:${message.senderJid ?? ""}`;
  return message.origin === "panel" ? `panel:${message.sentById ?? ""}` : "out";
}

const weekdayName = new Intl.DateTimeFormat("es-AR", { weekday: "long" });
const monthName = new Intl.DateTimeFormat("es-AR", { month: "long" });

/** «Hoy», «Ayer» o «Lunes 19 de octubre»; con el año si no es este. */
export function dayLabel(value: string, now = new Date()): string {
  const date = new Date(value);
  const key = dayKey(value);
  if (key === dayKey(now.toISOString())) return "Hoy";
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (key === dayKey(yesterday.toISOString())) return "Ayer";
  const weekday = weekdayName.format(date);
  const label = `${weekday.charAt(0).toUpperCase()}${weekday.slice(1)} ${date.getDate()} de ${monthName.format(date)}`;
  return date.getFullYear() === now.getFullYear() ? label : `${label} de ${date.getFullYear()}`;
}
