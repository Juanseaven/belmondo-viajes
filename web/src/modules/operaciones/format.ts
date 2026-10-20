/**
 * Formatos de la ficha. Las fechas del viaje son días de calendario, no
 * instantes: el backend las manda como "2026-09-12" o "2026-09-12T00:00:00Z", y
 * leerlas como UTC las correría un día hacia atrás en Argentina. Por eso se toma
 * solo la parte de la fecha y se la ubica al mediodía local.
 */

const shortFormat = new Intl.DateTimeFormat("es-AR", { day: "numeric", month: "short" });

/** El día de calendario de una fecha del viaje, como lo escribe un input date. */
export const dayOf = (value?: string) => value?.slice(0, 10) ?? "";

const calendarDay = (value: string) => new Date(`${dayOf(value)}T12:00:00`);

/** «14 sep»: para plazos y fechas del viaje. */
export const shortDate = (value: string) => shortFormat.format(calendarDay(value)).replace(".", "");

/** «3 oct» para un instante (cuándo se cerró algo), en la hora local. */
export const shortInstant = (value: string) => shortFormat.format(new Date(value)).replace(".", "");

export const money = (value: number) => new Intl.NumberFormat("es-AR").format(value);

/** El día de hoy como lo escribe un input date, para comparar plazos. */
export const today = () => {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
};

/**
 * Lee un monto como lo tipea alguien en Argentina: «1450,50» y «4.641.600» son
 * válidos, y «1450.5» también. Con coma, los puntos son de miles; sin coma, un
 * solo punto es decimal y varios son de miles. Vacío es undefined; ilegible es
 * NaN, para que quien llama pueda decir qué campo está mal.
 */
export function parseAmount(raw: string): number | undefined {
  const value = raw.trim().replace(/\s/g, "");
  if (!value) return undefined;
  let normalized = value;
  if (value.includes(",")) normalized = value.replace(/\./g, "").replace(",", ".");
  else if ((value.match(/\./g) ?? []).length > 1) normalized = value.replace(/\./g, "");
  return /^-?\d+(\.\d+)?$/.test(normalized) ? Number(normalized) : Number.NaN;
}
