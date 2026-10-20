"use client";

import type { Country } from "./types";

/** El nombre en español de un país. Sin catálogo todavía, el código. */
export const countryName = (countries: Country[], code?: string) =>
  (code && countries.find((country) => country.code === code)?.name) || code || "";

/**
 * Los países se eligen por nombre, nunca por código. Si el valor guardado no
 * está en el catálogo (todavía no cargó), se muestra igual: el selector no
 * puede mentir diciendo que no hay nada elegido.
 */
export function CountrySelect({ label, value, countries, onChange, emptyLabel = "Elegí un país", required }: {
  label: string;
  value: string;
  countries: Country[];
  onChange: (code: string) => void;
  emptyLabel?: string;
  required?: boolean;
}) {
  const options = value && !countries.some((country) => country.code === value)
    ? [{ code: value, name: value }, ...countries]
    : countries;
  return (
    <select aria-label={label} value={value} required={required} onChange={(event) => onChange(event.target.value)}>
      <option value="">{emptyLabel}</option>
      {options.map((country) => <option key={country.code} value={country.code}>{country.name}</option>)}
    </select>
  );
}
