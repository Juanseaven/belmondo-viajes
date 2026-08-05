"use client";

import { useEffect, useRef, useState } from "react";
import { mirrorApi, type Message, type MessageMedia } from "./api";

/**
 * Un mensaje del hilo.
 *
 * El origen decide de qué lado y con qué etiqueta se pinta:
 *   contact → izquierda, es el cliente.
 *   device  → derecha, salió del teléfono: sin autor en el sistema.
 *   panel   → derecha, con el nombre de quien lo escribió desde la app.
 */
export function MessageBubble({ message }: { message: Message }) {
  const mine = message.direction === "outbound";

  return (
    <article className={`bubble ${mine ? "out" : "in"}`}>
      {message.origin === "panel" && message.sentByName && (
        <span className="author">{message.sentByName}</span>
      )}
      {message.origin === "device" && <span className="author muted">desde el teléfono</span>}

      {message.media && <MediaBlock media={message.media} />}

      {message.deletedAt ? (
        // El borrado no se oculta: que alguien borrara un mensaje es, en sí,
        // información que el espejo tiene que conservar.
        <p className="deleted">Mensaje eliminado</p>
      ) : (
        message.body && <p className="body">{message.body}</p>
      )}

      <footer>
        <time dateTime={message.timestamp}>
          {new Date(message.timestamp).toLocaleTimeString([], {
            hour: "2-digit",
            minute: "2-digit",
          })}
        </time>
        {message.editedAt && <span className="edited">editado</span>}
      </footer>
    </article>
  );
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
          📎 {media.fileName ?? "Documento"}
        </a>
      )}
    </div>
  );
}
