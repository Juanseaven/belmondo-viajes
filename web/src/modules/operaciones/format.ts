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

const weekday = new Intl.DateTimeFormat("es-AR", { weekday: "long" });

/** Días entre dos días de calendario: positivo si `value` es posterior a `from`. */
const daysBetween = (from: string, value: string) => {
  const utc = (day: string) => {
    const [y, m, d] = dayOf(day).split("-").map(Number);
    return Date.UTC(y, m - 1, d);
  };
  return Math.round((utc(value) - utc(from)) / 86_400_000);
};

/**
 * Un día dicho como lo diría alguien en la agencia: «hoy», «mañana», el día de
 * la semana si cae en los próximos seis días, y la fecha si no. Seis y no siete:
 * «el martes» dicho un martes sería ambiguo.
 */
function relativeDay(value: string, from: string) {
  const diff = daysBetween(from, value);
  if (diff === 0) return "hoy";
  if (diff === 1) return "mañana";
  if (diff > 1 && diff < 7) return `el ${weekday.format(new Date(`${dayOf(value)}T12:00:00`))}`;
  return `el ${shortDate(value)}`;
}

const weekdayShort = new Intl.DateTimeFormat("es-AR", { weekday: "short" });
const pad2 = (n: number) => String(n).padStart(2, "0");
const parts = (value: string) => dayOf(value).split("-").map(Number);
const capitalize = (text: string) => text.charAt(0).toUpperCase() + text.slice(1);

/** «29/10». */
export function dayMonth(value: string) {
  const [, m, d] = parts(value);
  return `${pad2(d)}/${pad2(m)}`;
}

/** «07/11/2026». */
export function fullDate(value: string) {
  const [y] = parts(value);
  return `${dayMonth(value)}/${y}`;
}

/** «jue»: el día de la semana corto, sin el punto que agrega Intl. */
export const weekdayOf = (value: string) => weekdayShort.format(calendarDay(value)).replace(".", "");

/** «jue 29/10/2026»: una fecha suelta siempre dice qué día de la semana es. */
export const longDay = (value: string) => `${weekdayOf(value)} ${fullDate(value)}`;

/** «jue 29/10»: lo mismo en una tabla, donde el año sobra. */
export const shortDay = (value: string) => `${weekdayOf(value)} ${dayMonth(value)}`;

/** «jue 29 oct». */
export const weekdayDate = (value: string) => `${weekdayOf(value)} ${shortDate(value)}`;

/** El lunes de la semana de un día: para saber si otro cae en la misma. */
const mondayOf = (day: string) => {
  const date = calendarDay(day);
  const offset = (date.getDay() + 6) % 7;
  date.setDate(date.getDate() - offset);
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
};

/**
 * Un plazo en palabras, como lo diría alguien en la agencia: «Hoy», «Mañana»,
 * «Viernes» si cae esta semana, «En 6 días» si es la que viene, «Hace 1 día» si
 * ya pasó.
 */
export function inWords(value: string, from = today()) {
  const diff = daysBetween(from, value);
  if (diff === 0) return "Hoy";
  if (diff === 1) return "Mañana";
  if (diff === -1) return "Ayer";
  if (diff < 0) return `Hace ${-diff} días`;
  if (mondayOf(value) === mondayOf(from)) return capitalize(weekday.format(calendarDay(value)));
  return `En ${diff} días`;
}

/** Atrasado, esta semana o más adelante: el color de un plazo. */
export type Urgency = "late" | "soon" | "later";

export function urgency(value: string, from = today()): Urgency {
  const diff = daysBetween(from, value);
  if (diff < 0) return "late";
  return diff < 7 ? "soon" : "later";
}

/** «venció el 18 oct», «vence hoy», «vence el jueves». */
export function whenDue(dueAt: string, from = today()) {
  return daysBetween(from, dueAt) < 0 ? `venció el ${shortDate(dueAt)}` : `vence ${relativeDay(dueAt, from)}`;
}

/** «sale mañana», «sale el 10 dic», «salió el 3 oct». */
export function departs(departAt: string, from = today()) {
  return daysBetween(from, departAt) < 0 ? `salió el ${shortDate(departAt)}` : `sale ${relativeDay(departAt, from)}`;
}
