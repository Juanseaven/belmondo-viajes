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

	// Derives dice qué dato cumple el control. En un derivado es la única forma
	// de cerrarlo; en uno por acción es un atajo: si el dato ya está, no hay a
	// quién avisar.
	Derives DerivedField

	// ActionLabel y Message son el botón y el texto que precarga, cuando el
	// cierre es por acción. `{pax}` se reemplaza por el nombre del titular.
	ActionLabel string
	Message     string

	// Prompt es la pregunta que responde un control manual. La planilla decía
	// «Régimen de comidas», que hay que interpretar; acá se pregunta contra qué
	// comparar, para que nadie quede mirando un renglón sin saber qué hacer.
	Prompt string

	// Task es el renglón dicho como orden, para la lista de pendientes: «Agendar
	// el check in del vuelo de ida». El título describe el dato que se verifica
	// y la etiqueta del botón asume que se está mirando el viaje; leídos sueltos
	// en una lista, ninguno de los dos dice qué hacer. Solo la tienen los
	// renglones con plazo, que son los únicos que llegan a pendientes.
	Task string

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
//
// Ningún plazo queda antes del alta: un viaje vendido con la salida encima
// tendría todos sus controles atrasados desde antes de existir. El que cae
// antes pasa al día del alta.
func (i ChecklistItem) DueAt(alta, salida, regreso time.Time) (time.Time, bool) {
	due, ok := i.rawDueAt(alta, salida, regreso)
	if ok && due.Before(alta) {
		due = alta
	}
	return due, ok
}

func (i ChecklistItem) rawDueAt(alta, salida, regreso time.Time) (time.Time, bool) {
	switch i.base {
	case desdeAlta:
		return alta.AddDate(0, 0, i.days), true
	case antesDeSalida:
		if salida.IsZero() {
			return time.Time{}, false
		}
		return salida.AddDate(0, 0, -i.days), true
	case antesDeRegreso:
		if regreso.IsZero() {
			return time.Time{}, false
		}
		return regreso.AddDate(0, 0, -i.days), true
	default:
		return time.Time{}, false
	}
}

// HasDeadline dice si el renglón vence. Los derivados no: se cumplen solos.
func (i ChecklistItem) HasDeadline() bool { return i.base != sinVencimiento }

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
		Task:    "Ofrecer preexistencias",
		Closing: ClosingAccion, base: desdeAlta, days: 2,
		ActionLabel: "Ofrecer preexistencias",
		Message:     "{pax}, te consulto si alguno de los pasajeros tiene una condición médica preexistente. Podemos sumar una cobertura que la contemple.",
	},
	{
		ID: "carga-comprobante", Stage: StageCarga, Title: "Comprobante de pago y de carga en el portal, si es con TJ",
		Task:    "Pedir el comprobante de pago",
		Closing: ClosingAccion, base: desdeAlta, days: 3,
		ActionLabel: "Pedir el comprobante",
		Message:     "{pax}, para dejar el pago asentado necesitamos el comprobante de la tarjeta. ¿Nos lo pasás por acá?",
	},
	{
		ID: "carga-vouchers", Stage: StageCarga, Title: "Vouchers y términos y condiciones enviados al pax",
		Task:    "Enviar vouchers y T&C",
		Closing: ClosingAccion, base: antesDeSalida, days: 7,
		ActionLabel: "Enviar vouchers y T&C",
		Message:     "{pax}, te envío los vouchers del viaje junto con los términos y condiciones para que los revises.",
	},
	{
		ID: "carga-acepto", Stage: StageCarga, Title: "El pax aceptó los términos y condiciones",
		Task:    "Pedir que acepte los T&C",
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
		Task:    "Revisar el orden de nombres y apellidos",
		Closing: ClosingManual, Critical: true, base: antesDeSalida, days: 30,
		Prompt: "¿El orden de nombres y apellidos coincide con el documento?",
	},
	{
		ID: "control-fechas-vuelo", Stage: StageControl, Title: "Fechas de vuelo",
		Task:    "Revisar las fechas de vuelo",
		Closing: ClosingManual, Critical: true, base: antesDeSalida, days: 30,
		Prompt: "¿Las fechas de vuelo son las que cotizaste?",
	},
	{
		ID: "control-destino", Stage: StageControl, Title: "Destino",
		Task:    "Revisar el destino de la reserva",
		Closing: ClosingManual, Critical: true, base: antesDeSalida, days: 30,
		Prompt: "¿El destino de la reserva es el que vendiste?",
	},
	{
		ID: "control-equipaje", Stage: StageControl, Title: "Equipaje incluido",
		Task:    "Revisar el equipaje incluido",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿El equipaje incluido es el que cotizaste?",
	},
	{
		// La planilla decía «Aviso al pax si requiere visa o vacuna». Ahora el
		// sistema sabe qué requiere: se cierra solo cuando todo cumple, y el
		// aviso lleva lo que falta de cada pasajero (el mensaje de acá es el de
		// respaldo; la ficha arma el real con los requisitos evaluados).
		ID: "control-visa", Stage: StageControl, Title: "Requisitos migratorios",
		Task:    "Avisar los requisitos migratorios",
		Closing: ClosingAccion, Derives: DerivaRequisitos, Critical: true, base: antesDeSalida, days: 45,
		ActionLabel: "Avisar los requisitos",
		Message:     "{pax}, te confirmo los requisitos migratorios del destino para que los tengas presentes antes de viajar.",
	},
	{
		ID: "control-hotel-liquidacion", Stage: StageControl, Title: "El hotel reservado es el indicado en la liquidación",
		Task:    "Revisar que el hotel sea el de la liquidación",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿El hotel reservado es el de la liquidación?",
	},
	{
		ID: "control-hotel-datos", Stage: StageControl, Title: "Hotel, fechas de reserva y nombre del titular",
		Task:    "Revisar hotel, fechas y titular de la reserva",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿El hotel, las fechas y el titular de la reserva son correctos?",
	},
	{
		ID: "control-comidas", Stage: StageControl, Title: "Régimen de comidas",
		Task:    "Revisar el régimen de comidas",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿El régimen de comidas reservado es el que cotizaste?",
	},
	{
		ID: "control-traslados", Stage: StageControl, Title: "Traslados: números de vuelo y horarios informados",
		Task:    "Informar vuelos y horarios de los traslados",
		Closing: ClosingManual, base: antesDeSalida, days: 15,
		Prompt: "¿Le informaste al pasajero los vuelos y horarios de los traslados?",
	},
	{
		ID: "control-vto-alojamiento", Stage: StageControl, Title: "Vencimiento del alojamiento",
		Task:    "Registrar el vencimiento del alojamiento",
		Closing: ClosingManual, base: antesDeSalida, days: 30,
		Prompt: "¿Registraste hasta cuándo se puede cancelar el alojamiento?",
	},
	{
		ID: "control-checkin", Stage: StageControl, Title: "Se agendó el check in del vuelo",
		Task:    "Agendar el check in del vuelo de ida",
		Closing: ClosingManual, base: antesDeSalida, days: 2,
		Prompt: "¿Agendaste el check in del vuelo de ida?",
	},
	{
		ID: "control-checkout", Stage: StageControl, Title: "Se agendó el check out del vuelo",
		Task:    "Agendar el check in del vuelo de vuelta",
		Closing: ClosingManual, base: antesDeRegreso, days: 2,
		Prompt: "¿Agendaste el check in del vuelo de vuelta?",
	},
	{
		ID: "control-vto-doc", Stage: StageControl, Title: "Vencimiento de DNI o pasaporte",
		Task: "Revisar el vencimiento de los documentos",
		// Se cierra solo si todos los documentos llegan al regreso. Si falta un
		// vencimiento o alguno no llega, queda abierto hasta cargarlo o avisar.
		Closing: ClosingAccion, Derives: DerivaDocumentosVigentes,
		Critical: true, base: antesDeSalida, days: 60,
		ActionLabel: "Avisar al pasajero",
		Message:     "{pax}, un aviso importante: hay un documento que vence antes del regreso del viaje y hay que renovarlo. ¿Querés que te pase los turnos disponibles?",
	},
	{
		ID: "control-flight-plus", Stage: StageControl, Title: "Flight Plus emitida y enviada al pasajero",
		Task:    "Enviar la Flight Plus",
		Closing: ClosingAccion, base: antesDeSalida, days: 15,
		ActionLabel: "Enviar Flight Plus",
		Message:     "{pax}, te envío la Flight Plus emitida para el viaje.",
	},
	{
		ID: "control-asistencia", Stage: StageControl, Title: "Asistencia médica emitida",
		Task:    "Emitir la asistencia médica",
		Closing: ClosingManual, base: antesDeSalida, days: 15,
		Prompt: "¿La asistencia médica está emitida?",
	},
	{
		ID: "control-no-reembolsable", Stage: StageControl, Title: "Monto no reembolsable informado por mail",
		Task:    "Informar el monto no reembolsable",
		Closing: ClosingAccion, Critical: true, base: antesDeSalida, days: 30,
		ActionLabel: "Informar el monto",
		Message:     "{pax}, te confirmo por este medio cuál es el monto no reembolsable en caso de cancelación.",
	},
}
