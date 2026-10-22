"use client";

import { useState } from "react";
import { Check, ChevronRight, Info, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { countryName } from "./countries";
import { Flag } from "./Flag";
import { shortInstant } from "./format";
import { travelDocument } from "./PassengersTab";
import type { Citation, Country, DetailLine, Passenger, PreviewLine, Requirement, RequirementDetail, TripDetail } from "./types";

const STATUS_LABEL: Record<Requirement["status"], string> = {
  cumple: "Cumple",
  falta: "Falta",
  verificar: "Verificar",
  aviso: "Recomendación",
};

const SOURCE_LABEL: Record<string, string> = {
  wikipedia: "Wikipedia",
  passport_index: "Passport Index",
  constante: "norma oficial",
};

/** Vigencia y antigüedad son del control del documento, no del de requisitos. */
const documentControl = (key: string) => key === "vigencia" || key === "antiguedad";

const searching = (requirement: Requirement) => Boolean(requirement.searching);

/**
 * Cuántos requisitos cumplen, faltan o hay que verificar. Las recomendaciones
 * no cuentan, ni lo que todavía se está buscando.
 */
export function requirementCounts(reqs: Requirement[]) {
  const counts = { cumple: 0, falta: 0, verificar: 0 };
  for (const r of reqs) {
    if (r.status === "aviso" || searching(r)) continue;
    counts[r.status] += 1;
  }
  return counts;
}

/**
 * Lo que cada pasajero necesita para entrar a cada país del itinerario: una
 * tarjeta por pasajero y país, un renglón por requisito con su estado a color.
 * El texto largo queda plegado, con la fuente al pie: el panel no dice «podés
 * viajar», dice qué encontró y de dónde.
 */
export function RequirementsTab({ trip, countries, onConfirm, onAddPermit }: {
  trip: TripDetail;
  countries: Country[];
  onConfirm?: (requirement: Requirement) => Promise<void> | void;
  /** «Cargar visa»: lleva a Pasajeros con ese pasajero abierto. */
  onAddPermit?: (passengerId: string) => void;
}) {
  const [error, setError] = useState<string>();
  const reqs = trip.requirements ?? [];
  const preview = trip.passengers.length === 0 ? trip.preview ?? [] : [];

  if (preview.length > 0) return <RequirementsPreview lines={preview} countries={countries} />;

  if (reqs.length === 0) {
    return (
      <div className="panel-empty">
        <strong>Todavía no hay requisitos</strong>
        <p>Cargá el país de destino y los pasajeros para ver los requisitos. Con el documento de cada pasajero, el sistema busca solo qué pide cada país.</p>
      </div>
    );
  }

  const confirm = async (requirement: Requirement) => {
    if (!onConfirm) return;
    setError(undefined);
    try {
      await onConfirm(requirement);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo confirmar");
    }
  };

  const tripLevel = reqs.filter((r) => !r.passengerId);
  const pendingVerify = reqs.some((r) => r.status === "verificar" && !documentControl(r.key) && !searching(r));
  const row = (r: Requirement, i: number) => (
    <RequirementRow key={`${r.key}-${i}`} requirement={r} onConfirm={onConfirm ? confirm : undefined}
      onAddPermit={onAddPermit && r.passengerId && r.key === "visa" && r.status === "falta" ? () => onAddPermit(r.passengerId!) : undefined} />
  );

  return (
    <>
      {error && <p className="panel-error" role="alert">{error}</p>}
      {tripLevel.length > 0 && (
        <section className="panel-card req-card">
          <CardHead title="Todo el viaje" lines={tripLevel} />
          <ul className="req-list">{tripLevel.map(row)}</ul>
        </section>
      )}
      {trip.passengers.map((passenger) => {
        const own = reqs.filter((r) => r.passengerId === passenger.id);
        if (own.length === 0) return null;
        const name = `${passenger.firstName} ${passenger.lastName}`;
        const subtitle = `${name} · ${documentText(passenger, countries)}`;
        // Lo que no es de un país va primero: la autorización de menor y las recomendaciones.
        const general = own.filter((r) => !r.country).sort((a, b) => Number(b.status === "aviso") - Number(a.status === "aviso"));
        const byCountry = groupByCountry(own.filter((r) => r.country));
        return (
          <section key={passenger.id} className="req-passenger" aria-label={name}>
            {general.length > 0 && (
              <div className="panel-card req-card">
                <CardHead title="General" subtitle={subtitle} lines={general} />
                <ul className="req-list">{general.map(row)}</ul>
              </div>
            )}
            {byCountry.map(([code, lines]) => {
              const title = `${countryName(countries, code)}${lines[0].role === "escala" ? " · escala" : ""}`;
              return (
                <div key={code} className="panel-card req-card">
                  <CardHead code={code} title={title} subtitle={subtitle} lines={lines} />
                  {lines.every(searching) ? (
                    <p className="req-note">Buscando los requisitos de {countryName(countries, code)}…</p>
                  ) : (
                    <>
                      <OfficialNote details={lines.map((r) => r.detail)} />
                      <ul className="req-list">{lines.map(row)}</ul>
                    </>
                  )}
                </div>
              );
            })}
          </section>
        );
      })}
      {pendingVerify && (
        <p className="panel-foot"><Info aria-hidden />Lo que está en «Verificar» lo resuelve la agencia: hasta confirmarlo, el aviso no cierra «Requisitos migratorios».</p>
      )}
    </>
  );
}

/** La cabecera de una tarjeta: bandera, país, pasajero y documento, y cuántos van listos. */
function CardHead({ code, title, subtitle, lines }: { code?: string; title: string; subtitle?: string; lines?: Requirement[] }) {
  const counted = (lines ?? []).filter((r) => r.status !== "aviso" && !searching(r));
  const ready = counted.filter((r) => r.status === "cumple").length;
  return (
    <header className="req-card-head">
      {code && <Flag code={code} />}
      <span>
        <strong>{title}</strong>
        {subtitle && <small>{subtitle}</small>}
      </span>
      {counted.length > 0 && <span className="req-count">{ready} de {counted.length} listos</span>}
    </header>
  );
}

function documentText(passenger: Passenger, countries: Country[]) {
  const document = travelDocument(passenger);
  if (!document) return "sin documento cargado";
  return `${document.type.toLowerCase()} de ${countryName(countries, document.country)}`;
}

function groupByCountry<T extends { country?: string }>(reqs: T[]): [string, T[]][] {
  const groups = new Map<string, T[]>();
  for (const r of reqs) {
    const code = r.country ?? "";
    groups.set(code, [...(groups.get(code) ?? []), r]);
  }
  return [...groups.entries()];
}

/** Lo que falta del detalle oficial se dice una vez por país, no en cada renglón. */
function OfficialNote({ details }: { details: (RequirementDetail | undefined)[] }) {
  if (details.some((d) => d?.oficial === "buscando")) {
    return <p className="req-note">Buscando el detalle del trámite en páginas oficiales…</p>;
  }
  if (details.some((d) => d?.oficial === "sin_datos")) {
    return <p className="req-note">No se encontró el detalle del trámite en páginas oficiales.</p>;
  }
  return null;
}

/**
 * Antes de cargar pasajeros: lo que piden los países a un pasaporte argentino.
 * La misma forma, sin estado, porque no hay documento contra el cual medir;
 * solo marca lo que no se sabe.
 */
function RequirementsPreview({ lines, countries }: { lines: PreviewLine[]; countries: Country[] }) {
  return (
    <>
      <p className="panel-intro">
        <strong>Requisitos para un pasajero con pasaporte argentino</strong>
        Todavía no hay pasajeros. Cuando los cargues, se evalúa a cada uno con su documento.
      </p>
      {groupByCountry(lines).map(([code, own]) => {
        const name = countryName(countries, code);
        return (
          <section key={code} className="panel-card req-card" aria-label={name}>
            <CardHead code={code} title={`${name}${own[0].role === "escala" ? " · escala" : ""}`} />
            {own.some((line) => line.searching) ? (
              <p className="req-note">Buscando los requisitos de {name}…</p>
            ) : (
              <>
                <OfficialNote details={own.map((line) => line.detail)} />
                <ul className="req-list">
                  {own.map((line, i) => (
                    <RequirementRow key={i} requirement={{
                      key: line.key, text: line.text, status: line.verify ? "verificar" : "cumple", confirmable: false,
                      detail: { pide: line.detail?.pide, tramite: line.detail?.tramite }, sources: line.sources,
                    }} preview />
                  ))}
                </ul>
              </>
            )}
          </section>
        );
      })}
    </>
  );
}

/** El nombre de una fuente: el de las conocidas, o el dominio de la página oficial. */
function sourceLabel(source: Citation): string {
  if (source.source === "oficial" && source.url) {
    try {
      const host = new URL(source.url).hostname.replace(/^www\./, "");
      return source.readAt ? `${host} · ${dayFromISO(source.readAt)}` : host;
    } catch {
      return "página oficial";
    }
  }
  return SOURCE_LABEL[source.source] ?? source.source;
}

function dayFromISO(iso: string): string {
  const [y, m, d] = iso.split("-");
  return d && m && y ? `${d}/${m}/${y}` : iso;
}

/**
 * Las fuentes se ocultan por ahora (pedido del 2026-10-22): se vuelven a
 * mostrar poniendo esto en true. Siguen llegando del servidor.
 */
const SHOW_SOURCES = false;

/** Las fuentes del renglón, una vez cada una, con la cita al pasar el mouse. */
function Sources({ sources }: { sources?: Citation[] }) {
  if (!SHOW_SOURCES) return null;
  const seen = new Set<string>();
  const linked = (sources ?? []).filter((s) => {
    if (!s.url) return false;
    const key = `${s.source}|${s.url}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  if (linked.length === 0) return null;
  return (
    <small className="req-sources">
      {linked.length === 1 ? "Fuente" : "Fuentes"}:{" "}
      {linked.map((s, i) => (
        <span key={i}>
          {i > 0 && " · "}
          <a href={s.url} target="_blank" rel="noreferrer" title={s.quote ? `«${s.quote}»` : undefined}>{sourceLabel(s)}</a>
        </span>
      ))}
    </small>
  );
}

const PARTS: { key: keyof Pick<RequirementDetail, "pide" | "tramite" | "tiene" | "hacer">; label: string }[] = [
  { key: "pide", label: "Pide" },
  { key: "tramite", label: "Trámite" },
  { key: "tiene", label: "Tiene" },
  { key: "hacer", label: "Hacer" },
];

const texts = (detail: RequirementDetail | undefined, key: (typeof PARTS)[number]["key"]) =>
  ((detail?.[key] ?? []) as (DetailLine | string)[]).map((item) => (typeof item === "string" ? item : item.text));

/** Pide, Trámite, Tiene y Hacer, con un rótulo chico a la izquierda. Una parte vacía no se muestra. */
function DetailParts({ detail, only }: { detail?: RequirementDetail; only?: (typeof PARTS)[number]["key"][] }) {
  const parts = PARTS
    .filter(({ key }) => !only || only.includes(key))
    .map(({ key, label }) => ({ label, lines: only ? texts(detail, key).slice(0, 1) : texts(detail, key) }))
    .filter((part) => part.lines.length > 0);
  if (parts.length === 0) return null;
  return (
    <dl className={only ? "req-parts is-summary" : "req-parts"}>
      {parts.map((part) => (
        <div key={part.label}>
          <dt>{part.label}</dt>
          <dd>{part.lines.map((text, i) => <span key={i}>{text}</span>)}</dd>
        </div>
      ))}
    </dl>
  );
}

const ICONS: Record<Requirement["status"], typeof Check> = { cumple: Check, falta: X, verificar: Check, aviso: Info };

/**
 * Un requisito: ícono y etiqueta de estado, «Pide» y «Tiene» en una línea cada
 * uno, y la acción al lado del problema. La flecha despliega el texto completo
 * y las fuentes.
 */
function RequirementRow({ requirement, onConfirm, onAddPermit, preview }: {
  requirement: Requirement;
  onConfirm?: (requirement: Requirement) => void;
  onAddPermit?: () => void;
  preview?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const status = requirement.status;
  const confirmable = Boolean(onConfirm) && requirement.confirmable && status !== "cumple" && !searching(requirement);
  const expandable = PARTS.some(({ key }) => texts(requirement.detail, key).length > 0) || (SHOW_SOURCES && (requirement.sources ?? []).some((s) => s.url));
  const Icon = ICONS[status];
  const tone = preview ? (status === "verificar" ? "is-verificar" : "is-info") : `is-${status}`;

  return (
    <li className={`req-row requirement--${status}${open ? " is-open" : ""}`} aria-label={requirement.text}>
      <span className={`req-icon ${tone}`} aria-hidden>
        {status === "verificar" ? "!" : preview ? <Info /> : <Icon />}
      </span>
      <div className="req-main">
        <div className="req-top">
          <strong>{requirement.text}</strong>
          {(!preview || status === "verificar") && <span className={`req-pill ${tone}`}>{STATUS_LABEL[status]}</span>}
        </div>
        {open ? (
          <div className="req-detail">
            <DetailParts detail={requirement.detail} />
            <Sources sources={requirement.sources} />
          </div>
        ) : (
          <DetailParts detail={requirement.detail} only={["pide", "tiene"]} />
        )}
        {requirement.confirmed && (
          <small className="req-confirmed">Confirmado{requirement.confirmedByName ? ` por ${requirement.confirmedByName}` : ""}{requirement.confirmedAt ? ` · ${shortInstant(requirement.confirmedAt)}` : ""}</small>
        )}
        {(onAddPermit || confirmable) && (
          <div className="req-actions">
            {onAddPermit && <Button size="sm" variant="outline" onClick={onAddPermit}>Cargar visa</Button>}
            {confirmable && (
              <Button size="sm" variant="outline" aria-label={`Confirmar: ${requirement.text}`} onClick={() => onConfirm?.(requirement)}>
                Confirmar
              </Button>
            )}
          </div>
        )}
      </div>
      {expandable && (
        <button type="button" className="req-chevron" aria-expanded={open} aria-label={open ? `Plegar ${requirement.text}` : `Ver detalle de ${requirement.text}`}
          onClick={() => setOpen((value) => !value)}>
          <ChevronRight aria-hidden />
        </button>
      )}
    </li>
  );
}
