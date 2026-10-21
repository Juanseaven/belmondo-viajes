"use client";

import { useState } from "react";
import { ChevronDown, CircleAlert, Smartphone, Unplug } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";

import type { Line } from "./api";

function lineLabel(line: Line): string {
  return line.displayName || line.label;
}

function lineStatus(line: Line): string {
  if (line.status === "open") return "Conectada";
  if (line.status === "connecting") return "Conectando";
  return "Desconectada";
}

export function LineFilter({
  lines,
  selected,
  onSelect,
  canManage = false,
  onDisconnect,
}: {
  lines: Line[];
  selected: string;
  onSelect: (instanceId: string) => void;
  canManage?: boolean;
  onDisconnect?: (line: Line) => Promise<void>;
}) {
  const unreadChats = lines.reduce((sum, line) => sum + line.unread, 0);
  const current = lines.find((line) => line.id === selected);
  const currentLabel = current ? lineLabel(current) : "Todas";
  const [disconnecting, setDisconnecting] = useState<Line | null>(null);
  const [confirmation, setConfirmation] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const openConfirmation = (line: Line) => {
    setDisconnecting(line);
    setConfirmation("");
    setError(null);
  };

  const closeConfirmation = () => {
    if (submitting) return;
    setDisconnecting(null);
    setConfirmation("");
    setError(null);
  };

  const confirmDisconnect = async () => {
    if (!disconnecting || !onDisconnect) return;
    setSubmitting(true);
    setError(null);
    try {
      await onDisconnect(disconnecting);
      setDisconnecting(null);
      setConfirmation("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo desvincular la línea");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <div className="line-selector" aria-label="Filtrar por línea">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="line-pill"
              aria-label={`Todas las líneas, filtro actual: ${currentLabel}`}
            >
              <span className={`line-status-dot ${current?.status ?? "open"}`} aria-hidden />
              <span className="line-pill-label">{current ? lineLabel(current) : "Todas las líneas"}</span>
              <ChevronDown aria-hidden />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="center" sideOffset={6} className="line-selector-menu">
            <DropdownMenuLabel>Ver conversaciones de</DropdownMenuLabel>
            <DropdownMenuRadioGroup value={selected} onValueChange={onSelect}>
              <DropdownMenuRadioItem value="">
                <span className="line-menu-copy">
                  <strong>Todas las líneas</strong>
                  <small>{unreadChats ? `${unreadChats} ${unreadChats === 1 ? "chat" : "chats"} sin leer` : "Bandeja unificada"}</small>
                </span>
              </DropdownMenuRadioItem>
              {lines.map((line) => (
                <DropdownMenuRadioItem key={line.id} value={line.id}>
                  <span className={`line-status-dot ${line.status}`} aria-hidden />
                  <span className="line-menu-copy">
                    <strong>{lineLabel(line)}</strong>
                    <small>{line.phoneNumber || lineStatus(line)}</small>
                  </span>
                  {line.unread > 0 && <Badge>{line.unread}</Badge>}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
            {canManage && onDisconnect && lines.length > 0 && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuLabel>Desvincular</DropdownMenuLabel>
                {lines.map((line) => (
                  <DropdownMenuItem key={line.id} variant="destructive" onSelect={() => openConfirmation(line)}>
                    <Unplug aria-hidden />
                    Desvincular {lineLabel(line)}
                  </DropdownMenuItem>
                ))}
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <Dialog open={Boolean(disconnecting)} onOpenChange={(open) => !open && closeConfirmation()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Desvincular {disconnecting ? lineLabel(disconnecting) : "línea"}</DialogTitle>
            <DialogDescription>
              Esta acción elimina del CRM la línea y todo su historial.
            </DialogDescription>
          </DialogHeader>

          <Alert variant="destructive">
            <CircleAlert aria-hidden />
            <AlertTitle>Los datos se borrarán</AlertTitle>
            <AlertDescription>
              Si conectás el número nuevamente, empezará como una línea nueva.
            </AlertDescription>
          </Alert>

          <label className="disconnect-confirmation" htmlFor="disconnect-confirmation">
            Escribí <strong>“{disconnecting ? lineLabel(disconnecting) : ""}”</strong> para confirmar
            <Input
              id="disconnect-confirmation"
              value={confirmation}
              autoComplete="off"
              autoFocus
              disabled={submitting}
              onChange={(event) => setConfirmation(event.target.value)}
            />
          </label>

          {error && <p className="form-error">{error}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={closeConfirmation} disabled={submitting}>
              Cancelar
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={
                submitting ||
                !disconnecting ||
                confirmation.trim() !== lineLabel(disconnecting)
              }
              onClick={() => void confirmDisconnect()}
            >
              <Smartphone aria-hidden />
              {submitting ? "Desvinculando…" : "Desvincular"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
