"use client";

import { Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { Control } from "./types";
import { nextControl } from "./types";

export interface ControlActions {
  onRun: (control: Control) => void;
  onClose: (control: Control) => void;
  onReopen: (control: Control) => void;
  /**
   * El cliente no tiene chat: lo que se cierra mandando un mensaje se marca a
   * mano, y queda quién y cuándo.
   */
  markByHand?: boolean;
}

/**
 * Un control. Abierto: un círculo, el título, «Importante» si es crítico y
 * el botón de la acción. Hecho: tilde
 * gris, título en gris, quién y cuándo.
 */
export function ControlRow({ control, onRun, onClose, onReopen, markByHand }: { control: Control } & ControlActions) {
  const done = control.status === "cerrado";
  return (
    <li className={`control-row${done ? " is-done" : ""}`}>
      {done
        ? <span className="control-check" aria-hidden><Check /></span>
        : <span className="control-ring is-later" aria-hidden />}
      <span className="control-copy">
        <strong>{control.title}{!done && control.critical && <span className="control-critical">Importante</span>}</strong>
        {done
          ? <small>{closedBy(control)}</small>
          : null}
      </span>
      {!done && control.closing === "accion" && (markByHand
        ? <Button size="sm" variant="outline" onClick={() => onClose(control)}>Marcar hecho</Button>
        : <Button size="sm" variant="outline" onClick={() => onRun(control)}>{control.actionLabel || "Hacer ahora"}</Button>)}
      {!done && control.closing === "manual" && <Button size="sm" variant="outline" onClick={() => onClose(control)}>Confirmar</Button>}
      {/* Sin prueba, el cierre fue del dato: lo gobierna el dato, no una persona. */}
      {done && control.closing !== "derivado" && control.evidence.length > 0 && <Button size="sm" variant="ghost" onClick={() => onReopen(control)}>Reabrir</Button>}
    </li>
  );
}

const time = new Intl.DateTimeFormat("es-AR", { hour: "2-digit", minute: "2-digit", hour12: false });

/** Todo dice quién y cuándo: nadie se acuerda de un viaje que no vendió. */
function closedBy(control: Control) {
  const last = [...control.evidence].sort((a, b) => b.closedAt.localeCompare(a.closedAt))[0];
  // Un derivado, o uno por acción que el dato cerró antes de que hiciera falta
  // avisar: no hubo persona ni mensaje.
  if (control.closing === "derivado" || !last) return "Se completó con el dato cargado";
  const at = new Date(last.closedAt);
  const when = `${String(at.getDate()).padStart(2, "0")}/${String(at.getMonth() + 1).padStart(2, "0")} a las ${time.format(at)}`;
  return `Cerró ${last.closedByName || "alguien del equipo"} · ${when}`;
}

/** Los controles en dos grupos: lo que falta hacer y lo hecho. */
export function ControlsTab({ controls, ...actions }: { controls: Control[] } & ControlActions) {
  const open = controls.filter((control) => control.status === "abierto");
  const first = nextControl(open);
  // El mismo orden que «Lo que sigue»: lo crítico y lo más cercano arriba.
  const pending = first ? [first, ...sortByDue(open.filter((control) => control.id !== first.id))] : [];
  const done = controls.filter((control) => control.status === "cerrado");

  if (controls.length === 0) {
    return <div className="panel-empty"><strong>Este viaje no tiene controles</strong><p>Se generan solos al cargar los datos del viaje.</p></div>;
  }

  return (
    <>
      <h3 className="panel-group-title">Por hacer · {pending.length}</h3>
      {pending.length === 0
        ? <p className="panel-note">No queda nada por hacer en este viaje.</p>
        : <ul className="panel-card control-list">{pending.map((control) => <ControlRow key={control.id} control={control} {...actions} />)}</ul>}
      {done.length > 0 && <>
        <h3 className="panel-group-title">Hechos · {done.length}</h3>
        <ul className="panel-card control-list">{done.map((control) => <ControlRow key={control.id} control={control} {...actions} />)}</ul>
      </>}
    </>
  );
}

function sortByDue(controls: Control[]) {
  return [...controls].sort((a, b) => {
    if (a.critical !== b.critical) return a.critical ? -1 : 1;
    return (a.dueAt ?? "9999").localeCompare(b.dueAt ?? "9999");
  });
}
