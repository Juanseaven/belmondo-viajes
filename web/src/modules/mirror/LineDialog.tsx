"use client";

import { useEffect, useState } from "react";
import { LoaderCircle, QrCode, Smartphone } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

import { ApiError } from "../../kernel/api/client";
import { mirrorApi, type Line } from "./api";

export function LineDialog({
  existingLine,
  qrEvents,
  onClose,
  onConnected,
}: {
  existingLine?: Line;
  qrEvents: { instance: string; qr: string } | null;
  onClose: () => void;
  onConnected: () => void;
}) {
  const [ownerLabel, setOwnerLabel] = useState(existingLine?.displayName || existingLine?.label || "");
  const [creando, setCreando] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [linea, setLinea] = useState<{ id: string; instance: string } | null>(
    existingLine ? { id: existingLine.id, instance: existingLine.name } : null,
  );
  const [qr, setQr] = useState<string | null>(null);

  useEffect(() => {
    if (linea && qrEvents?.instance === linea.instance) setQr(qrEvents.qr);
  }, [qrEvents, linea]);

  useEffect(() => {
    if (!linea) return;
    const refresh = () => {
      mirrorApi
        .lineQR(linea.id)
        .then((res) => setQr(res.qr))
        .catch(() => {
          void mirrorApi
            .lines()
            .then((res) => {
              const actual = res.lines.find((line) => line.id === linea.id);
              if (actual?.status === "open") onConnected();
            })
            .catch(() => undefined);
        });
    };
    refresh();
    const timer = setInterval(refresh, 8000);
    return () => clearInterval(timer);
  }, [linea, onConnected]);

  const crear = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!ownerLabel.trim()) return;
    setCreando(true);
    setError(null);
    try {
      const response = await mirrorApi.createLine({ ownerLabel: ownerLabel.trim() });
      setLinea({ id: response.id, instance: response.instance });
      mirrorApi.lineQR(response.id).then((item) => setQr(item.qr)).catch(() => undefined);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "No se pudo crear la línea");
    } finally {
      setCreando(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="line-dialog sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{existingLine ? `Volver a vincular ${ownerLabel}` : "Conectar una línea"}</DialogTitle>
          <DialogDescription>
            {existingLine
              ? "La sesión anterior fue revocada. El historial se conserva en esta misma línea."
              : "Vinculá el WhatsApp de un vendedor para atenderlo desde esta bandeja."}
          </DialogDescription>
        </DialogHeader>

        {!linea ? (
          <form className="line-dialog-form" onSubmit={crear}>
            <label htmlFor="line-owner">Nombre de la línea</label>
            <Input
              id="line-owner"
              value={ownerLabel}
              onChange={(event) => setOwnerLabel(event.target.value)}
              placeholder="Ej. Ventas Juan"
              autoFocus
            />
            <p className="field-hint">
              Se mostrará junto a cada conversación para identificar el número de origen.
            </p>
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <DialogFooter className="line-dialog-footer">
              <Button type="button" variant="outline" onClick={onClose}>Cancelar</Button>
              <Button type="submit" disabled={creando || !ownerLabel.trim()}>
                {creando ? <LoaderCircle className="animate-spin" aria-hidden /> : <Smartphone aria-hidden />}
                {creando ? "Creando…" : "Crear y mostrar QR"}
              </Button>
            </DialogFooter>
          </form>
        ) : (
          <div className="qr-step">
            <ol>
              <li>Abrí WhatsApp en el teléfono de {ownerLabel}.</li>
              <li>Entrá en <strong>Dispositivos vinculados</strong>.</li>
              <li>Escaneá este código.</li>
            </ol>
            {qr ? (
              // eslint-disable-next-line @next/next/no-img-element -- QR en data URI.
              <img className="qr" src={qr} alt="Código QR de vinculación" />
            ) : (
              <div className="qr-waiting">
                <QrCode aria-hidden />
                <span>Esperando el código…</span>
              </div>
            )}
            <p className="field-hint">El código se renueva automáticamente y la ventana se cierra al vincularse.</p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
