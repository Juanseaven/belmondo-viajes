"use client";

import { useRef, useState } from "react";
import { ApiError } from "../../kernel/api/client";
import { mirrorApi } from "./api";

/**
 * Compositor de mensajes.
 *
 * Envía de forma optimista: avisa hacia arriba con la clave de idempotencia
 * ANTES de que el servidor responda, para que el hilo pinte la burbuja al
 * instante. El cliente de Evolution mete un retardo deliberado de 800 ms para no
 * parecer un bot, así que sin esto la app se sentiría trabada en la acción más
 * frecuente.
 */
export function Composer({
  conversationId,
  disabled,
  disabledReason,
  onOptimistic,
  onSettled,
}: {
  conversationId: string;
  disabled?: boolean;
  disabledReason?: string;
  /** Se llama al apretar enviar, antes de que el servidor conteste. */
  onOptimistic: (key: string, body: string, fileName?: string) => void;
  /** Resuelve la burbuja provisional: con el id de WhatsApp, o con el error. */
  onSettled: (key: string, waMessageId: string | null, error?: string) => void;
}) {
  const [body, setBody] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const vacio = body.trim() === "" && !file;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (vacio || disabled) return;

    // La clave se genera una vez y se conserva en el reintento: es lo que hace
    // que reintentar no duplique el mensaje.
    const key = crypto.randomUUID();
    const texto = body.trim();
    const adjunto = file ?? undefined;

    // Se limpia el compositor ya: quien escribe tiene que poder seguir tecleando
    // sin esperar a WhatsApp.
    setBody("");
    setFile(null);
    if (fileInput.current) fileInput.current.value = "";

    onOptimistic(key, texto, adjunto?.name);

    try {
      const res = await mirrorApi.send(conversationId, {
        body: texto,
        idempotencyKey: key,
        file: adjunto,
      });
      onSettled(key, res.waMessageId);
    } catch (err) {
      const msg =
        err instanceof ApiError ? err.message : "No se pudo enviar el mensaje";
      onSettled(key, null, msg);
    }
  };

  if (disabled) {
    return (
      <div className="composer disabled">
        <p>{disabledReason ?? "No se puede enviar por esta línea."}</p>
      </div>
    );
  }

  return (
    <form className="composer" onSubmit={submit}>
      {file && (
        <div className="attachment">
          <span>📎 {file.name}</span>
          <button type="button" onClick={() => { setFile(null); if (fileInput.current) fileInput.current.value = ""; }}>
            Quitar
          </button>
        </div>
      )}

      <div className="composer-row">
        <label className="attach" title="Adjuntar archivo">
          📎
          <input
            ref={fileInput}
            type="file"
            hidden
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </label>

        <textarea
          value={body}
          placeholder="Escribí un mensaje…"
          rows={1}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={(e) => {
            // Enter envía, Shift+Enter salta de línea. Es lo que espera cualquiera
            // que venga de WhatsApp.
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void submit(e);
            }
          }}
        />

        <button type="submit" disabled={vacio}>
          Enviar
        </button>
      </div>
    </form>
  );
}
