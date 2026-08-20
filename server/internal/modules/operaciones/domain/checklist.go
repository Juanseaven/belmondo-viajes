package domain

import "time"

// ChecklistItem es un renglón de las planillas de la agencia.
//
// La plantilla es una constante y no una tabla a propósito: es el proceso de la
// empresa, no un dato de cada viaje. Cada viaje se lleva su copia al nacer, de
// modo que cambiar el proceso no reescribe la historia de los viajes ya
// abiertos. Y nada que administrar es nada que aprender.
type ChecklistItem struct {
	ID    string
	Stage Stage
	// Title es lo que la planilla decía. Se conserva para que quien viene del
	// papel reconozca el renglón de un vistazo.
	Title   string
	Closing Closing
	// Critical no es «importante»: es lo que, si falla, arruina el viaje o deja a
	// la agencia expuesta. Es lo que el chip de la bandeja muestra primero.
	Critical bool

	// Derives dice qué dato cumple el control, cuando el cierre es derivado.
	Derives DerivedField

	// ActionLabel y Message son el botón y el texto que precarga, cuando el
	// cierre es por acción. `{pax}` se reemplaza por el nombre del titular.
	ActionLabel string
	Message     string

	// Prompt es la pregunta que responde un control manual. La planilla decía
	// «Régimen de comidas», que hay que interpretar; acá se pregunta contra qué
	// comparar, para que nadie quede mirando un renglón sin saber qué hacer.
	Prompt string

	base deadlineBase
	days int
}

type deadlineBase string

const (
	sinVencimiento deadlineBase = ""
	desdeAlta      deadlineBase = "alta"
	antesDeSalida  deadlineBase = "salida"
	antesDeRegreso deadlineBase = "regreso"
)

// DueAt calcula cuándo vence el renglón para un viaje concreto.
//
// El plazo es un dato de la plantilla y no una fecha que alguien tipea: por eso
// mover la fecha de salida corre todos los vencimientos sin que nadie toque
// nada. Un control derivado no vence —no hay a quién apurar— y devuelve false.
func (i ChecklistItem) DueAt(alta, salida, regreso time.Time) (time.Time, bool) {
	switch i.base {
	case desdeAlta:
		return alta.AddDate(0, 0, i.days), true
	case antesDeSalida:
		return salida.AddDate(0, 0, -i.days), true
	case antesDeRegreso:
		return regreso.AddDate(0, 0, -i.days), true
	default:
		return time.Time{}, false
	}
}

// Checklist devuelve las dos planillas transcritas, en el orden en que se
// llenan.
func Checklist() []ChecklistItem {
	items := make([]ChecklistItem, 0, len(carga)+len(control))
	items = append(items, carga...)
	items = append(items, control...)
	return items
}

// Hoja de carga de reserva. La llena quien vende, al cerrar.
//
// Diez de sus catorce renglones son campos disfrazados de casillero: el sistema
// tiene el dato o no lo tiene, y tildarlo no agrega información.
var carga = []ChecklistItem{
	{
		ID: "carga-nombres", Stage: StageCarga, Title: "Nombres completos",
		Closing: ClosingDerivado, Derives: DerivaPasajeroNombre,
	},
	{
		ID: "carga-apellidos", Stage: StageCarga, Title: "Apellidos",
		Closing: ClosingDerivado, Derives: DerivaPasajeroApellido,
	},
	{
		ID: "carga-destino", Stage: StageCarga, Title: "Destino",
		Closing: ClosingDerivado, Derives: DerivaDestino,
	},
	{
		ID: "carga-salida", Stage: StageCarga, Title: "Fecha de salida",
		Closing: ClosingDerivado, Derives: DerivaSalida,
	},
	{
		ID: "carga-llegada", Stage: StageCarga, Title: "Fecha de llegada",
		Closing: ClosingDerivado, Derives: DerivaRegreso,
	},
	{
		ID: "carga-contacto", Stage: StageCarga, Title: "Tel y mail del pax cargados en la reserva",
		Closing: ClosingDerivado, Derives: DerivaClienteContacto,
	},
	{
		ID: "carga-usd", Stage: StageCarga, Title: "Valor en USD",
		Closing: ClosingDerivado, Derives: DerivaTotalUsd,
	},
	{
		ID: "carga-pesos", Stage: StageCarga, Title: "Valor en pesos",
		Closing: ClosingDerivado, Derives: DerivaTotalArs,
	},
	{
		ID: "carga-cambio", Stage: StageCarga, Title: "Cambio del día",
		Closing: ClosingDerivado, Derives: DerivaCambio,
	},
	{
		ID: "carga-metodo", Stage: StageCarga, Title: "Método de pago",
		Closing: ClosingDerivado, Derives: DerivaMetodoPago,
	},
	{
		ID: "carga-preexistencia", Stage: StageCarga, Title: "Se ofreció preexistencia",
		Closing: ClosingAccion, base: desdeAlta, days: 2,
		ActionLabel: "Ofrecer preexistencias",
		Message:     "{pax}, te consulto si alguno de los pasajeros tiene una condición médica preexistente. Podemos sumar una cobertura que la contemple.",
	},
	{
		ID: "carga-comprobante", Stage: StageCarga, Title: "Comprobante de pago y de carga en el portal, si es con TJ",
		Closing: ClosingAccion, base: desdeAlta, days: 3,
		ActionLabel: "Pedir el comprobante",
		Message:     "{pax}, para dejar el pago asentado necesitamos el comprobante de la tarjeta. ¿Nos lo pasás por acá?",
	},
	{
		ID: "carga-vouchers", Stage: StageCarga, Title: "Vouchers y términos y condiciones enviados al pax",
		Closing: ClosingAccion, base: antesDeSalida, days: 7,
		ActionLabel: "Enviar vouchers y T&C",
		Message:     "{pax}, te envío los vouchers del viaje junto con los términos y condiciones para que los revises.",
	},
	{
		ID: "carga-acepto", Stage: StageCarga, Title: "El pax aceptó los términos y condiciones",
		Closing: ClosingAccion, Critical: true, base: desdeAlta, days: 5,
		ActionLabel: "Pedir la aceptación",
		Message:     "{pax}, cuando puedas confirmanos por este medio que leíste y aceptás los términos y condiciones.",
	},
}

// Hoja de control de reserva. Se audita después de la carga.
//
// Sus renglones manuales lo son con razón: comparan nuestro registro contra el
// del operador. Eso el sistema no lo puede saber.
var control = []ChecklistItem{
	{
		ID: "control-titular", Stage: StageControl, Title: "Nombre y apellido del titular de la reserva",
		Closing: ClosingDerivado, Derives: DerivaTitular,
	},
	{
		ID: "control-orden", Stage: StageControl, Title: "Orden de los nombres y apellidos",
		Closing: ClosingManual, Critical: true, base: antesDeSalida, days: 30,
		Prompt: "¿El orden de nombres y apellidos coincide con el documento?",
	},
	{
		ID: "control-fechas-vuelo", Stage: StageControl, Title: "Fechas de vuelo",
		Closing: ClosingManual, Critical: true, base: antesDeSalida, days: 30,
		Prompt: "¿Las fechas de vuelo son las que cotizaste?",
	},
	{
		ID: "control-destino", Stage: StageControl, Title: "Destino",
		Closing: ClosingManual, Critical: true, base: antesDeSalida, days: 30,
		Prompt: "¿El destino de la reserva es el que vendiste?",
	},
	{
		ID: "control-equipaje", Stage: StageControl, Title: "Equipaje incluido",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿El equipaje incluido es el que cotizaste?",
	},
	{
		ID: "control-visa", Stage: StageControl, Title: "Aviso al pax si requiere visa o vacuna",
		Closing: ClosingAccion, Critical: true, base: antesDeSalida, days: 45,
		ActionLabel: "Avisar por visa o vacuna",
		Message:     "{pax}, te confirmo los requisitos migratorios del destino para que los tengas presentes antes de viajar.",
	},
	{
		ID: "control-hotel-liquidacion", Stage: StageControl, Title: "El hotel reservado es el indicado en la liquidación",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿El hotel reservado es el de la liquidación?",
	},
	{
		ID: "control-hotel-datos", Stage: StageControl, Title: "Hotel, fechas de reserva y nombre del titular",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿El hotel, las fechas y el titular de la reserva son correctos?",
	},
	{
		ID: "control-comidas", Stage: StageControl, Title: "Régimen de comidas",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿El régimen de comidas reservado es el que cotizaste?",
	},
	{
		ID: "control-traslados", Stage: StageControl, Title: "Traslados: números de vuelo y horarios informados",
		Closing: ClosingManual, base: antesDeSalida, days: 15,
		Prompt: "¿Le informaste al pasajero los vuelos y horarios de los traslados?",
	},
	{
		ID: "control-vto-alojamiento", Stage: StageControl, Title: "Vencimiento del alojamiento",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿Registraste hasta cuándo se puede cancelar el alojamiento?",
	},
	{
		ID: "control-checkin", Stage: StageControl, Title: "Se agendó el check in del vuelo",
		Closing: ClosingManual, base: antesDeSalida, days: 2,
		Prompt: "¿Agendaste el check in del vuelo de ida?",
	},
	{
		ID: "control-checkout", Stage: StageControl, Title: "Se agendó el check out del vuelo",
		Closing: ClosingManual, base: antesDeRegreso, days: 2,
		Prompt: "¿Agendaste el check in del vuelo de vuelta?",
	},
	{
		ID: "control-vto-doc", Stage: StageControl, Title: "Vencimiento de DNI o pasaporte",
		Closing: ClosingAccion, Critical: true, base: antesDeSalida, days: 60,
		ActionLabel: "Avisar al pasajero",
		Message:     "{pax}, un aviso importante: hay un documento que vence antes del regreso del viaje y hay que renovarlo. ¿Querés que te pase los turnos disponibles?",
	},
	{
		ID: "control-flight-plus", Stage: StageControl, Title: "Flight Plus emitida y enviada al pasajero",
		Closing: ClosingAccion, base: antesDeSalida, days: 15,
		ActionLabel: "Enviar Flight Plus",
		Message:     "{pax}, te envío la Flight Plus emitida para el viaje.",
	},
	{
		ID: "control-asistencia", Stage: StageControl, Title: "Asistencia médica emitida",
		Closing: ClosingManual, base: antesDeSalida, days: 15,
		Prompt: "¿La asistencia médica está emitida?",
	},
	{
		ID: "control-no-reembolsable", Stage: StageControl, Title: "Monto no reembolsable informado por mail",
		Closing: ClosingAccion, Critical: true, base: antesDeSalida, days: 30,
		ActionLabel: "Informar el monto",
		Message:     "{pax}, te confirmo por este medio cuál es el monto no reembolsable en caso de cancelación.",
	},
}
