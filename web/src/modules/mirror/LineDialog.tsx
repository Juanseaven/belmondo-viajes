"use client";

import { useEffect, useState } from "react";
import { ApiError } from "../../kernel/api/client";
import { mirrorApi } from "./api";

/**
 * Alta de una línea nueva.
 *
 * Dos pasos: se registra la línea, y después se muestra el QR para escanear
 * desde el teléfono del vendedor. El QR **rota cada ~30 segundos**, así que la
 * pantalla lo va reemplazando a medida que llegan por websocket; el primero se
 * pide por HTTP para no arrancar en blanco.
 *
 * Reemplaza el `curl` más `INSERT` a mano que documentaba el README.
 */
export function LineDialog({
  qrEvents,
  onClose,
  onConnected,
}: {
  /** Último QR recibido por websocket: { instance, qr }. */
  qrEvents: { instance: string; qr: string } | null;
  onClose: () => void;
  onConnected: () => void;
}) {
  const [ownerLabel, setOwnerLabel] = useState("");
  const [creando, setCreando] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [linea, setLinea] = useState<{ id: string; instance: string } | null>(null);
  const [qr, setQr] = useState<string | null>(null);

  // Los QR que llegan en vivo solo interesan si son de la línea que estamos
  // dando de alta: puede haber otra vinculándose al mismo tiempo.
  useEffect(() => {
    if (linea && qrEvents?.instance === linea.instance) setQr(qrEvents.qr);
  }, [qrEvents, linea]);

  // Sondeo de respaldo: si el websocket se cortó, el QR igual se refresca. Es
  // barato y evita que el alta se quede clavada sin explicación.
  useEffect(() => {
    if (!linea) return;
    const timer = setInterval(() => {
      mirrorApi
        .lineQR(linea.id)
        .then((res) => setQr(res.qr))
        .catch(() => {
          // 404 significa que ya no hay QR vigente: o se vinculó, o expiró.
          void mirrorApi
            .lines()
            .then((res) => {
              const actual = res.lines.find((l) => l.id === linea.id);
              if (actual?.status === "open") onConnected();
            })
            .catch(() => undefined);
        });
    }, 8000);
    return () => clearInterval(timer);
  }, [linea, onConnected]);

  const crear = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!ownerLabel.trim()) return;

    setCreando(true);
    setError(null);
    try {
      const res = await mirrorApi.createLine({ ownerLabel: ownerLabel.trim() });
      setLinea({ id: res.id, instance: res.instance });
      // El primer QR puede haber llegado antes de que montáramos el efecto.
      mirrorApi.lineQR(res.id).then((r) => setQr(r.qr)).catch(() => undefined);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "No se pudo crear la línea");
    } finally {
      setCreando(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header>
          <h2>Conectar una línea</h2>
          <button type="button" onClick={onClose}>×</button>
        </header>

        {!linea ? (
          <form onSubmit={crear}>
            <label>
              ¿De quién es el número?
              <input
                value={ownerLabel}
                onChange={(e) => setOwnerLabel(e.target.value)}
                placeholder="Ana Pérez"
                autoFocus
              />
            </label>
            <p className="hint">
              Aparece en la bandeja para saber por qué número entró cada mensaje.
            </p>
            {error && <p className="error">{error}</p>}
            <button type="submit" disabled={creando || !ownerLabel.trim()}>
              {creando ? "Creando…" : "Crear y mostrar el QR"}
            </button>
          </form>
        ) : (
          <div className="qr-step">
            <ol className="hint">
              <li>Abrí WhatsApp en el teléfono de {ownerLabel}.</li>
              <li>Andá a <strong>Dispositivos vinculados</strong>.</li>
              <li>Escaneá este código.</li>
            </ol>

            {qr ? (
              // eslint-disable-next-line @next/next/no-img-element -- es un data
              // URI en base64 que manda Evolution, no un archivo remoto.
              <img className="qr" src={qr} alt="Código QR de vinculación" />
            ) : (
              <p className="empty">Esperando el código…</p>
            )}

            <p className="hint">
              El código se renueva solo cada medio minuto. La ventana se cierra
              sola al vincularse.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
