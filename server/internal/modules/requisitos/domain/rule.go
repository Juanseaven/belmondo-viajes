// Package domain define el vocabulario de las reglas de entrada: qué pide cada
// país a cada nacionalidad, de dónde salió cada dato y cómo se combinan las
// fuentes. No importa nada del proyecto.
package domain

import "time"

// ModuleName es la identidad del módulo: prefijo de rutas y clave de
// migraciones.
const ModuleName = "requisitos"

// Pair es la clave de una regla: la nacionalidad del documento con el que se
// viaja y el país al que se entra, los dos en ISO 3166-1 alfa-2.
type Pair struct {
	Nationality string `json:"nationality"`
	Country     string `json:"country"`
}

// RuleStatus dice si la regla ya tiene datos.
type RuleStatus string

const (
	// RuleBuscando: la regla nació porque un viaje la necesita y todavía no se
	// buscó. Todo cuenta como «verificar» hasta que llegue.
	RuleBuscando RuleStatus = "buscando"
	// RuleVigente: tiene datos de al menos una fuente.
	RuleVigente RuleStatus = "vigente"
	// RuleSinFuentes: las fuentes fallaron y nunca hubo datos. Todo cuenta como
	// «verificar»; se reintenta en la próxima pasada.
	RuleSinFuentes RuleStatus = "sin_fuentes"
)

// Visa es lo que un país pide para entrar, del menos al más exigente.
type Visa string

const (
	VisaNoRequiere              Visa = "no_requiere"
	VisaALaLlegada              Visa = "visa_a_la_llegada"
	VisaAutorizacionElectronica Visa = "autorizacion_electronica"
	VisaElectronica             Visa = "visa_electronica"
	VisaConsular                Visa = "visa_consular"
	VisaNoAdmitido              Visa = "no_admitido"
)

// Documento dice con qué documento se puede entrar.
type Documento string

const (
	DocumentoDNIOPasaporte Documento = "dni_o_pasaporte"
	DocumentoPasaporte     Documento = "pasaporte"
)

// Vigencia es cuánto tiene que durar el pasaporte y desde cuándo se cuenta.
type Vigencia struct {
	Amount int `json:"amount"`
	// Unit es «dias» o «meses».
	Unit string `json:"unit"`
	// Base es «llegada» (desde que se entra) o «salida_del_pais» (desde que se
	// sale del país, o del espacio Schengen).
	Base string `json:"base"`
}

// Exencion cambia lo que se pide cuando el pasajero ya tiene un permiso
// vigente de otro país: «con visa de EE.UU. vigente alcanza la eTA».
type Exencion struct {
	// PermitKinds son los tipos de permiso que la habilitan («visa»,
	// «autorizacion_electronica», «residencia»). Vacío: cualquiera.
	PermitKinds   []string `json:"permitKinds,omitempty"`
	PermitCountry string   `json:"permitCountry"`
	Visa          Visa     `json:"visa"`
}

// Vacuna es una vacuna exigida para entrar.
type Vacuna struct {
	Name string `json:"name"`
	// Scope es «todos» o «desde_pais_de_riesgo».
	Scope string `json:"scope"`
}

const (
	VacunaParaTodos       = "todos"
	VacunaDesdePaisRiesgo = "desde_pais_de_riesgo"
)

// Citation es de dónde salió un dato: la fuente, la cita textual y su enlace.
type Citation struct {
	Source   string `json:"source"`
	Quote    string `json:"quote,omitempty"`
	URL      string `json:"url,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// Fuentes conocidas.
const (
	SourceWikipedia     = "wikipedia"
	SourcePassportIndex = "passport_index"
	SourceConstante     = "constante"
)

// Field es un dato de la regla. O tiene valor, o está en «verificar» con el
// motivo. Un campo sin valor y sin «verificar» es uno que no se exige.
type Field[T any] struct {
	Value   *T         `json:"value,omitempty"`
	Verify  bool       `json:"verify,omitempty"`
	Reason  string     `json:"reason,omitempty"`
	Sources []Citation `json:"sources,omitempty"`
}

func known[T any](value T, sources ...Citation) Field[T] {
	return Field[T]{Value: &value, Sources: sources}
}

func verify[T any](reason string, sources ...Citation) Field[T] {
	return Field[T]{Verify: true, Reason: reason, Sources: sources}
}

// Fields son los datos de una regla. La lista es cerrada: el formulario
// manual, el agente futuro y el motor de evaluación comparten esta forma.
type Fields struct {
	Documento       Field[Documento]  `json:"documento"`
	Visa            Field[Visa]       `json:"visa"`
	EstadiaMaxDias  Field[int]        `json:"estadiaMaxDias"`
	Vigencia        Field[Vigencia]   `json:"vigencia"`
	EmisionMaxAnios Field[int]        `json:"emisionMaxAnios"`
	Exenciones      Field[[]Exencion] `json:"exenciones"`
	Vacunas         Field[[]Vacuna]   `json:"vacunas"`
}

// AllVerify es la regla de la que no se sabe nada: todo en «verificar».
func AllVerify(reason string) Fields {
	return Fields{
		Documento:       verify[Documento](reason),
		Visa:            verify[Visa](reason),
		EstadiaMaxDias:  verify[int](reason),
		Vigencia:        verify[Vigencia](reason),
		EmisionMaxAnios: verify[int](reason),
		Exenciones:      verify[[]Exencion](reason),
		Vacunas:         verify[[]Vacuna](reason),
	}
}

// Rule es lo que un país pide a una nacionalidad.
type Rule struct {
	Pair
	Status    RuleStatus `json:"status"`
	Fields    Fields     `json:"fields"`
	Version   int        `json:"version"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Effective devuelve los campos que valen para evaluar: una regla que todavía
// se busca, o que nunca tuvo fuentes, no sabe nada.
func (r Rule) Effective() Fields {
	switch r.Status {
	case RuleBuscando:
		return AllVerify("buscando requisitos")
	case RuleSinFuentes:
		return AllVerify("no se pudieron consultar las fuentes")
	default:
		return r.Fields
	}
}
