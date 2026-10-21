// Package evaluacion cruza las reglas de entrada con los datos de cada pasajero
// y dice, por pasajero y por país del itinerario, qué cumple, qué falta y qué
// hay que verificar.
//
// Es código común, sin red ni base: lee campos y no sabe quién los cargó. Lo
// que el sistema no sabe lo dice —«verificar»—, nunca lo da por bueno.
package evaluacion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

type Status string

const (
	Cumple    Status = "cumple"
	Falta     Status = "falta"
	Verificar Status = "verificar"
	// Aviso es una recomendación: no es un faltante, no pesa en los controles
	// y no se confirma.
	Aviso Status = "aviso"
)

// Las claves de los requisitos. Las vacunas son «vacuna:<nombre>» y las
// condiciones «condicion:<tipo>».
const (
	KeyItinerario       = "itinerario"
	KeyDocumentoDeViaje = "documento_de_viaje"
	KeyDocumento        = "documento"
	KeyVigencia         = "vigencia"
	KeyAntiguedad       = "antiguedad"
	KeyVisa             = "visa"
	KeyEstadia          = "estadia"
	KeyVacunas          = "vacunas"
	KeyTransito         = "transito"
	KeyMenor            = "menor"
	KeyPaisPropio       = "pais_propio"
	KeyCondiciones      = "condiciones"
	KeyRenovar          = "renovar_documento"
	vacunaPrefix        = "vacuna:"
	condicionPrefix     = reglas.CondicionPrefix
)

// Requirement es una línea del panel de requisitos.
type Requirement struct {
	// PassengerID vacío: es del viaje (falta el itinerario).
	PassengerID string `json:"passengerId,omitempty"`
	// Country vacío: no es de un país (la autorización de menor).
	Country string `json:"country,omitempty"`
	Role    string `json:"role,omitempty"`
	Key     string `json:"key"`
	Status  Status `json:"status"`
	// Text es el título del renglón, el nombre del requisito; Detail lo
	// explica. ClientText es lo que va al aviso del cliente.
	Text       string `json:"text"`
	Detail     Detail `json:"detail"`
	ClientText string `json:"-"`
	// Confirmable: lo que el sistema no puede saber y una persona sí. Un dato
	// que falta no se confirma: se carga.
	Confirmable bool `json:"confirmable"`
	Confirmed   bool `json:"confirmed,omitempty"`
	// MissingData: falta porque no está cargado el dato, no porque no cumpla.
	MissingData bool `json:"missingData,omitempty"`
	// Searching: la regla del país todavía se está buscando.
	Searching bool              `json:"searching,omitempty"`
	Sources   []reglas.Citation `json:"sources,omitempty"`
	// FieldHash ata una confirmación al valor de la regla con que se hizo.
	FieldHash string `json:"-"`
}

// Detail explica un renglón: qué pide el país (con la explicación de las
// páginas oficiales, si la hay), el trámite, qué tiene el pasajero y qué hay
// que hacer. Una parte vacía no se muestra.
type Detail struct {
	Pide    []Line   `json:"pide,omitempty"`
	Tramite []Line   `json:"tramite,omitempty"`
	Tiene   []string `json:"tiene,omitempty"`
	Hacer   []string `json:"hacer,omitempty"`
	// Oficial dice, cuando el renglón todavía no tiene detalle de las páginas
	// oficiales, si se está buscando («buscando») o no se encontró
	// («sin_datos»).
	Oficial reglas.DetallesStatus `json:"oficial,omitempty"`
}

// Line es una frase del detalle. Las que salen de una fuente la llevan; las
// que arma el sistema con la regla, no.
type Line struct {
	Text   string           `json:"text"`
	Source *reglas.Citation `json:"source,omitempty"`
}

type Trip struct {
	// Origin es de dónde sale el viaje. Vacío: Argentina, como en la base.
	Origin     string
	DepartAt   *time.Time
	ReturnAt   *time.Time
	Itinerary  []Stop
	Passengers []Passenger
}

// Argentina es el origen por defecto y la nacionalidad de la vista previa: la
// agencia vende desde ahí, y el pasaporte argentino es el caso más común.
const Argentina = "AR"

// PreviewNationality es la nacionalidad con la que se muestra lo que piden los
// países mientras el viaje no tiene pasajeros.
const PreviewNationality = Argentina

// renewWithin es cuánto antes de la vuelta se recomienda renovar el pasaporte.
// No es una regla de ningún país: algunas aerolíneas piden 6 meses de vigencia
// aunque el destino no los exija, y es lo primero que mira un cliente.
const renewWithin = 6

func (t Trip) origin() string {
	if t.Origin == "" {
		return Argentina
	}
	return t.Origin
}

type Stop struct {
	Country  string
	Role     string
	ArriveAt *time.Time
	LeaveAt  *time.Time
}

type Passenger struct {
	ID               string
	Name             string
	Minor            bool
	ResidenceCountry string
	// Doc es el documento con el que viaja. Nil: no hay ninguno cargado.
	Doc *Document
	// Docs son todos sus documentos, entre los que ChooseDocuments elige.
	Docs []Document
	// DocManual: una persona eligió Doc a mano y el sistema no lo cambia.
	DocManual bool
	Permits   []Permit
}

type Document struct {
	ID        string
	Type      string
	Country   string
	Number    string
	IssuedAt  *time.Time
	ExpiresAt *time.Time
}

type Permit struct {
	Kind      string
	Country   string
	Label     string
	ValidFrom *time.Time
	ExpiresAt *time.Time
}

type Confirmation struct {
	PassengerID string
	Country     string
	Key         string
	FieldHash   string
}

// Pairs son las reglas que hacen falta para evaluar el viaje. Sin pasajeros,
// las de un pasaporte argentino, que son las de la vista previa: así la
// búsqueda arranca apenas se carga el destino.
func Pairs(t Trip) []reglas.Pair {
	nationalities := []string{}
	for _, p := range t.Passengers {
		for _, doc := range candidates(p) {
			nationalities = append(nationalities, doc.Country)
		}
	}
	if len(t.Passengers) == 0 {
		nationalities = append(nationalities, PreviewNationality)
	}
	seen := map[reglas.Pair]bool{}
	var out []reglas.Pair
	for _, nationality := range nationalities {
		for _, stop := range t.Itinerary {
			pair := reglas.Pair{Nationality: nationality, Country: stop.Country}
			if stop.Country != nationality && !seen[pair] {
				seen[pair] = true
				out = append(out, pair)
			}
		}
	}
	return out
}

func Evaluate(t Trip, rules map[reglas.Pair]reglas.Rule, confirmed []Confirmation) []Requirement {
	var out []Requirement
	if len(t.Itinerary) == 0 {
		out = append(out, Requirement{Key: KeyItinerario, Status: Falta, MissingData: true, Text: "Destino del viaje",
			Detail: Detail{Tiene: []string{"El viaje no tiene países cargados."},
				Hacer: []string{"Cargar el país de destino en Viaje."}}})
	}
	for _, p := range t.Passengers {
		// La autorización de salida de menores es una regla argentina: las de
		// salida de los demás países no se modelan.
		if p.Minor && t.origin() == Argentina {
			out = append(out, Requirement{PassengerID: p.ID, Key: KeyMenor, Status: Falta, Confirmable: true,
				FieldHash: hash(KeyMenor), Text: "Autorización de viaje del menor",
				Detail: Detail{
					Pide:  []Line{{Text: "La Argentina pide que los menores de 18 años que salen del país viajen con ambos padres o con una autorización de viaje."}},
					Tiene: []string{"Es menor a la fecha de salida."},
					Hacer: []string{"Conseguir la autorización de viaje, o confirmar que viaja con ambos padres."},
				},
				ClientText: "al ser menor, necesita la autorización de viaje si no viaja con ambos padres"})
		}
		if p.Doc == nil {
			out = append(out, Requirement{PassengerID: p.ID, Key: KeyDocumentoDeViaje, Status: Falta, MissingData: true,
				Text: "Documento de viaje", Detail: Detail{Tiene: []string{"No tiene documentos cargados."},
					Hacer: []string{"Cargar el DNI o el pasaporte en Pasajeros."}}})
			continue
		}
		var own []Requirement
		for _, stop := range t.Itinerary {
			if stop.Country == p.Doc.Country {
				own = append(own, Requirement{PassengerID: p.ID, Country: stop.Country, Role: stop.Role,
					Key: KeyPaisPropio, Status: Cumple, Text: "Su país",
					Detail: Detail{Pide: []Line{{Text: "Es el país de su documento: no necesita nada para entrar."}}}})
				continue
			}
			e := evaluator{p: p, stop: stop, f: reglas.AllVerify("buscando requisitos"), searching: true,
				name: countryName(stop.Country), origin: t.origin(), returnAt: t.ReturnAt}
			if rule, ok := rules[reglas.Pair{Nationality: p.Doc.Country, Country: stop.Country}]; ok {
				e.f, e.det, e.searching = rule.Effective(), rule.Details, rule.Status == reglas.RuleBuscando
			}
			e.arrive, e.leave = stopDates(t, stop)
			if stop.Role == "escala" {
				own = append(own, e.mark(e.transito()))
				continue
			}
			own = append(own, e.mark(e.documento()), e.mark(e.vigencia()))
			if r, ok := e.antiguedad(); ok {
				own = append(own, e.mark(r))
			}
			visa, level := e.visa()
			own = append(own, e.mark(visa))
			if r, ok := e.estadia(level); ok {
				own = append(own, e.mark(r))
			}
			for _, r := range e.vacunas() {
				own = append(own, e.mark(r))
			}
			for _, r := range e.condiciones() {
				own = append(own, e.mark(r))
			}
		}
		if r, ok := renewal(t, p, own); ok {
			out = append(out, r)
		}
		out = append(out, own...)
	}

	for i := range out {
		r := &out[i]
		if !r.Confirmable || r.Status == Cumple {
			continue
		}
		for _, c := range confirmed {
			if c.PassengerID == r.PassengerID && c.Country == r.Country && c.Key == r.Key && c.FieldHash == r.FieldHash {
				r.Status, r.Confirmed = Cumple, true
				break
			}
		}
	}
	return out
}

// renewal recomienda renovar el documento que vence a menos de 6 meses de la
// vuelta. Si ya no cumple la vigencia o la antigüedad de algún país, ese
// renglón está en falta y la recomendación sobra.
func renewal(t Trip, p Passenger, reqs []Requirement) (Requirement, bool) {
	doc := p.Doc
	if t.ReturnAt == nil || doc.ExpiresAt == nil || !doc.ExpiresAt.Before(t.ReturnAt.AddDate(0, renewWithin, 0)) {
		return Requirement{}, false
	}
	for _, r := range reqs {
		if DocumentControl(r.Key) && r.Status == Falta && !r.MissingData {
			return Requirement{}, false
		}
	}
	kind := docKind(doc)
	when := fmt.Sprintf("a menos de %d meses de la vuelta (%s)", renewWithin, day(*t.ReturnAt))
	if doc.ExpiresAt.Before(*t.ReturnAt) {
		when = fmt.Sprintf("antes de la vuelta (%s)", day(*t.ReturnAt))
	}
	return Requirement{PassengerID: p.ID, Key: KeyRenovar, Status: Aviso, Text: "Renovación del " + kind,
		Detail: Detail{
			Pide:  []Line{{Text: fmt.Sprintf("Recomendamos que el %s venza al menos %d meses después de la vuelta: algunas aerolíneas lo piden aunque el destino no lo exija.", kind, renewWithin)}},
			Tiene: []string{fmt.Sprintf("El %s vence el %s, %s.", kind, day(*doc.ExpiresAt), when)},
			Hacer: []string{fmt.Sprintf("Recomendarle al cliente que renueve el %s.", kind)},
		},
		ClientText: fmt.Sprintf("renovar el %s de %s: vence el %s, %s", kind, p.Name, day(*doc.ExpiresAt), when)}, true
}

func stopDates(t Trip, stop Stop) (*time.Time, *time.Time) {
	arrive, leave := stop.ArriveAt, stop.LeaveAt
	if arrive == nil {
		arrive = t.DepartAt
	}
	if leave == nil {
		leave = t.ReturnAt
	}
	return arrive, leave
}

// evaluator evalúa un pasajero en un país.
type evaluator struct {
	p             Passenger
	stop          Stop
	f             reglas.Fields
	det           reglas.Detalles
	searching     bool
	name          string
	origin        string
	arrive, leave *time.Time
	returnAt      *time.Time
}

func (e evaluator) req(key string, status Status, title string) Requirement {
	return Requirement{PassengerID: e.p.ID, Country: e.stop.Country, Role: e.stop.Role, Key: key, Status: status, Text: title}
}

func (e evaluator) verify(key, title string, field any) Requirement {
	r := e.req(key, Verificar, title)
	r.Confirmable, r.FieldHash = true, hash(key, field)
	return r
}

func (e evaluator) missing(key, title string) Requirement {
	r := e.req(key, Falta, title)
	r.MissingData = true
	return r
}

// mark marca los renglones de una regla que todavía se busca.
func (e evaluator) mark(r Requirement) Requirement {
	r.Searching = e.searching
	return r
}

// official suma al renglón el detalle de las páginas oficiales del grupo. El
// trámite va solo donde hay algo que hacer. Sin el grupo, el renglón dice si
// todavía se busca o si no se encontró.
func (e evaluator) official(r *Requirement, group string, withTramite bool) {
	g, ok := e.det.Grupos[group]
	if !ok {
		if e.det.Status == reglas.DetallesBuscando || e.det.Status == reglas.DetallesSinDatos {
			r.Detail.Oficial = e.det.Status
		}
		return
	}
	for _, fr := range g.Pide {
		r.Detail.Pide = append(r.Detail.Pide, sourced(fr))
		r.Sources = appendSource(r.Sources, fr.Source)
	}
	if !withTramite {
		return
	}
	for _, fr := range g.Tramite {
		r.Detail.Tramite = append(r.Detail.Tramite, sourced(fr))
		r.Sources = appendSource(r.Sources, fr.Source)
	}
}

func sourced(fr reglas.Frase) Line {
	src := fr.Source
	return Line{Text: fr.Text, Source: &src}
}

func appendSource(sources []reglas.Citation, c reglas.Citation) []reglas.Citation {
	for _, s := range sources {
		if s.URL == c.URL && s.Quote == c.Quote {
			return sources
		}
	}
	return append(sources, c)
}

func (r *Requirement) pide(format string, args ...any) {
	r.Detail.Pide = append(r.Detail.Pide, Line{Text: fmt.Sprintf(format, args...)})
}

func (r *Requirement) tiene(format string, args ...any) {
	r.Detail.Tiene = append(r.Detail.Tiene, fmt.Sprintf(format, args...))
}

func (r *Requirement) hacer(format string, args ...any) {
	r.Detail.Hacer = append(r.Detail.Hacer, fmt.Sprintf(format, args...))
}

// docKind es el tipo de documento en minúscula para el medio de una frase.
func docKind(doc *Document) string {
	if doc.Type == "DNI" {
		return "DNI"
	}
	return strings.ToLower(doc.Type)
}

// describeDoc dice el documento con todo lo que se sabe de él: «pasaporte de
// Argentina N° AAB123456, emitido el 12/05/2021, que vence el 12/05/2031».
func describeDoc(doc *Document) string {
	s := fmt.Sprintf("%s de %s", docKind(doc), countryName(doc.Country))
	if doc.Number != "" {
		s += " N° " + doc.Number
	}
	if doc.IssuedAt != nil {
		s += ", emitido el " + day(*doc.IssuedAt)
	}
	if doc.ExpiresAt != nil {
		s += ", que vence el " + day(*doc.ExpiresAt)
	} else {
		s += ", sin vencimiento cargado"
	}
	return s
}

// nationality dice con qué se viaja, para las frases de la regla: «con
// pasaporte de Argentina».
func (e evaluator) nationality() string {
	return fmt.Sprintf("con %s de %s", docKind(e.p.Doc), countryName(e.p.Doc.Country))
}

func (e evaluator) documento() Requirement {
	f := e.f.Documento
	var r Requirement
	switch {
	case f.Verify:
		r = e.verify(KeyDocumento, "Documento", f)
		r.pide("No se sabe con qué documento se entra a %s: %s.", e.name, f.Reason)
		r.hacer("Revisar con qué documento se entra a %s y confirmarlo.", e.name)
	case f.Value != nil && *f.Value == reglas.DocumentoPasaporte && e.p.Doc.Type == "DNI":
		r = e.req(KeyDocumento, Falta, "Documento")
		r.ClientText = fmt.Sprintf("para entrar a %s necesita pasaporte, no alcanza con el DNI", e.name)
		r.hacer("Viajar con pasaporte. Si ya tiene uno, cargarlo en Pasajeros.")
	default:
		r = e.req(KeyDocumento, Cumple, "Documento")
	}
	if !f.Verify && f.Value != nil {
		switch {
		case *f.Value == reglas.DocumentoDNIOPasaporte:
			r.pide("A %s se entra con DNI o con pasaporte.", e.name)
		case len(f.Sources) == 0:
			r.pide("%s pide pasaporte: ninguna fuente dice que alcance el DNI.", e.name)
		default:
			r.pide("%s pide pasaporte: no alcanza con el DNI.", e.name)
		}
	}
	r.tiene("Viaja con %s.", describeDoc(e.p.Doc))
	r.Sources = append(r.Sources, f.Sources...)
	e.official(&r, reglas.GrupoDocumento, r.Status != Cumple)
	return r
}

// vigenciaRule dice la vigencia que pide el país, en palabras.
func vigenciaRule(v reglas.Vigencia, name, country, kind string) string {
	if v.Amount == 0 {
		if v.Base == "salida_del_pais" {
			return fmt.Sprintf("%s pide que el %s esté vigente toda la estadía, hasta el día en que se sale del país.", name, kind)
		}
		return fmt.Sprintf("%s pide que el %s esté vigente al llegar.", name, kind)
	}
	unit := "días"
	if v.Unit == "meses" {
		unit = "meses"
		if v.Amount == 1 {
			unit = "mes"
		}
	}
	from := "desde la llegada"
	if v.Base == "salida_del_pais" {
		from = fmt.Sprintf("después de salir de %s", name)
		if reglas.InSchengen(country) {
			from = "después de salir del espacio Schengen"
		}
	}
	return fmt.Sprintf("%s pide que el %s dure al menos %d %s %s.", name, kind, v.Amount, unit, from)
}

func addVigencia(t time.Time, v reglas.Vigencia, sign int) time.Time {
	if v.Unit == "meses" {
		return t.AddDate(0, sign*v.Amount, 0)
	}
	return t.AddDate(0, 0, sign*v.Amount)
}

func (e evaluator) vigencia() Requirement {
	f := e.f.Vigencia
	doc := e.p.Doc
	kind := docKind(doc)
	title := "Vigencia del " + kind
	known := !f.Verify && f.Value != nil

	var r Requirement
	switch {
	case doc.ExpiresAt == nil:
		r = e.missing(KeyVigencia, title)
		if known {
			r.pide("%s", vigenciaRule(*f.Value, e.name, e.stop.Country, kind))
		}
		r.tiene("No está cargado el vencimiento del %s.", kind)
		r.hacer("Cargar el vencimiento del %s en Pasajeros.", kind)
	case !known:
		reason := f.Reason
		if reason == "" {
			reason = "ninguna fuente lo dice"
		}
		r = e.verify(KeyVigencia, title, f)
		r.pide("No se sabe cuánta vigencia del documento pide %s: %s.", e.name, reason)
		r.tiene("El %s vence el %s.", kind, day(*doc.ExpiresAt))
		r.hacer("Revisar cuánto tiene que durar el %s para entrar a %s.", kind, e.name)
	default:
		v := *f.Value
		r = e.req(KeyVigencia, Cumple, title)
		r.pide("%s", vigenciaRule(v, e.name, e.stop.Country, kind))
		if f.Reason != "" {
			r.pide("%s.", capitalize(f.Reason))
		}
		base, moment := e.arrive, "llega a "+e.name
		if v.Base == "salida_del_pais" {
			base, moment = e.leave, "sale de "+e.name
		}
		if base == nil {
			r.Status, r.MissingData = Falta, true
			r.tiene("El %s vence el %s. El viaje no tiene fechas cargadas.", kind, day(*doc.ExpiresAt))
			r.hacer("Cargar las fechas del viaje. Si %s hasta el %s, cumple.", moment, day(addVigencia(*doc.ExpiresAt, v, -1)))
			break
		}
		required := addVigencia(*base, v, 1)
		r.pide("Para este viaje tiene que durar hasta el %s.", day(required))
		r.tiene("El %s vence el %s.", kind, day(*doc.ExpiresAt))
		if doc.ExpiresAt.Before(required) {
			r.Status = Falta
			r.ClientText = fmt.Sprintf("el %s vence el %s y para %s tiene que durar hasta el %s: hay que renovarlo",
				kind, day(*doc.ExpiresAt), e.name, day(required))
			r.hacer("Renovar el %s antes de viajar.", kind)
		}
	}
	r.Sources = append(r.Sources, f.Sources...)
	e.official(&r, reglas.GrupoVigencia, r.Status == Falta && !r.MissingData)
	return r
}

func (e evaluator) antiguedad() (Requirement, bool) {
	f := e.f.EmisionMaxAnios
	kind := docKind(e.p.Doc)
	title := "Antigüedad del " + kind
	var r Requirement
	switch {
	case f.Verify:
		r = e.verify(KeyAntiguedad, title, f)
		r.pide("No se sabe si %s pide un %s reciente: %s.", e.name, kind, f.Reason)
		r.hacer("Revisar si %s pide un %s emitido hace poco y confirmarlo.", e.name, kind)
	case f.Value == nil:
		return Requirement{}, false
	default:
		years := *f.Value
		issued := e.p.Doc.IssuedAt
		r = e.req(KeyAntiguedad, Cumple, title)
		r.pide("%s pide que el %s se haya emitido hace menos de %d años.", e.name, kind, years)
		switch {
		case issued == nil:
			r.Status, r.MissingData = Falta, true
			r.tiene("No está cargada la fecha de emisión del %s.", kind)
			r.hacer("Cargar la fecha de emisión del %s en Pasajeros.", kind)
		case e.arrive == nil:
			r.Status, r.MissingData = Falta, true
			r.tiene("El %s se emitió el %s. El viaje no tiene fechas cargadas.", kind, day(*issued))
			r.hacer("Cargar las fechas del viaje. Si llega a %s antes del %s, cumple.", e.name, day(issued.AddDate(years, 0, 0)))
		case issued.AddDate(years, 0, 0).Before(*e.arrive):
			r.Status = Falta
			r.tiene("El %s se emitió el %s: al llegar tiene más de %d años.", kind, day(*issued), years)
			r.hacer("Renovar el %s antes de viajar.", kind)
			r.ClientText = fmt.Sprintf("el %s tiene más de %d años y %s no lo acepta: hay que renovarlo", kind, years, e.name)
		default:
			r.tiene("El %s se emitió el %s.", kind, day(*issued))
		}
	}
	r.Sources = append(r.Sources, f.Sources...)
	e.official(&r, reglas.GrupoAntiguedad, false)
	return r, true
}

var visaLabels = map[reglas.Visa]string{
	reglas.VisaAutorizacionElectronica: "autorización electrónica",
	reglas.VisaElectronica:             "visa electrónica",
	reglas.VisaConsular:                "visa",
}

// visaRule dice la categoría de visa en palabras.
func visaRule(level reglas.Visa, name, with string) string {
	switch level {
	case reglas.VisaNoRequiere:
		return fmt.Sprintf("%s no pide visa a quien viaja %s.", name, with)
	case reglas.VisaALaLlegada:
		return fmt.Sprintf("%s pide visa a quien viaja %s, pero se tramita al llegar.", name, with)
	case reglas.VisaAutorizacionElectronica:
		return fmt.Sprintf("%s no pide visa a quien viaja %s, pero sí una autorización electrónica de viaje, que se pide online antes de viajar.", name, with)
	case reglas.VisaElectronica:
		return fmt.Sprintf("%s pide visa electrónica a quien viaja %s: se pide online antes de viajar.", name, with)
	case reglas.VisaConsular:
		return fmt.Sprintf("%s pide visa consular a quien viaja %s: se tramita antes de viajar, ante la embajada o el consulado.", name, with)
	case reglas.VisaNoAdmitido:
		return fmt.Sprintf("%s no admite el ingreso de quien viaja %s.", name, with)
	}
	return ""
}

// exencionRule dice una exención en palabras: «Con visa de Estados Unidos
// vigente, alcanza con la autorización electrónica».
func exencionRule(ex reglas.Exencion) string {
	kinds := "un permiso"
	if len(ex.PermitKinds) > 0 {
		labels := make([]string, len(ex.PermitKinds))
		for i, k := range ex.PermitKinds {
			labels[i] = kindLabel(k)
		}
		kinds = strings.Join(labels, " o ")
	}
	needs := "no pide visa"
	if label, ok := visaLabels[ex.Visa]; ok {
		needs = "alcanza con la " + label
	} else if ex.Visa == reglas.VisaALaLlegada {
		needs = "la visa se tramita al llegar"
	}
	return fmt.Sprintf("Con %s de %s vigente, %s.", kinds, countryName(ex.PermitCountry), needs)
}

// visa devuelve el requisito y lo que se termina pidiendo después de aplicar
// las exenciones.
func (e evaluator) visa() (Requirement, reglas.Visa) {
	f := e.f.Visa
	if f.Verify || f.Value == nil {
		r := e.verify(KeyVisa, "Visa", f)
		r.pide("No se sabe qué visa pide %s: %s.", e.name, f.Reason)
		e.permitsHeld(&r, nil)
		r.hacer("Revisar qué visa pide %s a quien viaja %s y confirmarlo.", e.name, e.nationality())
		r.Sources = append(r.Sources, f.Sources...)
		e.official(&r, reglas.GrupoVisa, true)
		return r, ""
	}
	level := *f.Value
	var exemptBy *Permit
	exenciones := []reglas.Exencion{}
	if e.f.Exenciones.Value != nil {
		exenciones = *e.f.Exenciones.Value
	}
	if level != reglas.VisaNoRequiere && level != reglas.VisaALaLlegada && level != reglas.VisaNoAdmitido {
		for _, ex := range exenciones {
			if permit, ok := e.exemptingPermit(ex); ok {
				level, exemptBy = ex.Visa, &permit
				break
			}
		}
	}
	via := ""
	if exemptBy != nil {
		via = fmt.Sprintf("con %s de %s vigente", kindLabel(exemptBy.Kind), countryName(exemptBy.Country))
	}

	r := e.req(KeyVisa, Cumple, "Visa")
	r.Sources = append(r.Sources, f.Sources...)
	r.pide("%s", visaRule(*f.Value, e.name, e.nationality()))
	if *f.Value != reglas.VisaNoRequiere && *f.Value != reglas.VisaALaLlegada && *f.Value != reglas.VisaNoAdmitido {
		for _, ex := range exenciones {
			r.pide("%s", exencionRule(ex))
		}
		if len(exenciones) > 0 {
			r.Sources = append(r.Sources, e.f.Exenciones.Sources...)
		}
	}
	if exemptBy != nil {
		held := fmt.Sprintf("Tiene %s de %s", kindLabel(exemptBy.Kind), countryName(exemptBy.Country))
		if exemptBy.ExpiresAt != nil {
			held += " vigente hasta el " + day(*exemptBy.ExpiresAt)
		}
		r.tiene("%s: se aplica la excepción.", held)
	}

	switch level {
	case reglas.VisaNoRequiere:
		e.official(&r, reglas.GrupoVisa, false)
		return r, level
	case reglas.VisaALaLlegada:
		r.hacer("Avisarle al cliente que la visa se tramita en el aeropuerto al llegar.")
		e.official(&r, reglas.GrupoVisa, true)
		return r, level
	case reglas.VisaNoAdmitido:
		r.Status = Falta
		r.ClientText = fmt.Sprintf("%s no admite el ingreso con este documento", e.name)
		r.hacer("Viajar con otro documento, si tiene.")
		e.official(&r, reglas.GrupoVisa, false)
		return r, level
	}

	label := visaLabels[level]
	accepted := []string{"visa"}
	if level == reglas.VisaAutorizacionElectronica {
		accepted = []string{"autorizacion_electronica", "visa"}
	}
	var expiring *Permit
	for i := range e.p.Permits {
		pm := e.p.Permits[i]
		if pm.Country != e.stop.Country || !contains(accepted, pm.Kind) {
			continue
		}
		if e.coversStay(pm) {
			r.tiene("Tiene %s de %s%s.", kindLabel(pm.Kind), e.name, until(pm.ExpiresAt))
			e.official(&r, reglas.GrupoVisa, false)
			return r, level
		}
		expiring = &pm
	}

	r.Status = Falta
	switch {
	case expiring != nil && expiring.ExpiresAt != nil:
		r.tiene("La %s de %s vence el %s, antes de terminar el viaje.", kindLabel(expiring.Kind), e.name, day(*expiring.ExpiresAt))
		r.hacer("Renovar la %s antes del viaje y cargar la nueva en Pasajeros.", kindLabel(expiring.Kind))
		r.ClientText = fmt.Sprintf("la %s de %s vence el %s, antes de terminar el viaje: hay que renovarla",
			kindLabel(expiring.Kind), e.name, day(*expiring.ExpiresAt))
	case via != "":
		r.tiene("No tiene %s de %s cargada.", label, e.name)
		r.hacer("Si ya tiene la %s, cargarla en Pasajeros con su vencimiento; si no, tramitarla antes de viajar.", label)
		r.ClientText = fmt.Sprintf("%s alcanza con la %s de %s, que se tramita antes de viajar", via, label, e.name)
	default:
		r.tiene("No tiene %s de %s cargada.", label, e.name)
		r.hacer("Si ya tiene la %s, cargarla en Pasajeros con su vencimiento; si no, tramitarla antes de viajar.", label)
		r.ClientText = fmt.Sprintf("para %s necesita %s, que se tramita antes de viajar", e.name, label)
	}
	e.official(&r, reglas.GrupoVisa, true)
	if len(r.Detail.Tramite) > 0 && r.ClientText != "" {
		r.ClientText += ". " + r.Detail.Tramite[0].Text
	}
	return r, level
}

// permitsHeld dice qué permisos del país tiene el pasajero cargados.
func (e evaluator) permitsHeld(r *Requirement, accepted []string) {
	for _, pm := range e.p.Permits {
		if pm.Country == e.stop.Country && (accepted == nil || contains(accepted, pm.Kind)) {
			r.tiene("Tiene %s de %s%s.", kindLabel(pm.Kind), e.name, until(pm.ExpiresAt))
		}
	}
}

// exemptingPermit busca un permiso del pasajero que habilite la exención. La
// residencia del pasajero cuenta como un permiso de residencia de ese país.
func (e evaluator) exemptingPermit(ex reglas.Exencion) (Permit, bool) {
	kindOK := func(kind string) bool { return len(ex.PermitKinds) == 0 || contains(ex.PermitKinds, kind) }
	for _, pm := range e.p.Permits {
		if pm.Country == ex.PermitCountry && kindOK(pm.Kind) && e.coversStay(pm) {
			return pm, true
		}
	}
	if e.p.ResidenceCountry == ex.PermitCountry && kindOK("residencia") {
		return Permit{Kind: "residencia", Country: ex.PermitCountry}, true
	}
	return Permit{}, false
}

// coversStay dice si el permiso vale durante toda la estadía. Sin fechas del
// viaje, alcanza con que no esté vencido.
func (e evaluator) coversStay(pm Permit) bool {
	if pm.ValidFrom != nil && e.arrive != nil && pm.ValidFrom.After(*e.arrive) {
		return false
	}
	if pm.ExpiresAt == nil {
		return true
	}
	end := e.leave
	if end == nil {
		end = e.arrive
	}
	return end == nil || !pm.ExpiresAt.Before(*end)
}

// estadia mide la estadía donde no hace falta visa: con visa, los días los
// da la visa. Sin fechas no hay renglón: la vigencia ya pide cargarlas.
func (e evaluator) estadia(level reglas.Visa) (Requirement, bool) {
	f := e.f.EstadiaMaxDias
	if (level != reglas.VisaNoRequiere && level != reglas.VisaALaLlegada) || e.arrive == nil || e.leave == nil {
		return Requirement{}, false
	}
	days := int(e.leave.Sub(*e.arrive).Hours()/24) + 1
	stay := fmt.Sprintf("Se queda %d días, del %s al %s.", days, day(*e.arrive), day(*e.leave))
	var r Requirement
	switch {
	case f.Verify:
		r = e.verify(KeyEstadia, "Estadía", f)
		r.pide("No se sabe cuántos días permite %s sin visa: %s.", e.name, f.Reason)
		r.tiene("%s", stay)
		r.hacer("Revisar cuántos días se puede quedar en %s sin visa y confirmarlo.", e.name)
	case f.Value == nil:
		return Requirement{}, false
	default:
		r = e.req(KeyEstadia, Cumple, "Estadía")
		r.pide("%s permite quedarse hasta %d días sin visa.", e.name, *f.Value)
		if f.Reason != "" {
			r.pide("%s.", capitalize(f.Reason))
		}
		r.tiene("%s", stay)
		if days > *f.Value {
			r.Status = Falta
			r.hacer("Acortar la estadía a %d días o tramitar una visa.", *f.Value)
			r.ClientText = fmt.Sprintf("la estadía en %s es de %d días y el máximo sin visa es %d", e.name, days, *f.Value)
		}
	}
	r.Sources = append(r.Sources, f.Sources...)
	e.official(&r, reglas.GrupoEstadia, r.Status != Cumple)
	return r, true
}

func (e evaluator) vacunas() []Requirement {
	f := e.f.Vacunas
	if f.Verify || f.Value == nil {
		r := e.verify(KeyVacunas, "Vacunas", f)
		r.pide("No se sabe qué vacunas pide %s: %s.", e.name, f.Reason)
		r.hacer("Revisar qué vacunas pide %s y confirmarlo.", e.name)
		r.Sources = append(r.Sources, f.Sources...)
		e.official(&r, reglas.GrupoVacunas, true)
		return []Requirement{r}
	}
	var out []Requirement
	for _, v := range *f.Value {
		key := vacunaPrefix + v.Name
		r := e.req(key, Falta, "Vacuna: "+v.Name)
		r.Confirmable, r.FieldHash, r.Sources = true, hash(key, v), append([]reglas.Citation{}, f.Sources...)
		r.ClientText = fmt.Sprintf("para %s necesita la vacuna de %s, con su certificado", e.name, v.Name)
		if v.Scope == reglas.VacunaDesdePaisRiesgo {
			r.Status = Verificar
			r.pide("%s pide la vacuna de %s a quienes llegan desde países de riesgo.", e.name, v.Name)
			r.pide("%s.", capitalize(riskOrigin(e.origin)))
			r.hacer("Revisar si el viaje llega a %s desde un país de riesgo y confirmarlo.", e.name)
		} else {
			r.pide("%s pide la vacuna de %s a todos los viajeros, con el certificado internacional de vacunación.", e.name, v.Name)
			r.hacer("Conseguir el certificado de vacunación y confirmarlo acá cuando lo tenga.")
		}
		e.official(&r, reglas.GrupoVacunas, true)
		out = append(out, r)
	}
	return out
}

var condicionLabels = map[string]string{
	reglas.CondicionRegistro:        "Registro al llegar",
	reglas.CondicionPreregistro:     "Registro antes de viajar",
	reglas.CondicionPasajeRegreso:   "Pasaje de regreso",
	reglas.CondicionFondos:          "Fondos suficientes",
	reglas.CondicionAlojamiento:     "Alojamiento",
	reglas.CondicionSeguro:          "Seguro",
	reglas.CondicionTarjetaTurista:  "Tarjeta turista",
	reglas.CondicionPuertoDeEntrada: "Lugar de entrada",
	reglas.CondicionPaginasEnBlanco: "Páginas en blanco",
	reglas.CondicionOtra:            "Otra condición",
}

// condiciones evalúa lo demás que pide el país. Lo que el pasajero tiene que
// tener o hacer falta hasta que alguien lo confirme y va al aviso del cliente;
// el pasaje de regreso cumple si el viaje tiene regreso; por dónde se entra lo
// revisa la agencia contra los vuelos, y el seguro contra la asistencia médica
// que emite en todos los viajes. Una regla de antes de que se extrajeran
// condiciones no dice nada.
func (e evaluator) condiciones() []Requirement {
	f := e.f.Condiciones
	if f.Verify {
		r := e.verify(KeyCondiciones, "Otras condiciones", f)
		r.pide("No se sabe si %s pide otras condiciones para entrar: %s.", e.name, f.Reason)
		r.hacer("Revisar las condiciones de entrada de %s y confirmarlo.", e.name)
		r.Sources = append(r.Sources, f.Sources...)
		return []Requirement{r}
	}
	if f.Value == nil {
		return nil
	}
	var out []Requirement
	for i, c := range condicionKeys(*f.Value) {
		cond := (*f.Value)[i]
		title := condicionLabels[cond.Kind]
		if title == "" {
			title = condicionLabels[reglas.CondicionOtra]
		}
		r := e.req(c, Falta, title)
		r.Confirmable, r.FieldHash, r.Sources = true, hash(c, cond), condicionSources(f, i)
		r.pide("%s.", condicionText(e.name, cond))
		withTramite := true
		switch cond.Kind {
		case reglas.CondicionPasajeRegreso:
			if e.returnAt != nil {
				r.Status = Cumple
				r.tiene("El viaje tiene regreso el %s.", day(*e.returnAt))
				withTramite = false
			} else {
				r.tiene("El viaje no tiene fecha de regreso cargada.")
				r.hacer("Cargar la fecha de regreso, o conseguir un pasaje de salida del país.")
				r.ClientText = fmt.Sprintf("para %s hay que %s", e.name, lowerFirst(cond.Text))
			}
		case reglas.CondicionPuertoDeEntrada:
			r.Status = Verificar
			r.hacer("Revisar que los vuelos entren por ahí y confirmarlo.")
		case reglas.CondicionSeguro:
			r.Status = Verificar
			r.hacer("Revisar que la asistencia médica del viaje lo cubra y confirmarlo.")
		default:
			r.hacer("Avisarle al cliente y confirmarlo cuando esté resuelto.")
			r.ClientText = fmt.Sprintf("para %s hay que %s", e.name, lowerFirst(cond.Text))
		}
		e.official(&r, c, withTramite)
		out = append(out, r)
	}
	return out
}

func condicionKeys(cs []reglas.Condicion) []string { return reglas.CondicionKeys(cs) }

func condicionText(name string, c reglas.Condicion) string {
	return fmt.Sprintf("Para %s hay que %s", name, lowerFirst(c.Text))
}

// condicionSources da la cita de cada condición: se guardan una por una y en
// orden.
func condicionSources(f reglas.Field[[]reglas.Condicion], i int) []reglas.Citation {
	if len(f.Sources) == len(*f.Value) {
		return append([]reglas.Citation{}, f.Sources[i:i+1]...)
	}
	return append([]reglas.Citation{}, f.Sources...)
}

func (e evaluator) transito() Requirement {
	f := e.f.Visa
	var r Requirement
	switch {
	case f.Verify || f.Value == nil:
		r = e.verify(KeyTransito, "Tránsito", f)
		r.pide("No se sabe si %s pide visa de tránsito: %s.", e.name, f.Reason)
		r.hacer("Revisar con la aerolínea si la escala en %s necesita visa de tránsito y confirmarlo.", e.name)
	case *f.Value == reglas.VisaNoRequiere || *f.Value == reglas.VisaALaLlegada:
		r = e.req(KeyTransito, Cumple, "Tránsito")
		r.pide("%s no pide visa a quien viaja %s: la escala no necesita visa de tránsito.", e.name, e.nationality())
	default:
		r = e.verify(KeyTransito, "Tránsito", f)
		r.pide("%s pide visa a quien viaja %s: puede hacer falta visa de tránsito para la escala.", e.name, e.nationality())
		r.hacer("Revisar con la aerolínea si la escala en %s necesita visa de tránsito y confirmarlo.", e.name)
	}
	r.Sources = append(r.Sources, f.Sources...)
	e.official(&r, reglas.GrupoTransito, r.Status != Cumple)
	return r
}

func countryName(code string) string {
	if c, ok := reglas.CountryByCode(code); ok {
		return c.Name
	}
	return code
}

// riskOrigin dice qué se sabe del país de salida para una vacuna que se pide a
// quienes vienen de país de riesgo. No hay lista de países de riesgo: fuera de
// la Argentina, solo se dice cuál hay que mirar.
func riskOrigin(origin string) string {
	if origin == Argentina {
		return "el viaje sale de Argentina, que la OMS considera de riesgo en parte"
	}
	return fmt.Sprintf("el viaje sale de %s: hay que ver si es de riesgo", countryName(origin))
}

func kindLabel(kind string) string {
	switch kind {
	case "autorizacion_electronica":
		return "autorización electrónica"
	case "residencia":
		return "residencia"
	default:
		return "visa"
	}
}

func until(t *time.Time) string {
	if t == nil {
		return ""
	}
	return " vigente hasta el " + day(*t)
}

func day(t time.Time) string { return t.Format("02/01/2006") }

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToLower(string(r[0])) + string(r[1:])
}

func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func hash(parts ...any) string {
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}
