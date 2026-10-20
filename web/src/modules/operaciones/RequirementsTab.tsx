"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { countryName } from "./countries";
import { shortInstant } from "./format";
import type { Citation, Country, PreviewLine, Requirement, TripDetail } from "./types";

const STATUS_LABEL: Record<Requirement["status"], string> = {
  cumple: "Cumple",
  falta: "Falta",
  verificar: "Verificar",
};

const SOURCE_LABEL: Record<string, string> = {
  wikipedia: "Wikipedia",
  passport_index: "Passport Index",
  constante: "norma oficial",
};

/** Vigencia y antigüedad son del control del documento, no del de requisitos. */
const documentControl = (key: string) => key === "vigencia" || key === "antiguedad";

const searching = (requirement: Requirement) =>
  requirement.status === "verificar" && requirement.text.includes("buscando requisitos");

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

  return (
    <section className="detail-section requirements">
      <h3>Requisitos de viaje</h3>
      {error && <p className="trip-form-error" role="alert">{error}</p>}
      {pendingVerify && (
        <p className="trip-note">
          Lo que está en «verificar» lo resuelve la agencia: hasta confirmarlo, el aviso no cierra «Requisitos migratorios».
        </p>
      )}
      {tripLevel.length > 0 && <ul className="requirement-list">{tripLevel.map((r, i) => <RequirementLine key={i} requirement={r} onConfirm={confirm} canConfirm={Boolean(onConfirm)} />)}</ul>}
      {trip.passengers.map((passenger) => {
        const own = reqs.filter((r) => r.passengerId === passenger.id);
        if (own.length === 0) return null;
        const name = `${passenger.firstName} ${passenger.lastName}`;
        const general = own.filter((r) => !r.country);
        const byCountry = groupByCountry(own.filter((r) => r.country));
        return (
          <section key={passenger.id} className="requirement-passenger" aria-label={name}>
            <h4>{name}</h4>
            {general.length > 0 && <ul className="requirement-list">{general.map((r, i) => <RequirementLine key={i} requirement={r} onConfirm={confirm} canConfirm={Boolean(onConfirm)} />)}</ul>}
            {byCountry.map(([code, lines]) => {
              const label = `${countryName(countries, code)}${lines[0].role === "escala" ? " · escala" : ""}`;
              return (
                <div key={code} className="requirement-country">
                  <h5>{label}</h5>
                  {lines.every(searching) ? (
                    <p className="trip-note">Buscando los requisitos de {countryName(countries, code)}…</p>
                  ) : (
                    <ul className="requirement-list">
                      {lines.map((r, i) => <RequirementLine key={i} requirement={r} onConfirm={confirm} canConfirm={Boolean(onConfirm)} />)}
                    </ul>
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
              <ul className="requirement-list">
                {own.map((line, i) => (
                  <li key={i} className={`requirement${line.verify ? " requirement--verificar" : " requirement--info"}`}>
                    {line.verify && <span className="requirement-status">Verificar</span>}
                    <span className="requirement-body">
                      <span>{line.text}</span>
                      <SourceLink sources={line.sources} />
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        );
      })}
    </section>
  );
}

function SourceLink({ sources }: { sources?: Citation[] }) {
  const source = sources?.find((s) => s.url);
  if (!source) return null;
  return (
    <small>
      <a href={source.url} target="_blank" rel="noreferrer">Fuente: {SOURCE_LABEL[source.source] ?? source.source}</a>
    </small>
  );
}

function RequirementLine({ requirement, onConfirm, canConfirm }: {
  requirement: Requirement;
  onConfirm: (requirement: Requirement) => void;
  canConfirm: boolean;
}) {
  const confirmable = canConfirm && requirement.confirmable && requirement.status !== "cumple" && !searching(requirement);
  return (
    <li className={`requirement requirement--${requirement.status}`}>
      <span className="requirement-status">{STATUS_LABEL[requirement.status]}</span>
      <span className="requirement-body">
        <span>{requirement.text}</span>
        {requirement.confirmed && (
          <small>Confirmado{requirement.confirmedByName ? ` por ${requirement.confirmedByName}` : ""}{requirement.confirmedAt ? ` · ${shortInstant(requirement.confirmedAt)}` : ""}</small>
        )}
        <SourceLink sources={requirement.sources} />
      </span>
      {confirmable && (
        <Button size="sm" variant="outline" aria-label={`Confirmar: ${requirement.text}`} onClick={() => onConfirm(requirement)}>
          Confirmar
        </Button>
      )}
    </li>
  );
}
