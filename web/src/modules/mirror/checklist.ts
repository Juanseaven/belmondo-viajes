import type { ControlStage } from "./operations";

/**
 * Las dos planillas de la agencia, transcritas renglón por renglón.
 *
 * Es una plantilla y no datos sueltos por operación: el checklist es el mismo
 * para todos los viajes —es el proceso de la empresa—, y cada operación aporta
 * solamente el estado de cada renglón.
 *
 * `{pax}` en un mensaje se reemplaza por el nombre del titular.
 */
export interface ChecklistItem {
  id: string;
  stage: ControlStage;
  /** La redacción sigue a la hoja para que se reconozca de un vistazo. */
  title: string;
  closing: "accion" | "manual";
  actionLabel?: string;
  message?: string;
}

/** Hoja de carga de reserva. La llena quien vende, al cerrar. */
const CARGA: ChecklistItem[] = [
  { id: "carga-nombres", stage: "carga", title: "Nombres completos", closing: "manual" },
  { id: "carga-apellidos", stage: "carga", title: "Apellidos", closing: "manual" },
  { id: "carga-destino", stage: "carga", title: "Destino", closing: "manual" },
  { id: "carga-salida", stage: "carga", title: "Fecha de salida", closing: "manual" },
  { id: "carga-llegada", stage: "carga", title: "Fecha de llegada", closing: "manual" },
  {
    id: "carga-preexistencia", stage: "carga", title: "Se ofreció preexistencia",
    closing: "accion", actionLabel: "Ofrecer preexistencias",
    message: "{pax}, te consulto si alguno de los pasajeros tiene una condición médica preexistente. Podemos sumar una cobertura que la contemple.",
  },
  { id: "carga-contacto", stage: "carga", title: "Tel y mail del pax cargados en la reserva", closing: "manual" },
  { id: "carga-usd", stage: "carga", title: "Valor en USD", closing: "manual" },
  { id: "carga-pesos", stage: "carga", title: "Valor en pesos", closing: "manual" },
  { id: "carga-cambio", stage: "carga", title: "Cambio del día", closing: "manual" },
  { id: "carga-metodo", stage: "carga", title: "Método de pago", closing: "manual" },
  {
    id: "carga-comprobante", stage: "carga",
    title: "Comprobante de pago y de carga en el portal, si es con TJ",
    closing: "accion", actionLabel: "Pedir el comprobante",
    message: "{pax}, para dejar el pago asentado necesitamos el comprobante de la tarjeta. ¿Nos lo pasás por acá?",
  },
  {
    id: "carga-vouchers", stage: "carga",
    title: "Vouchers y términos y condiciones enviados al pax",
    closing: "accion", actionLabel: "Enviar vouchers y T&C",
    message: "{pax}, te envío los vouchers del viaje junto con los términos y condiciones para que los revises.",
  },
  {
    id: "carga-acepto", stage: "carga", title: "El pax aceptó los términos y condiciones",
    closing: "accion", actionLabel: "Pedir la aceptación",
    message: "{pax}, cuando puedas confirmanos por este medio que leíste y aceptás los términos y condiciones.",
  },
];

/** Hoja de control de reserva. Se audita después de la carga. */
const CONTROL: ChecklistItem[] = [
  { id: "control-titular", stage: "control", title: "Nombre y apellido del titular de la reserva", closing: "manual" },
  { id: "control-orden", stage: "control", title: "Orden de los nombres y apellidos", closing: "manual" },
  { id: "control-fechas-vuelo", stage: "control", title: "Fechas de vuelo", closing: "manual" },
  { id: "control-destino", stage: "control", title: "Destino", closing: "manual" },
  { id: "control-equipaje", stage: "control", title: "Equipaje incluido", closing: "manual" },
  {
    id: "control-visa", stage: "control", title: "Aviso al pax si requiere visa o vacuna",
    closing: "accion", actionLabel: "Avisar por visa o vacuna",
    message: "{pax}, te confirmo los requisitos migratorios del destino para que los tengas presentes antes de viajar.",
  },
  { id: "control-hotel-liquidacion", stage: "control", title: "El hotel reservado es el indicado en la liquidación", closing: "manual" },
  { id: "control-hotel-datos", stage: "control", title: "Hotel, fechas de reserva y nombre del titular", closing: "manual" },
  { id: "control-comidas", stage: "control", title: "Régimen de comidas", closing: "manual" },
  { id: "control-traslados", stage: "control", title: "Traslados: números de vuelo y horarios informados", closing: "manual" },
  { id: "control-vto-alojamiento", stage: "control", title: "Vencimiento del alojamiento", closing: "manual" },
  { id: "control-checkin", stage: "control", title: "Se agendó el check in del vuelo", closing: "manual" },
  { id: "control-checkout", stage: "control", title: "Se agendó el check out del vuelo", closing: "manual" },
  {
    id: "control-vto-doc", stage: "control", title: "Vencimiento de DNI o pasaporte",
    closing: "accion", actionLabel: "Avisar al pasajero",
    message: "{pax}, un aviso importante: hay un documento que vence antes del regreso del viaje y hay que renovarlo. ¿Querés que te pase los turnos disponibles?",
  },
  {
    id: "control-flight-plus", stage: "control", title: "Flight Plus emitida y enviada al pasajero",
    closing: "accion", actionLabel: "Enviar Flight Plus",
    message: "{pax}, te envío la Flight Plus emitida para el viaje.",
  },
  { id: "control-asistencia", stage: "control", title: "Asistencia médica emitida", closing: "manual" },
  {
    id: "control-no-reembolsable", stage: "control",
    title: "Monto no reembolsable informado por mail",
    closing: "accion", actionLabel: "Informar el monto",
    message: "{pax}, te confirmo por este medio cuál es el monto no reembolsable en caso de cancelación.",
  },
];

export const CHECKLIST_TEMPLATE: ChecklistItem[] = [...CARGA, ...CONTROL];
