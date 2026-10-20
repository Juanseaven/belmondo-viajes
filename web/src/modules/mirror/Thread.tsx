"use client";

import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { MessageCircleMore, PanelRightOpen, Phone } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { Conversation, Message, MessageQuote, SendResult } from "./api";
import { Composer } from "./Composer";
import { ContactAvatar } from "./ContactAvatar";
import { MessageBubble } from "./MessageBubble";
import { useThread } from "./useThread";

export function Thread({
  conversation,
  focusId,
  lineStatus,
  detailsHidden = false,
  onShowDetails,
  threadRef,
	banner,
	draft,
	onMessageSent,
}: {
  conversation: Conversation | null;
  focusId?: string;
  lineStatus?: string;
  detailsHidden?: boolean;
  onShowDetails?: () => void;
  threadRef?: (thread: ReturnType<typeof useThread>) => void;
  banner?: ReactNode;
  draft?: string;
  onMessageSent?: (message: SendResult) => void;
}) {
  const [replyingTo, setReplyingTo] = useState<MessageQuote>();
  const [aroundId, setAroundId] = useState(focusId);
  const [highlightedId, setHighlightedId] = useState(focusId);
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
        ? message.sentByName || (message.origin === "device" ? "Desde el teléfono" : "Vos")
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

  return (
    <div className="thread">
      <header className="thread-header">
        <div className="thread-identity">
          <ContactAvatar name={conversation.contactName} src={conversation.avatarUrl} size="lg" />
          <div className="thread-contact">
            <h2>{conversation.contactName}</h2>
            <span>
              {conversation.phoneNumber && <><Phone aria-hidden />{conversation.phoneNumber}<i aria-hidden>·</i></>}
              <span className={`line-status-dot ${lineStatus ?? "open"}`} aria-hidden />
              {conversation.instanceLabel}
            </span>
          </div>
        </div>
        {detailsHidden && onShowDetails && (
          <Button type="button" variant="outline" size="sm" onClick={onShowDetails} aria-label="Ver detalles">
            <PanelRightOpen aria-hidden />
            Ver detalles
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
          const startsDay = index === 0 || dayKey(messages[index - 1].timestamp) !== dayKey(message.timestamp);
          return (
            <Fragment key={message.id}>
              {startsDay && (
                <div className="day-divider" role="separator" aria-label={label}>
                  <span>{label}</span>
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

function dayLabel(value: string): string {
  return new Intl.DateTimeFormat("es-AR", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(new Date(value));
}
