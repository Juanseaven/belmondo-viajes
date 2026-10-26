"use client";

import { useEffect, useRef, type ComponentType, type ReactNode } from "react";
import { AlertCircle, Camera, FileText, MapPin, Mic, Radio, Sticker, UserRound, Video } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { displayName } from "@/lib/phone";

import type { Conversation } from "./api";
import { ContactAvatar } from "./ContactAvatar";
import { MessageTicks } from "./MessageTicks";
import type { useInbox } from "./useInbox";

/** Lo que se dice de un adjunto sin texto, con el ícono que usa WhatsApp. */
const TYPES: Record<string, { label: string; Icon: ComponentType<{ className?: string; "aria-hidden"?: boolean }> }> = {
  image: { label: "Foto", Icon: Camera },
  video: { label: "Video", Icon: Video },
  audio: { label: "Audio", Icon: Mic },
  document: { label: "Documento", Icon: FileText },
  sticker: { label: "Sticker", Icon: Sticker },
  location: { label: "Ubicación", Icon: MapPin },
  contact: { label: "Contacto", Icon: UserRound },
};

export function ConversationList({
  inbox,
  selectedId,
  onSelect,
  showAvatars = true,
  renderTags,
  emptyDescription,
}: {
  inbox: ReturnType<typeof useInbox>;
  selectedId: string;
  onSelect: (id: string) => void;
  showAvatars?: boolean;
  /** Un tercer renglón por fila: lo usa la pantalla para el viaje del contacto. */
  renderTags?: (conversation: Conversation) => ReactNode;
  emptyDescription?: string;
}) {
  const sentinel = useRef<HTMLDivElement>(null);

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
      <Alert variant="destructive" className="inbox-alert">
        <AlertCircle aria-hidden />
        <AlertTitle>No se pudo cargar la bandeja</AlertTitle>
        <AlertDescription>
          <span>{inbox.error}</span>
          <Button type="button" variant="outline" size="sm" onClick={() => void inbox.reload()}>
            Reintentar
          </Button>
        </AlertDescription>
      </Alert>
    );
  }

  if (inbox.loading && inbox.items.length === 0) {
    return (
      <div className="conversation-skeletons" aria-label="Cargando conversaciones">
        {Array.from({ length: 7 }, (_, index) => (
          <div className="conversation-skeleton" key={index}>
            <Skeleton className="size-12 rounded-full" />
            <div>
              <Skeleton className="mb-2 h-3.5 w-36" />
              <Skeleton className="h-3 w-48" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (inbox.items.length === 0) {
    return (
      <div className="inbox-empty">
        <Radio aria-hidden />
        <strong>No hay conversaciones todavía.</strong>
        <p>{emptyDescription ?? "Conectá una línea para empezar a recibir mensajes en esta bandeja."}</p>
      </div>
    );
  }

  return (
    <div className="conversation-scroll">
      <ul className="conversation-list">
        {inbox.items.map((conversation) => {
          const unread = conversation.unread > 0;
          const tags = renderTags?.(conversation);
          const classes = ["chat-row"];
          if (unread) classes.push("is-unread");
          if (conversation.id === selectedId) classes.push("active");
          return (
            <li key={conversation.id}>
              <button type="button" className={classes.join(" ")} onClick={() => onSelect(conversation.id)}>
                {showAvatars && (
                  <ContactAvatar name={conversation.contactName} src={conversation.avatarUrl} className="avatar-48" />
                )}
                <span className="chat-copy">
                  <span className="chat-top">
                    <strong>{displayName(conversation.contactName)}</strong>
                    {conversation.lastMessageAt && (
                      <time dateTime={conversation.lastMessageAt}>{listTime(conversation.lastMessageAt)}</time>
                    )}
                  </span>
                  <span className="chat-preview">
                    <Preview conversation={conversation} />
                    {unread && <span className="unread-badge" aria-label={`${conversation.unread} sin leer`}>{conversation.unread}</span>}
                  </span>
                  {tags && <span className="chat-tags">{tags}</span>}
                </span>
              </button>
            </li>
          );
        })}
        <li aria-hidden><div ref={sentinel} /></li>
      </ul>
    </div>
  );
}

/** La vista previa como en WhatsApp: tildes si lo mandó la agencia y, en un grupo, quién escribió. */
function Preview({ conversation }: { conversation: Conversation }) {
  const type = TYPES[conversation.lastType ?? ""];
  const outbound = conversation.lastDirection === "outbound";
  const text = conversation.preview || type?.label || "Sin vista previa";
  return (
    <>
      {outbound && <MessageTicks status={conversation.lastStatus} />}
      {type && <type.Icon className="chat-type" aria-hidden />}
      <span className="chat-text">
        {conversation.isGroup && !outbound && conversation.lastSender && (
          <span className="chat-who">{conversation.lastSender}: </span>
        )}
        {text}
      </span>
    </>
  );
}

const weekday = new Intl.DateTimeFormat("es-AR", { weekday: "long" });

/** La hora si es de hoy, «Ayer», el día de la semana si es de esta semana, y si no la fecha. */
export function listTime(iso: string, now = new Date()): string {
  const date = new Date(iso);
  const startOf = (value: Date) => new Date(value.getFullYear(), value.getMonth(), value.getDate()).getTime();
  const days = Math.round((startOf(now) - startOf(date)) / 86_400_000);
  if (days <= 0) return date.toLocaleTimeString("es-AR", { hour: "2-digit", minute: "2-digit", hour12: false });
  if (days === 1) return "Ayer";
  if (days < 7) return weekday.format(date);
  return date.toLocaleDateString("es-AR", { day: "2-digit", month: "2-digit", year: "numeric" });
}
