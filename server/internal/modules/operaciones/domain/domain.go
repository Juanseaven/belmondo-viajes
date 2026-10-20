// Package domain define el vocabulario del módulo de operaciones. No importa
// nada del proyecto: todos los demás paquetes del módulo dependen de este y
// ninguno al revés.
package domain

// ModuleName es la identidad del módulo: prefijo de rutas (/api/operaciones/...),
// clave en schema_migrations y remitente en los eventos de websocket.
const ModuleName = "operaciones"

// Stage distingue las dos planillas de papel que este módulo reemplaza: la de
// carga, que llena quien vende al cerrar, y la de control, que se audita
// después.
type Stage string

const (
	StageCarga   Stage = "carga"
	StageControl Stage = "control"
)

// Closing es la forma en que un control deja de estar abierto, y es la decisión
// que ordena todo el diseño.
//
// Una planilla de treinta y un casilleros es teatro: alguien los tilda sin
// mirar y el error que la hoja quería evitar ocurre igual, ahora con un registro
// que afirma lo contrario. El estado de un control tiene que ser una
// consecuencia, no una declaración.
type Closing string

const (
	// ClosingDerivado: el control se cumple porque el dato existe. El sistema lo
	// cierra solo. «Destino» no es un control, es un campo: tildarlo en un viaje
	// que tiene destino es la definición del teatro.
	ClosingDerivado Closing = "derivado"

	// ClosingAccion: el control se cierra al ejecutar la acción desde el panel, y
	// queda enlazado al mensaje que lo prueba.
	ClosingAccion Closing = "accion"

	// ClosingManual: se confirma a mano, registrando quién y cuándo. Queda para
	// lo único que lo merece —comparar nuestro registro contra el del operador—,
	// que el sistema no puede saber y una persona sí.
	ClosingManual Closing = "manual"
)

// DerivedField nombra el dato cuya existencia cumple un control derivado. El
// store las traduce a consultas; la plantilla solo declara cuál corresponde.
type DerivedField string

const (
	DerivaPasajeroNombre   DerivedField = "pasajero.nombre"
	DerivaPasajeroApellido DerivedField = "pasajero.apellido"
	DerivaTitular          DerivedField = "pasajero.titular"
	DerivaDestino          DerivedField = "viaje.destino"
	DerivaSalida           DerivedField = "viaje.salida"
	DerivaRegreso          DerivedField = "viaje.regreso"
	DerivaClienteContacto  DerivedField = "cliente.mail_y_telefono"
	DerivaTotalUsd         DerivedField = "pago.total_usd"
	DerivaTotalArs         DerivedField = "pago.total_ars"
	DerivaCambio           DerivedField = "pago.cambio"
	DerivaMetodoPago       DerivedField = "pago.metodo"
	// DerivaDocumentosVigentes: todos los pasajeros tienen un documento que
	// vence después del regreso. Lo usa un control por acción, no un derivado:
	// el dato lo cierra, pero si falta, el aviso al pasajero también.
	DerivaDocumentosVigentes DerivedField = "pasajero.documentos_vigentes"
)

// ControlStatus es el estado de la copia que cada viaje guarda de un renglón.
type ControlStatus string

const (
	ControlAbierto ControlStatus = "abierto"
	ControlCerrado ControlStatus = "cerrado"
)

// TripStatus es dónde está el viaje en su vida comercial.
type TripStatus string

const (
	// TripConsulta: alguien preguntó y todavía no se le cotizó. Es como nace
	// todo viaje.
	TripConsulta   TripStatus = "consulta"
	TripCotizado   TripStatus = "cotizado"
	TripConfirmado TripStatus = "confirmado"
	TripEnViaje    TripStatus = "en viaje"
	TripCerrado    TripStatus = "cerrado"
)
