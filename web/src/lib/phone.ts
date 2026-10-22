/**
 * Un teléfono como se escribe en Argentina: «+54 9 11 2612-6437». Se hace a
 * mano y solo para números argentinos: los demás se muestran con «+» y los
 * dígitos tal cual. No vale una librería entera para eso.
 */

// Las características de tres dígitos. Buenos Aires es 11; el resto, cuatro.
const AREA_3 = new Set([
  "220", "221", "223", "230", "236", "237", "249", "260", "261", "263", "264", "266",
  "280", "291", "294", "297", "298", "299", "336", "341", "342", "343", "345", "348",
  "351", "353", "358", "362", "364", "370", "376", "379", "380", "381", "383", "385",
  "387", "388",
]);

export function formatPhone(raw: string): string {
  const digits = raw.replace(/\D/g, "");
  if (!digits) return raw;
  if (!digits.startsWith("54")) return `+${digits}`;

  let national = digits.slice(2);
  const mobile = national.startsWith("9");
  if (mobile) national = national.slice(1);
  if (national.length !== 10) return `+${digits}`;

  const areaLength = national.startsWith("11") ? 2 : AREA_3.has(national.slice(0, 3)) ? 3 : 4;
  const area = national.slice(0, areaLength);
  const local = national.slice(areaLength);
  const split = local.length - 4;
  return `+54 ${mobile ? "9 " : ""}${area} ${local.slice(0, split)}-${local.slice(split)}`;
}

/** El nombre es el número: WhatsApp no tiene otro. */
export const isPhoneLike = (value: string) => /^\+?[\d\s-]{8,}$/.test(value.trim());

/** Un nombre que puede ser un número, listo para mostrar. */
export const displayName = (name: string) => (isPhoneLike(name) ? formatPhone(name) : name);

/**
 * Un teléfono escrito a mano («11 4788-2210») se muestra como se cargó: pasarlo
 * a internacional inventaría la característica del país. Solo se le da formato
 * si ya viene internacional, como los que llegan de WhatsApp.
 */
export function formatTypedPhone(raw: string): string {
  const text = raw.trim();
  const digits = text.replace(/\D/g, "");
  return text.startsWith("+") || (digits.startsWith("54") && digits.length >= 12) ? formatPhone(text) : text;
}
