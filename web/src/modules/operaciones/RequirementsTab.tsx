"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { countryName } from "./countries";
import { shortInstant } from "./format";
import type { Citation, Country, DetailLine, PreviewLine, Requirement, RequirementDetail, TripDetail } from "./types";

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
 * Lo que cada pasajero necesita para entrar a cada país del itinerario, dicho
 * con palabras y con la fuente a mano. El panel no dice «podés viajar»: dice qué
 * encontró y de dónde.
 */
export function RequirementsTab({ trip, countries, onConfirm }: {
  trip: TripDetail;
  countries: Country[];
  onConfirm?: (requirement: Requirement) => Promise<void> | void;
}) {
  const [error, setError] = useState<string>();
  const reqs = trip.requirements ?? [];
  const preview = trip.passengers.length === 0 ? trip.preview ?? [] : [];

  if (preview.length > 0) return <RequirementsPreview lines={preview} countries={countries} />;

  if (reqs.length === 0) {
    return (
      <section className="detail-section">
        <h3>Requisitos de viaje</h3>
        <div className="detail-empty trip-empty">
          <strong>Cargá el país de destino y los pasajeros para ver los requisitos.</strong>
          <p>Con el documento de cada pasajero, el sistema busca solo qué pide cada país.</p>
        </div>
      </section>
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
  const line = (r: Requirement, i: number) => (
    <RequirementLine key={i} requirement={r} onConfirm={confirm} canConfirm={Boolean(onConfirm)} />
  );

  return (
    <section className="detail-section requirements">
      <h3>Requisitos de viaje</h3>
      {error && <p className="trip-form-error" role="alert">{error}</p>}
      {pendingVerify && (
        <p className="trip-note">
          Lo que está en «verificar» lo resuelve la agencia: hasta confirmarlo, el aviso no cierra «Requisitos migratorios».
        </p>
      )}
      {tripLevel.length > 0 && <ul className="requirement-list">{tripLevel.map(line)}</ul>}
      {trip.passengers.map((passenger) => {
        const own = reqs.filter((r) => r.passengerId === passenger.id);
        if (own.length === 0) return null;
        const name = `${passenger.firstName} ${passenger.lastName}`;
        // Las recomendaciones van primero: son del viaje entero, no de un país.
        const general = own.filter((r) => !r.country).sort((a, b) => Number(b.status === "aviso") - Number(a.status === "aviso"));
        const byCountry = groupByCountry(own.filter((r) => r.country));
        return (
          <section key={passenger.id} className="requirement-passenger" aria-label={name}>
            <h4>{name}</h4>
            {general.length > 0 && <ul className="requirement-list">{general.map(line)}</ul>}
            {byCountry.map(([code, lines]) => {
              const label = `${countryName(countries, code)}${lines[0].role === "escala" ? " · escala" : ""}`;
              return (
                <div key={code} className="requirement-country">
                  <h5>{label}</h5>
                  {lines.every(searching) ? (
                    <p className="trip-note">Buscando los requisitos de {countryName(countries, code)}…</p>
                  ) : (
                    <>
                      <OfficialNote details={lines.map((r) => r.detail)} />
                      <ul className="requirement-list">{lines.map(line)}</ul>
                    </>
                  )}
                </div>
              );
            })}
          </section>
        );
      })}
    </section>
  );
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
    return <p className="trip-note">Buscando el detalle del trámite en páginas oficiales…</p>;
  }
  if (details.some((d) => d?.oficial === "sin_datos")) {
    return <p className="trip-note">No se encontró el detalle del trámite en páginas oficiales.</p>;
  }
  return null;
}

/**
 * Antes de cargar pasajeros: lo que piden los países a un pasaporte argentino.
 * No dice cumple ni falta, porque no hay documento contra el cual medir; solo
 * marca lo que no se sabe.
 */
function RequirementsPreview({ lines, countries }: { lines: PreviewLine[]; countries: Country[] }) {
  return (
    <section className="detail-section requirements">
      <h3>Requisitos para un pasajero con pasaporte argentino</h3>
      <p className="trip-note">Todavía no hay pasajeros. Cuando los cargues, se evalúa a cada uno con su documento.</p>
      {groupByCountry(lines).map(([code, own]) => {
        const name = countryName(countries, code);
        return (
          <div key={code} className="requirement-country">
            <h5>{name}{own[0].role === "escala" ? " · escala" : ""}</h5>
            {own.some((line) => line.searching) ? (
              <p className="trip-note">Buscando los requisitos de {name}…</p>
            ) : (
              <>
                <OfficialNote details={own.map((line) => line.detail)} />
                <ul className="requirement-list">
                  {own.map((line, i) => (
                    <li key={i} className={`requirement${line.verify ? " requirement--verificar" : " requirement--info"}`}>
                      {line.verify && <span className="requirement-status">Verificar</span>}
                      <span className="requirement-body">
                        <span>{line.text}</span>
                        <DetailParts detail={{ pide: line.detail?.pide, tramite: line.detail?.tramite }} />
                        <Sources sources={line.sources} />
                      </span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </div>
        );
      })}
    </section>
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

/** Las fuentes del renglón, una vez cada una, con la cita al pasar el mouse. */
function Sources({ sources }: { sources?: Citation[] }) {
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
    <small className="requirement-sources">
      Fuentes:{" "}
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

/** Pide, Trámite, Tiene y Hacer, con una etiqueta chica a la izquierda. Una parte vacía no se muestra. */
function DetailParts({ detail }: { detail?: RequirementDetail }) {
  if (!detail) return null;
  const parts = PARTS.map(({ key, label }) => {
    const items = (detail[key] ?? []) as (DetailLine | string)[];
    return { label, texts: items.map((item) => (typeof item === "string" ? item : item.text)) };
  }).filter((part) => part.texts.length > 0);
  if (parts.length === 0) return null;
  return (
    <dl className="requirement-detail">
      {parts.map((part) => (
        <div key={part.label}>
          <dt>{part.label}</dt>
          <dd>{part.texts.map((text, i) => <span key={i}>{text}</span>)}</dd>
        </div>
      ))}
    </dl>
  );
}

function RequirementLine({ requirement, onConfirm, canConfirm }: {
  requirement: Requirement;
  onConfirm: (requirement: Requirement) => void;
  canConfirm: boolean;
}) {
  const confirmable = canConfirm && requirement.confirmable && requirement.status !== "cumple" && !searching(requirement);
  return (
    <li className={`requirement requirement--${requirement.status}`} aria-label={requirement.text}>
      <span className="requirement-status">{STATUS_LABEL[requirement.status]}</span>
      <span className="requirement-body">
        <strong className="requirement-title">{requirement.text}</strong>
        <DetailParts detail={requirement.detail} />
        {requirement.confirmed && (
          <small>Confirmado{requirement.confirmedByName ? ` por ${requirement.confirmedByName}` : ""}{requirement.confirmedAt ? ` · ${shortInstant(requirement.confirmedAt)}` : ""}</small>
        )}
        <Sources sources={requirement.sources} />
      </span>
      {confirmable && (
        <Button size="sm" variant="outline" aria-label={`Confirmar: ${requirement.text}`} onClick={() => onConfirm(requirement)}>
          Confirmar
        </Button>
      )}
    </li>
  );
}
