"use client";

import { useEffect, useRef, useState } from "react";
import { Paperclip, SendHorizontal, WifiOff, X } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

import { ApiError } from "../../kernel/api/client";
import { mirrorApi, type MessageQuote, type SendResult } from "./api";

export function Composer({
  conversationId,
  disabled,
  disabledReason,
  onOptimistic,
  onSettled,
  replyTo,
  onCancelReply,
  onSent,
	initialBody,
}: {
  conversationId: string;
  disabled?: boolean;
  disabledReason?: string;
  onOptimistic: (key: string, body: string, fileName?: string, quote?: MessageQuote) => void;
  onSettled: (key: string, waMessageId: string | null, error?: string) => void;
  replyTo?: MessageQuote;
  onCancelReply?: () => void;
  onSent?: (message: SendResult) => void;
  initialBody?: string;
}) {
  const [body, setBody] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const empty = body.trim() === "" && !file;

  useEffect(() => {
    if (initialBody !== undefined) setBody(initialBody);
  }, [initialBody]);

  const clearFile = () => {
    setFile(null);
    if (fileInput.current) fileInput.current.value = "";
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (empty || disabled) return;
    const key = crypto.randomUUID();
    const text = body.trim();
    const attachment = file ?? undefined;
    setBody("");
    clearFile();
    onOptimistic(key, text, attachment?.name, replyTo);

    try {
      const response = await mirrorApi.send(conversationId, {
        body: text,
        idempotencyKey: key,
        file: attachment,
        replyToMessageId: replyTo?.id,
      });
      onSettled(key, response.waMessageId);
      onSent?.(response);
    } catch (cause) {
      const message = cause instanceof ApiError ? cause.message : "No se pudo enviar el mensaje";
      onSettled(key, null, message);
    }
  };

  if (disabled) {
    return (
      <div className="composer composer-disabled">
        <Alert>
          <WifiOff aria-hidden />
          <AlertDescription>{disabledReason ?? "No se puede enviar por esta línea."}</AlertDescription>
        </Alert>
      </div>
    );
  }

  return (
    <form className="composer" onSubmit={submit}>
      {replyTo && (
        <div className="composer-reply">
          <span>
            <strong>Respondiendo a {replyTo.senderName || (replyTo.direction === "outbound" ? "Vos" : "Contacto")}</strong>
            <small>{quotePreview(replyTo)}</small>
          </span>
          <Button type="button" variant="ghost" size="icon-xs" onClick={onCancelReply} aria-label="Cancelar respuesta">
            <X aria-hidden />
          </Button>
        </div>
      )}
      {file && (
        <div className="attachment-preview">
          <Paperclip aria-hidden />
          <span>{file.name}</span>
          <Button type="button" variant="ghost" size="icon-xs" onClick={clearFile} aria-label="Quitar archivo">
            <X aria-hidden />
          </Button>
        </div>
      )}

      <div className="composer-row">
        <Button type="button" variant="ghost" size="icon" className="icon-button" asChild>
          <label aria-label="Adjuntar archivo" title="Adjuntar archivo">
            <Paperclip aria-hidden />
            <input
              ref={fileInput}
              type="file"
              hidden
              onChange={(event) => setFile(event.target.files?.[0] ?? null)}
            />
          </label>
        </Button>
        <Textarea
          value={body}
          aria-label="Mensaje"
          placeholder="Escribí un mensaje"
          rows={1}
          className="composer-field"
          onChange={(event) => setBody(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape" && replyTo) {
              event.preventDefault();
              onCancelReply?.();
              return;
            }
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              void submit(event);
            }
          }}
        />
        <Button type="submit" size="icon-lg" className="composer-send" disabled={empty} aria-label="Enviar mensaje">
          <SendHorizontal aria-hidden />
        </Button>
      </div>
    </form>
  );
}

function quotePreview(quote: MessageQuote): string {
  if (quote.body?.trim()) return quote.body;
  const labels: Record<string, string> = {
    image: "Foto",
    video: "Video",
    audio: "Audio",
    document: "Documento",
    sticker: "Sticker",
  };
  return labels[quote.type] ?? "Mensaje";
}
