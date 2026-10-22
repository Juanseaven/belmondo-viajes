"use client";

import { useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ContactAvatar } from "@/modules/mirror/ContactAvatar";
import { dayOf, monthYear, today } from "./format";
import type { Companion } from "./types";

/**
 * Traer pasajeros de los viajes anteriores del cliente. Cada persona aparece
 * una vez, en su versión más reciente, con el viaje de donde sale. Vienen
 * marcadas las del viaje anterior más reciente; las que ya están en este viaje,
 * deshabilitadas.
 */
export function BringPassengersDialog({ companions, onClose, onBring }: {
  companions: Companion[];
  onClose: () => void;
  onBring: (passengerIds: string[]) => Promise<void>;
}) {
  const [chosen, setChosen] = useState<Set<string>>(
    () => new Set(companions.filter((c) => c.selected && !c.inTrip).map((c) => c.passengerId)),
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  const toggle = (id: string) => setChosen((current) => {
    const next = new Set(current);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });

  const bring = async () => {
    setSaving(true);
    setError(undefined);
    try {
      // En el orden de la lista: si vienen dos titulares, queda el primero.
      await onBring(companions.filter((c) => chosen.has(c.passengerId)).map((c) => c.passengerId));
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudieron traer los pasajeros");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="bring-dialog sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Traer pasajeros de viajes anteriores</DialogTitle>
          <DialogDescription>
            Se copian con sus documentos, visas y permisos. Después se pueden editar en este viaje sin tocar los anteriores.
          </DialogDescription>
        </DialogHeader>
        <ul className="bring-list">
          {companions.map((companion) => {
            const name = `${companion.lastName}, ${companion.firstName}`;
            return (
              <li key={companion.passengerId}>
                <label className={companion.inTrip ? "bring-row is-disabled" : "bring-row"}>
                  <input type="checkbox" disabled={companion.inTrip} checked={!companion.inTrip && chosen.has(companion.passengerId)}
                    onChange={() => toggle(companion.passengerId)} aria-label={name} />
                  <ContactAvatar name={`${companion.firstName} ${companion.lastName}`} className="avatar-32" />
                  <span className="bring-copy">
                    <strong>{name}</strong>
                    <small>{companion.inTrip ? "Ya está en este viaje" : companionLine(companion)}</small>
                  </span>
                  <span className="bring-from">viaje #{companion.tripNumber}<br />{companion.destination || "sin destino"}</span>
                </label>
              </li>
            );
          })}
        </ul>
        {error && <p className="panel-error" role="alert">{error}</p>}
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button type="button" disabled={saving || chosen.size === 0} onClick={() => void bring()}>
            {saving ? "Trayendo…" : `Traer ${chosen.size} ${chosen.size === 1 ? "pasajero" : "pasajeros"}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** «Adulto · Pasaporte AR vence 08/2031 · 1 visa», con «vencido» si ya venció. */
export function companionLine(companion: Companion) {
  const parts: ReactNode[] = [companion.kind === "menor" ? minorText(companion.birthDate) : "Adulto"];
  const document = companion.document;
  if (document) {
    const expired = document.expiresAt && dayOf(document.expiresAt) < today();
    parts.push(<>
      {document.type} {document.country}{document.expiresAt && ` vence ${monthYear(document.expiresAt)}`}
      {expired && <span className="is-late"> · vencido</span>}
    </>);
  }
  if (companion.permitCount > 0) parts.push(`${companion.permitCount} ${companion.permitCount === 1 ? "visa" : "visas"}`);
  return parts.map((part, index) => <span key={index}>{index > 0 && " · "}{part}</span>);
}

function minorText(birthDate?: string) {
  if (!birthDate) return "Menor";
  const [by, bm, bd] = dayOf(birthDate).split("-").map(Number);
  const [y, m, d] = today().split("-").map(Number);
  const age = y - by - (m < bm || (m === bm && d < bd) ? 1 : 0);
  return `Menor · ${age} ${age === 1 ? "año" : "años"}`;
}
