"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { ChevronLeft, ChevronRight, Download, FileText, MessageCircle, Play, Star } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { useSocket, type ServerEvent } from "@/kernel/api/useSocket";
import { mirrorApi } from "@/modules/mirror/api";
import { operacionesApi } from "./api";
import { dayOf, shortDay, today } from "./format";
import type { CustomerFile, FilePage } from "./types";

type Chip = "todos" | "favoritos" | "documentos";

const PAGE = 60;
const DOCS_PREVIEW = 5;

/**
 * Los archivos de los chats del cliente: fotos y videos en grilla, documentos
 * en lista, y la estrella para tener a mano los importantes. Las dos listas
 * paginan por separado: con cientos de fotos, los documentos siguen a la vista.
 *
 * `preview` es la versión de la página del cliente: muestra las primeras fotos
 * y documentos, y «Ver todos» la despliega en el lugar.
 */
export function CustomerFiles({ customerId, customerName, columns = 3, preview }: {
  customerId: string;
  customerName: string;
  columns?: 3 | 6;
  /** Cuántas fotos mostrar antes de «Ver todos». Sin esto, todas. */
  preview?: number;
}) {
  const [chip, setChip] = useState<Chip>("todos");
  const [expanded, setExpanded] = useState(!preview);
  const [viewing, setViewing] = useState<number>();
  const chipChosen = useRef(false);
  const files = useFiles(customerId, chip);

  // La página del cliente abre en Favoritos si hay alguno; la pestaña, en Todos.
  useEffect(() => {
    if (preview && !chipChosen.current && files.counts.favoritos) {
      chipChosen.current = true;
      setChip("favoritos");
    }
  }, [preview, files.counts.favoritos]);

  const choose = (next: Chip) => { chipChosen.current = true; setChip(next); };

  const media = expanded || !preview ? files.media : files.media.slice(0, preview);
  const docs = expanded || chip === "documentos" ? files.docs : files.docs.slice(0, DOCS_PREVIEW);
  const total = files.counts.todos ?? 0;

  const toggle = async (file: CustomerFile) => {
    files.patch(file.messageId, !file.favorite);
    try {
      await operacionesApi.setFavorite(file, !file.favorite);
    } catch {
      files.patch(file.messageId, file.favorite);
    }
  };

  const chips: { value: Chip; label: string; count?: number }[] = [
    { value: "todos", label: "Todos", count: files.counts.todos },
    { value: "favoritos", label: "Favoritos", count: files.counts.favoritos },
    { value: "documentos", label: "Documentos", count: files.counts.documentos },
  ];

  return (
    <div className={`files${columns === 6 ? " is-wide" : ""}`}>
      <div className="files-chips" role="group" aria-label="Archivos">
        {chips.map((option) => (
          <button key={option.value} type="button" className="trips-chip" aria-pressed={chip === option.value} onClick={() => choose(option.value)}>
            {option.value === "favoritos" && <Star aria-hidden />}{option.label}{option.count !== undefined && <> <em>{option.count}</em></>}
          </button>
        ))}
      </div>

      {files.error && <p className="panel-error" role="alert">{files.error}</p>}
      {!files.loaded ? <p className="panel-note">Cargando archivos…</p> : (
        files.media.length === 0 && files.docs.length === 0 ? (
          <p className="files-empty">
            {chip === "favoritos" ? "Marcá con la estrella los archivos importantes para tenerlos a mano."
              : chip === "documentos" ? `Todavía no hay documentos en los chats de ${customerName}.`
                : `Todavía no hay archivos en los chats de ${customerName}.`}
          </p>
        ) : <>
          {chip !== "documentos" && media.length > 0 && (
            <MediaGrid files={media} onOpen={(index) => setViewing(index)} onStar={(file) => void toggle(file)} />
          )}
          {chip !== "documentos" && expanded && files.mediaCursor && <LoadMore onMore={files.moreMedia} label="Ver más fotos" />}
          {docs.length > 0 && <>
            {chip !== "documentos" && <h4 className="files-group">Documentos</h4>}
            <ul className="panel-card files-docs">
              {docs.map((file) => <DocumentRow key={file.messageId} file={file} onStar={() => void toggle(file)} />)}
            </ul>
          </>}
          {chip !== "documentos" && !expanded && files.docs.length > DOCS_PREVIEW && (
            <button type="button" className="link-button files-more" onClick={() => setExpanded(true)}>Ver más documentos</button>
          )}
          {(chip === "documentos" || expanded) && files.docsCursor && <LoadMore onMore={files.moreDocs} label="Ver más documentos" />}
          {preview && !expanded && (files.media.length > preview || files.docs.length > DOCS_PREVIEW || files.mediaCursor) && (
            <button type="button" className="link-button files-more" onClick={() => setExpanded(true)}>Ver todos ({total})</button>
          )}
        </>
      )}

      {viewing !== undefined && media[viewing] && (
        <FileViewer files={media} index={viewing} onIndex={setViewing} onClose={() => setViewing(undefined)}
          onStar={(file) => void toggle(file)} />
      )}
    </div>
  );
}

/** Cuántos archivos tiene el cliente, para el número de la pestaña. */
export function useFileCount(customerId: string) {
  const [count, setCount] = useState<number>();
  const load = useCallback(() => {
    Promise.resolve(operacionesApi.customerFiles(customerId, { tipo: "media", limit: 1 }))
      .then((page) => setCount(page?.counts.todos))
      .catch(() => setCount(undefined));
  }, [customerId]);
  useEffect(() => { load(); }, [load]);
  useSocket("mirror", (event: ServerEvent) => { if (isFileEvent(event)) load(); });
  return count;
}

function isFileEvent(event: ServerEvent) {
  if (event.type === "media.stored") return true;
  return event.type === "message.new" && Boolean((event.payload as { hasMedia?: boolean }).hasMedia);
}

/** Las dos listas, con su cursor cada una, y los números de los chips. */
function useFiles(customerId: string, chip: Chip) {
  const [media, setMedia] = useState<CustomerFile[]>([]);
  const [docs, setDocs] = useState<CustomerFile[]>([]);
  const [mediaCursor, setMediaCursor] = useState<string>();
  const [docsCursor, setDocsCursor] = useState<string>();
  const [counts, setCounts] = useState<FilePage["counts"]>({});
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string>();
  const [version, setVersion] = useState(0);
  const favoritos = chip === "favoritos";

  useEffect(() => {
    let current = true;
    const lists: Promise<FilePage | undefined>[] = [
      chip === "documentos" ? Promise.resolve(undefined) : operacionesApi.customerFiles(customerId, { tipo: "media", favoritos, limit: PAGE }),
      operacionesApi.customerFiles(customerId, { tipo: "documentos", favoritos, limit: PAGE }),
    ];
    Promise.all(lists)
      .then(([mediaPage, docsPage]) => {
        if (!current) return;
        setMedia(mediaPage?.items ?? []);
        setMediaCursor(mediaPage?.nextCursor);
        setDocs(docsPage?.items ?? []);
        setDocsCursor(docsPage?.nextCursor);
        setCounts(docsPage?.counts ?? mediaPage?.counts ?? {});
        setError(undefined);
      })
      .catch((cause) => { if (current) setError(cause instanceof Error ? cause.message : "No se pudieron cargar los archivos"); })
      .finally(() => { if (current) setLoaded(true); });
    return () => { current = false; };
  }, [customerId, chip, favoritos, version]);

  // Llega una foto nueva, termina de bajarse una, o alguien marca una estrella.
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const refresh = () => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setVersion((value) => value + 1), 800);
  };
  useEffect(() => () => clearTimeout(timer.current), []);
  useSocket("mirror", (event: ServerEvent) => { if (isFileEvent(event)) refresh(); });
  useSocket("operaciones", (event: ServerEvent) => { if (event.type === "file.favorite") refresh(); });

  const more = (tipo: "media" | "documentos") => {
    const cursor = tipo === "media" ? mediaCursor : docsCursor;
    if (!cursor) return;
    const setCursor = tipo === "media" ? setMediaCursor : setDocsCursor;
    const setItems = tipo === "media" ? setMedia : setDocs;
    setCursor(undefined);
    operacionesApi.customerFiles(customerId, { tipo, favoritos, cursor, limit: PAGE })
      .then((page) => {
        setItems((items) => [...items, ...page.items]);
        setCursor(page.nextCursor);
      })
      .catch(() => setCursor(cursor));
  };

  /** La estrella cambia al momento; si el servidor falla, vuelve. */
  const patch = (messageId: string, favorite: boolean) => {
    const apply = (items: CustomerFile[]) => items.map((file) => (file.messageId === messageId ? { ...file, favorite } : file));
    setMedia(apply);
    setDocs(apply);
    setCounts((current) => ({ ...current, favoritos: Math.max(0, (current.favoritos ?? 0) + (favorite ? 1 : -1)) }));
  };

  return { media, docs, mediaCursor, docsCursor, counts, loaded, error, patch,
    moreMedia: () => more("media"), moreDocs: () => more("documentos") };
}

/** «Esta semana», «Este mes», «septiembre», «diciembre 2025». */
export function period(sentAt: string, from = today()) {
  const day = dayOf(sentAt);
  const diff = (new Date(`${from}T12:00:00`).getTime() - new Date(`${day}T12:00:00`).getTime()) / 86_400_000;
  if (diff < 7) return "Esta semana";
  if (day.slice(0, 7) === from.slice(0, 7)) return "Este mes";
  const month = new Intl.DateTimeFormat("es-AR", { month: "long" }).format(new Date(`${day}T12:00:00`));
  return day.slice(0, 4) === from.slice(0, 4) ? month.charAt(0).toUpperCase() + month.slice(1) : `${month.charAt(0).toUpperCase() + month.slice(1)} ${day.slice(0, 4)}`;
}

function MediaGrid({ files, onOpen, onStar }: { files: CustomerFile[]; onOpen: (index: number) => void; onStar: (file: CustomerFile) => void }) {
  const groups: { title: string; items: { file: CustomerFile; index: number }[] }[] = [];
  files.forEach((file, index) => {
    const title = period(file.sentAt);
    const last = groups.at(-1);
    if (last?.title === title) last.items.push({ file, index });
    else groups.push({ title, items: [{ file, index }] });
  });
  return <>
    {groups.map((group) => (
      <section key={group.title} className="files-section">
        <h4 className="files-group">{group.title}</h4>
        <ul className="files-grid">
          {group.items.map(({ file, index }) => (
            <li key={file.messageId}>
              <Thumb file={file} onOpen={() => onOpen(index)} onStar={() => onStar(file)} />
            </li>
          ))}
        </ul>
      </section>
    ))}
  </>;
}

/** La URL firmada del archivo, pedida recién cuando entra en pantalla. */
function useSignedUrl(file: CustomerFile, lazy = true) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [url, setUrl] = useState<string>();
  useEffect(() => {
    if (file.status !== "stored") return;
    const load = () => mirrorApi.media(file.assetId).then((signed) => setUrl(signed.url)).catch(() => setUrl(undefined));
    const node = ref.current;
    if (!lazy || !node || typeof IntersectionObserver === "undefined") {
      void load();
      return;
    }
    const observer = new IntersectionObserver(([entry]) => {
      if (!entry.isIntersecting) return;
      observer.disconnect();
      void load();
    }, { rootMargin: "200px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, [file.assetId, file.status, lazy]);
  return { ref, url };
}

const duration = (seconds?: number) => seconds === undefined ? "" : `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;

function Thumb({ file, onOpen, onStar }: { file: CustomerFile; onOpen: () => void; onStar: () => void }) {
  const { ref, url } = useSignedUrl(file);
  const ready = file.status === "stored";
  const label = file.type === "video" ? `Video del ${shortDay(file.sentAt)}` : `Foto del ${shortDay(file.sentAt)}`;
  return (
    <div className={`files-thumb${ready ? "" : " is-pending"}`} ref={ref}>
      <button type="button" className="files-open" aria-label={label} disabled={!ready} onClick={onOpen}>
        {ready && url && (file.type === "video"
          // eslint-disable-next-line jsx-a11y/media-has-caption -- miniatura muda.
          ? <video src={url} preload="metadata" muted />
          // eslint-disable-next-line @next/next/no-img-element -- URL firmada de vida corta.
          : <img src={url} alt="" loading="lazy" />)}
        {!ready && <span className="files-status">{file.status === "failed" ? "No se pudo bajar" : "Cargando…"}</span>}
        {file.type === "video" && ready && <span className="files-play" aria-hidden><Play /></span>}
        <span className="files-date">{file.type === "video" && file.durationSeconds !== undefined ? duration(file.durationSeconds) : shortDay(file.sentAt)}</span>
      </button>
      {file.direction === "outbound" && <span className="files-who">Agencia</span>}
      <StarButton file={file} onStar={onStar} />
    </div>
  );
}

function StarButton({ file, onStar }: { file: CustomerFile; onStar: () => void }) {
  return (
    <button type="button" className={`files-star${file.favorite ? " is-on" : ""}`} aria-pressed={file.favorite}
      aria-label={file.favorite ? "Sacar de favoritos" : "Marcar como favorito"} title={file.favorite ? "Sacar de favoritos" : "Marcar como favorito"}
      onClick={(event) => { event.stopPropagation(); onStar(); }}>
      <Star aria-hidden />
    </button>
  );
}

const size = (bytes?: number) => {
  if (bytes === undefined) return undefined;
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1).replace(".", ",")} MB`;
};

/** Un documento no abre el visor: se abre en una pestaña nueva. */
function DocumentRow({ file, onStar }: { file: CustomerFile; onStar: () => void }) {
  const ext = file.fileName?.split(".").pop()?.toUpperCase().slice(0, 4) || "DOC";
  const open = async () => {
    const tab = window.open("", "_blank");
    try {
      const signed = await mirrorApi.media(file.assetId);
      if (tab) tab.location.href = signed.url; else window.open(signed.url, "_blank");
    } catch {
      tab?.close();
    }
  };
  return (
    <li className="files-doc">
      <span className="files-ext" aria-hidden>{file.status === "stored" ? ext : <FileText />}</span>
      <button type="button" className="files-doc-open" disabled={file.status !== "stored"} onClick={() => void open()}>
        <strong>{file.fileName || "Documento"}</strong>
        <small>{[shortDay(file.sentAt), size(file.sizeBytes), file.direction === "outbound" ? "de la agencia" : "del cliente",
          file.status === "failed" ? "no se pudo bajar" : file.status !== "stored" ? "cargando…" : undefined].filter(Boolean).join(" · ")}</small>
      </button>
      <StarButton file={file} onStar={onStar} />
    </li>
  );
}

function LoadMore({ onMore, label }: { onMore: () => void; label: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = ref.current;
    if (!node || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(([entry]) => entry.isIntersecting && onMore(), { rootMargin: "200px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, [onMore]);
  return <div ref={ref} className="files-load"><Button size="sm" variant="ghost" onClick={onMore}>{label}</Button></div>;
}

const timeFormat = new Intl.DateTimeFormat("es-AR", { hour: "2-digit", minute: "2-digit", hour12: false });

/**
 * El visor: la foto o el video en grande, con flechas para pasar dentro de la
 * vista, la estrella, «Ver en el chat» y «Descargar».
 */
export function FileViewer({ files, index, onIndex, onClose, onStar }: {
  files: CustomerFile[];
  index: number;
  onIndex: (index: number) => void;
  onClose: () => void;
  onStar: (file: CustomerFile) => void;
}) {
  const router = useRouter();
  const file = files[index];
  const { url } = useSignedUrl(file, false);
  const prev = index > 0 ? () => onIndex(index - 1) : undefined;
  const next = index < files.length - 1 ? () => onIndex(index + 1) : undefined;
  const who = file.direction === "outbound" ? "de la agencia" : "del cliente";

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "ArrowLeft") prev?.();
      if (event.key === "ArrowRight") next?.();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="file-viewer" showCloseButton>
        <div className="file-viewer-head">
          <DialogTitle>{file.type === "video" ? "Video" : "Foto"} {who}</DialogTitle>
          <DialogDescription>
            {shortDay(file.sentAt)} a las {timeFormat.format(new Date(file.sentAt))}{file.instanceLabel && ` · Línea ${file.instanceLabel}`}
          </DialogDescription>
          {file.caption && <p className="file-viewer-caption">{file.caption}</p>}
        </div>
        <div className="file-viewer-body">
          {prev && <button type="button" className="file-viewer-nav is-prev" aria-label="Anterior" onClick={prev}><ChevronLeft /></button>}
          {url ? (file.type === "video"
            // eslint-disable-next-line jsx-a11y/media-has-caption -- video del chat.
            ? <video src={url} controls autoPlay />
            // eslint-disable-next-line @next/next/no-img-element -- URL firmada de vida corta.
            : <img src={url} alt={file.caption || `Foto ${who}`} />)
            : <span className="panel-note">Cargando…</span>}
          {next && <button type="button" className="file-viewer-nav is-next" aria-label="Siguiente" onClick={next}><ChevronRight /></button>}
        </div>
        <div className="file-viewer-foot">
          <Button size="sm" variant={file.favorite ? "default" : "outline"} aria-pressed={file.favorite} onClick={() => onStar(file)}>
            <Star aria-hidden className={file.favorite ? "is-filled" : undefined} />Favorito
          </Button>
          <Button size="sm" variant="outline" onClick={() => router.push(`/mirror?c=${encodeURIComponent(file.conversationId)}&m=${encodeURIComponent(file.messageId)}`)}>
            <MessageCircle aria-hidden />Ver en el chat
          </Button>
          {url && <Button size="sm" variant="outline" asChild><a href={url} download target="_blank" rel="noreferrer"><Download aria-hidden />Descargar</a></Button>}
        </div>
      </DialogContent>
    </Dialog>
  );
}
