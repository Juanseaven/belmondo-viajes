"use client";

import { useEffect, useRef, useState } from "react";
import { AlertCircle, LoaderCircle, Paperclip, Reply, Smartphone } from "lucide-react";
import { mirrorApi, type Message, type MessageMedia } from "./api";
import { MessageTicks } from "./MessageTicks";

/**
 * Un mensaje del hilo, como en WhatsApp.
 *
 * El origen decide de qué lado y cómo se firma:
 *   contact → izquierda, es el cliente.
 *   device  → derecha, salió del teléfono: un celular chico al lado de la hora.
 *   panel   → derecha, con el nombre de quien lo escribió desde la app.
 *
 * `first` es la primera de una tanda del mismo lado y el mismo autor: lleva la
 * colita y, si corresponde, el nombre arriba. Las demás no lo repiten.
 */
export function MessageBubble({
  message,
  isGroup = false,
  first = true,
  onReply,
  onOpenQuote,
}: {
  message: Message;
  isGroup?: boolean;
  first?: boolean;
  onReply?: (message: Message) => void;
  onOpenQuote?: (messageId: string) => void;
}) {
  const mine = message.direction === "outbound";

  // Provisional: pintado al enviar, todavía sin confirmar por WhatsApp.
  const provisional = Boolean(message.pending && !message.pending.error);
  const fallido = Boolean(message.pending?.error);

  const clases = ["bubble", mine ? "out" : "in"];
  if (first) clases.push("first");
  if (provisional) clases.push("provisional");
  if (fallido) clases.push("fallido");

  const groupAuthor = isGroup && !mine ? message.senderName || message.senderPhone || message.senderJid : undefined;
  const panelAuthor = mine && message.origin === "panel" ? message.sentByName : undefined;
  const author = first ? groupAuthor || panelAuthor : undefined;

  const meta = (
    <span className="bubble-meta">
      {message.editedAt && <span className="edited">editado</span>}
      {message.origin === "device" && (
        <span className="from-phone" title="Enviado desde el teléfono" aria-label="Enviado desde el teléfono">
          <Smartphone aria-hidden />
        </span>
      )}
      <time dateTime={message.timestamp}>
        {new Date(message.timestamp).toLocaleTimeString("es-AR", { hour: "2-digit", minute: "2-digit", hour12: false })}
      </time>
      {provisional && <span className="estado"><LoaderCircle className="spin" aria-hidden />Enviando</span>}
      {mine && !provisional && !fallido && <MessageTicks status={message.status} />}
    </span>
  );

  return (
    <article className={clases.join(" ")}>
      {onReply && !message.pending && (
        <button
          type="button"
          className="bubble-reply-action"
          aria-label="Responder mensaje"
          title="Responder"
          onClick={() => onReply(message)}
        >
          <Reply aria-hidden />
        </button>
      )}
      {author && <span className="author">{author}</span>}

      {message.quote ? (
        <button
          type="button"
          className="message-quote"
          aria-label="Ir al mensaje citado"
          onClick={() => onOpenQuote?.(message.quote!.id)}
          disabled={!onOpenQuote}
        >
          <strong>{message.quote.senderName || (message.quote.direction === "outbound" ? "Vos" : "Contacto")}</strong>
          <span>{quotePreview(message.quote.body, message.quote.type)}</span>
        </button>
      ) : message.quotedId ? (
        <div className="message-quote unavailable">
          <strong>Mensaje citado</strong>
          <span>Mensaje citado no disponible</span>
        </div>
      ) : null}

      {message.pending?.fileName && (
        <span className="attachment-name"><Paperclip aria-hidden />{message.pending.fileName}</span>
      )}

      {message.media && <MediaBlock media={message.media} />}

      {/* La hora va dentro del texto, flotando a la derecha del último renglón. */}
      {message.deletedAt ? (
        // El borrado no se oculta: que alguien borrara un mensaje es, en sí,
        // información que el espejo tiene que conservar.
        <p className="deleted">Mensaje eliminado{meta}</p>
      ) : message.body ? (
        <p className="body">{message.body}{meta}</p>
      ) : (
        <p className="body is-empty">{meta}</p>
      )}
      {fallido && <p className="estado error"><AlertCircle aria-hidden />{message.pending?.error}</p>}
    </article>
  );
}

function quotePreview(body: string | undefined, type: string): string {
  if (body?.trim()) return body;
  const labels: Record<string, string> = {
    image: "Foto",
    video: "Video",
    audio: "Audio",
    document: "Documento",
    sticker: "Sticker",
  };
  return labels[type] ?? "Mensaje";
}

/**
 * Adjunto con carga perezosa.
 *
 * La URL se pide solo cuando la burbuja entra en viewport. Pedirlas todas al
 * abrir el hilo serían 50 peticiones de firma para ver cinco imágenes, y cada
 * una caduca en minutos.
 */
function MediaBlock({ media }: { media: MessageMedia }) {
  const [url, setUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Todavía subiéndose: no hay nada que firmar.
    if (media.status !== "stored") return;

    const node = ref.current;
    if (!node) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry.isIntersecting) return;
        observer.disconnect();
        mirrorApi
          .media(media.id)
          .then((signed) => setUrl(signed.url))
          .catch(() => setFailed(true));
      },
      { rootMargin: "200px" }, // se adelanta al scroll para que no aparezca vacío
    );

    observer.observe(node);
    return () => observer.disconnect();
  }, [media.id, media.status]);

  if (media.status === "failed") {
    return <div className="media error">No se pudo recuperar el archivo</div>;
  }
  if (media.status !== "stored") {
    return <div className="media pending">Descargando adjunto…</div>;
  }

  const kind = media.mimeType?.split("/")[0];

  return (
    <div className="media" ref={ref}>
      {failed && <span className="error">No se pudo abrir el archivo</span>}
      {!failed && !url && <span className="pending">Cargando…</span>}

      {url && kind === "image" && (
        // eslint-disable-next-line @next/next/no-img-element -- la URL es firmada
        // y de vida corta; el optimizador de Next tendría que proxyarla.
        <img src={url} alt={media.fileName ?? "Imagen"} loading="lazy" />
      )}
      {url && kind === "video" && <video src={url} controls preload="none" />}
      {url && kind === "audio" && <audio src={url} controls preload="none" />}
      {url && kind !== "image" && kind !== "video" && kind !== "audio" && (
        <a href={url} target="_blank" rel="noreferrer" className="document">
          <Paperclip aria-hidden /> {media.fileName ?? "Documento"}
        </a>
      )}
    </div>
  );
}
