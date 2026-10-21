"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * Una pantalla de edición del panel: reemplaza su contenido (no es un modal),
 * con «←» arriba para volver y un único botón de guardar fijo abajo, con un
 * aviso de qué cambió al lado. Volver con cambios sin guardar pregunta antes de
 * descartarlos.
 */
export function PanelEditor({ title, saveLabel = "Guardar cambios", changes, saving, error, canSave = true, onSave, onBack, children }: {
  title: string;
  saveLabel?: string;
  /** Qué cambió, dicho en una frase. Vacío: no hay nada sin guardar. */
  changes?: string;
  saving?: boolean;
  error?: string;
  canSave?: boolean;
  onSave: () => void;
  onBack: () => void;
  children: ReactNode;
}) {
  const [confirming, setConfirming] = useState(false);
  const back = () => (changes ? setConfirming(true) : onBack());

  return (
    <form className="panel-editor" aria-label={title} onSubmit={(event: FormEvent) => { event.preventDefault(); onSave(); }}>
      <header className="panel-editor-head">
        <Button type="button" variant="ghost" size="icon" className="icon-button" aria-label="Volver" title="Volver" onClick={back}>
          <ArrowLeft aria-hidden />
        </Button>
        <strong>{title}</strong>
      </header>
      <div className="panel-editor-body">{children}</div>
      {error && <p className="panel-error" role="alert">{error}</p>}
      <footer className="panel-editor-foot">
        {confirming ? (
          <>
            <span className="panel-editor-question">¿Descartar los cambios?</span>
            <Button type="button" variant="destructive" size="sm" onClick={onBack}>Descartar</Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => setConfirming(false)}>Seguir editando</Button>
          </>
        ) : (
          <>
            <Button type="submit" size="sm" disabled={saving || !canSave}>{saving ? "Guardando…" : saveLabel}</Button>
            <Button type="button" variant="ghost" size="sm" onClick={back}>Cancelar</Button>
            {changes && <small>{changes}</small>}
          </>
        )}
      </footer>
    </form>
  );
}

/** Un campo con su rótulo arriba. */
export function Field({ label, children, wide }: { label: string; children: ReactNode; wide?: boolean }) {
  return <label className={wide ? "field is-wide" : "field"}><span>{label}</span>{children}</label>;
}

/** «Cambiaste las fechas y el cliente». */
export function describeChanges(parts: string[]): string {
  if (parts.length === 0) return "";
  const list = parts.length === 1 ? parts[0] : `${parts.slice(0, -1).join(", ")} y ${parts.at(-1)}`;
  return `Cambiaste ${list}`;
}
