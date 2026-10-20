"use client";

import { useEffect, useRef } from "react";
import { AlertCircle, Radio } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

import { ContactAvatar } from "./ContactAvatar";
import type { useInbox } from "./useInbox";

const TYPE_LABEL: Record<string, string> = {
  image: "Foto",
  video: "Vídeo",
  audio: "Audio",
  document: "Documento",
  sticker: "Sticker",
  location: "Ubicación",
  contact: "Contacto",
};

export function ConversationList({
  inbox,
  selectedId,
  onSelect,
  showAvatars = true,
}: {
  inbox: ReturnType<typeof useInbox>;
  selectedId: string;
  onSelect: (id: string) => void;
  showAvatars?: boolean;
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
            <Skeleton className="size-10 rounded-full" />
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
        <p>Conectá una línea para empezar a recibir mensajes en esta bandeja.</p>
      </div>
    );
  }

  return (
    <div className="conversation-scroll">
      <ul className="conversation-list">
        {inbox.items.map((conversation) => (
          <li key={conversation.id}>
            <button
              type="button"
              className={conversation.id === selectedId ? "conversation-row active" : "conversation-row"}
              onClick={() => onSelect(conversation.id)}
            >
              {showAvatars && (
                <ContactAvatar
                  name={conversation.contactName}
                  src={conversation.avatarUrl}
                  size="lg"
                  className="conversation-avatar"
                />
              )}
              <span className="conversation-copy">
                <span className="conversation-heading">
                  <strong>{conversation.contactName}</strong>
                  {conversation.lastMessageAt && (
                    <time dateTime={conversation.lastMessageAt}>{shortTime(conversation.lastMessageAt)}</time>
                  )}
                </span>
                <span className="conversation-preview-row">
                  <span className="conversation-preview">
                    {conversation.preview || TYPE_LABEL[conversation.lastType ?? ""] || "Sin vista previa"}
                  </span>
                  {conversation.unread > 0 && <Badge>{conversation.unread}</Badge>}
                </span>
                {conversation.unread > 0 && (
                  <span className="conversation-meta">
                    <span className="conversation-state warn">Sin leer</span>
                  </span>
                )}
              </span>
            </button>
          </li>
        ))}
        <li aria-hidden><div ref={sentinel} /></li>
      </ul>
    </div>
  );
}

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
