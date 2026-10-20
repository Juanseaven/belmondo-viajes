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
)

// Las claves de los requisitos. Las vacunas son «vacuna:<nombre>».
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
	vacunaPrefix        = "vacuna:"
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
	// Text es la frase para la agencia; ClientText, para el aviso al cliente.
	Text       string `json:"text"`
	ClientText string `json:"-"`
	// Confirmable: lo que el sistema no puede saber y una persona sí. Un dato
	// que falta no se confirma: se carga.
	Confirmable bool `json:"confirmable"`
	Confirmed   bool `json:"confirmed,omitempty"`
	// MissingData: falta porque no está cargado el dato, no porque no cumpla.
	MissingData bool              `json:"missingData,omitempty"`
	Sources     []reglas.Citation `json:"sources,omitempty"`
	// FieldHash ata una confirmación al valor de la regla con que se hizo.
	FieldHash string `json:"-"`
}

type Trip struct {
	DepartAt   *time.Time
	ReturnAt   *time.Time
	Itinerary  []Stop
	Passengers []Passenger
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
	Doc     *Document
	Permits []Permit
}

type Document struct {
	Type      string
	Country   string
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

// Pairs son las reglas que hacen falta para evaluar el viaje.
func Pairs(t Trip) []reglas.Pair {
	seen := map[reglas.Pair]bool{}
	var out []reglas.Pair
	for _, p := range t.Passengers {
		if p.Doc == nil {
			continue
		}
		for _, stop := range t.Itinerary {
			pair := reglas.Pair{Nationality: p.Doc.Country, Country: stop.Country}
			if stop.Country != p.Doc.Country && !seen[pair] {
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
		out = append(out, Requirement{Key: KeyItinerario, Status: Falta, MissingData: true,
			Text: "Falta cargar el país de destino del viaje"})
	}
	for _, p := range t.Passengers {
		if p.Minor {
			out = append(out, Requirement{PassengerID: p.ID, Key: KeyMenor, Status: Falta, Confirmable: true,
				FieldHash:  hash(KeyMenor),
				Text:       "Es menor: necesita la autorización de viaje si no viaja con ambos padres",
				ClientText: "al ser menor, necesita la autorización de viaje si no viaja con ambos padres"})
		}
		if p.Doc == nil {
			out = append(out, Requirement{PassengerID: p.ID, Key: KeyDocumentoDeViaje, Status: Falta, MissingData: true,
				Text: "Falta cargar el documento con el que viaja"})
			continue
		}
		for _, stop := range t.Itinerary {
			if stop.Country == p.Doc.Country {
				out = append(out, Requirement{PassengerID: p.ID, Country: stop.Country, Role: stop.Role,
					Key: KeyPaisPropio, Status: Cumple, Text: "Es su país: no necesita nada para entrar"})
				continue
			}
			f := reglas.AllVerify("buscando requisitos")
			if rule, ok := rules[reglas.Pair{Nationality: p.Doc.Country, Country: stop.Country}]; ok {
				f = rule.Effective()
			}
			e := evaluator{p: p, stop: stop, f: f, name: countryName(stop.Country)}
			e.arrive, e.leave = stopDates(t, stop)
			if stop.Role == "escala" {
				out = append(out, e.transito())
				continue
			}
			out = append(out, e.documento(), e.vigencia())
			if r, ok := e.antiguedad(); ok {
				out = append(out, r)
			}
			visa, level := e.visa()
			out = append(out, visa)
			if r, ok := e.estadia(level); ok {
				out = append(out, r)
			}
			out = append(out, e.vacunas()...)
		}
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
	name          string
	arrive, leave *time.Time
}

func (e evaluator) req(key string, status Status, text string) Requirement {
	return Requirement{PassengerID: e.p.ID, Country: e.stop.Country, Role: e.stop.Role, Key: key, Status: status, Text: text}
}

func (e evaluator) verify(key, text string, field any) Requirement {
	r := e.req(key, Verificar, text)
	r.Confirmable, r.FieldHash = true, hash(key, field)
	return r
}

func (e evaluator) missing(key, text string) Requirement {
	r := e.req(key, Falta, text)
	r.MissingData = true
	return r
}

func (e evaluator) documento() Requirement {
	f := e.f.Documento
	switch {
	case f.Verify:
		return e.verify(KeyDocumento, fmt.Sprintf("No se sabe con qué documento se entra a %s: %s", e.name, f.Reason), f)
	case f.Value != nil && *f.Value == reglas.DocumentoPasaporte && e.p.Doc.Type == "DNI":
		r := e.req(KeyDocumento, Falta, fmt.Sprintf("Viaja con DNI y %s pide pasaporte", e.name))
		r.ClientText = fmt.Sprintf("para entrar a %s necesita pasaporte, no alcanza con el DNI", e.name)
		r.Sources = f.Sources
		return r
	}
	r := e.req(KeyDocumento, Cumple, fmt.Sprintf("El %s alcanza para entrar a %s", e.p.Doc.Type, e.name))
	r.Sources = f.Sources
	return r
}

func (e evaluator) vigencia() Requirement {
	f := e.f.Vigencia
	doc := e.p.Doc
	switch {
	case doc.ExpiresAt == nil:
		return e.missing(KeyVigencia, fmt.Sprintf("Falta cargar el vencimiento del %s", doc.Type))
	case f.Verify || f.Value == nil:
		reason := f.Reason
		if reason == "" {
			reason = "ninguna fuente lo dice"
		}
		return e.verify(KeyVigencia, fmt.Sprintf("No se sabe cuánta vigencia del documento pide %s: %s", e.name, reason), f)
	}
	v := *f.Value
	base := e.arrive
	if v.Base == "salida_del_pais" {
		base = e.leave
	}
	if base == nil {
		return e.missing(KeyVigencia, "Faltan las fechas del viaje para medir la vigencia del documento")
	}
	required := base.AddDate(0, 0, v.Amount)
	if v.Unit == "meses" {
		required = base.AddDate(0, v.Amount, 0)
	}
	if doc.ExpiresAt.Before(required) {
		r := e.req(KeyVigencia, Falta, fmt.Sprintf("El %s vence el %s y %s pide que dure hasta el %s",
			doc.Type, day(*doc.ExpiresAt), e.name, day(required)))
		r.ClientText = fmt.Sprintf("el %s vence el %s y para %s tiene que durar hasta el %s: hay que renovarlo",
			doc.Type, day(*doc.ExpiresAt), e.name, day(required))
		r.Sources = f.Sources
		return r
	}
	r := e.req(KeyVigencia, Cumple, fmt.Sprintf("El %s vence el %s; %s pide que dure hasta el %s",
		doc.Type, day(*doc.ExpiresAt), e.name, day(required)))
	r.Sources = f.Sources
	return r
}

func (e evaluator) antiguedad() (Requirement, bool) {
	f := e.f.EmisionMaxAnios
	switch {
	case f.Verify:
		return e.verify(KeyAntiguedad, fmt.Sprintf("No se sabe si %s pide un pasaporte reciente: %s", e.name, f.Reason), f), true
	case f.Value == nil:
		return Requirement{}, false
	case e.p.Doc.IssuedAt == nil:
		return e.missing(KeyAntiguedad, fmt.Sprintf("Falta cargar la fecha de emisión del %s: %s pide que tenga menos de %d años",
			e.p.Doc.Type, e.name, *f.Value)), true
	case e.arrive == nil:
		return e.missing(KeyAntiguedad, "Faltan las fechas del viaje para medir la antigüedad del documento"), true
	}
	issued, years := *e.p.Doc.IssuedAt, *f.Value
	if issued.AddDate(years, 0, 0).Before(*e.arrive) {
		r := e.req(KeyAntiguedad, Falta, fmt.Sprintf("El %s se emitió el %s y %s pide que tenga menos de %d años",
			e.p.Doc.Type, day(issued), e.name, years))
		r.ClientText = fmt.Sprintf("el %s tiene más de %d años y %s no lo acepta: hay que renovarlo", e.p.Doc.Type, years, e.name)
		r.Sources = f.Sources
		return r, true
	}
	r := e.req(KeyAntiguedad, Cumple, fmt.Sprintf("El %s se emitió el %s: tiene menos de %d años", e.p.Doc.Type, day(issued), years))
	r.Sources = f.Sources
	return r, true
}

var visaLabels = map[reglas.Visa]string{
	reglas.VisaAutorizacionElectronica: "autorización electrónica",
	reglas.VisaElectronica:             "visa electrónica",
	reglas.VisaConsular:                "visa",
}

// visa devuelve el requisito y lo que se termina pidiendo después de aplicar
// las exenciones.
func (e evaluator) visa() (Requirement, reglas.Visa) {
	f := e.f.Visa
	if f.Verify || f.Value == nil {
		return e.verify(KeyVisa, fmt.Sprintf("No se sabe qué visa pide %s: %s", e.name, f.Reason), f), ""
	}
	level := *f.Value
	var exemptBy *Permit
	if level != reglas.VisaNoRequiere && level != reglas.VisaALaLlegada && level != reglas.VisaNoAdmitido &&
		e.f.Exenciones.Value != nil {
		for _, ex := range *e.f.Exenciones.Value {
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

	r := e.req(KeyVisa, Cumple, "")
	r.Sources = f.Sources
	switch level {
	case reglas.VisaNoRequiere:
		r.Text = fmt.Sprintf("%s no pide visa", e.name)
		if via != "" {
			r.Text = fmt.Sprintf("%s, %s no pide visa", capitalize(via), e.name)
		}
		return r, level
	case reglas.VisaALaLlegada:
		r.Text = fmt.Sprintf("La visa de %s se tramita al llegar", e.name)
		return r, level
	case reglas.VisaNoAdmitido:
		r.Status = Falta
		r.Text = fmt.Sprintf("%s no admite pasajeros con este documento", e.name)
		r.ClientText = fmt.Sprintf("%s no admite el ingreso con este documento", e.name)
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
			r.Text = fmt.Sprintf("Tiene %s de %s%s", kindLabel(pm.Kind), e.name, until(pm.ExpiresAt))
			return r, level
		}
		expiring = &pm
	}

	r.Status = Falta
	if expiring != nil && expiring.ExpiresAt != nil {
		r.Text = fmt.Sprintf("La %s de %s vence el %s, antes de terminar el viaje", kindLabel(expiring.Kind), e.name, day(*expiring.ExpiresAt))
		r.ClientText = fmt.Sprintf("la %s de %s vence el %s, antes de terminar el viaje: hay que renovarla",
			kindLabel(expiring.Kind), e.name, day(*expiring.ExpiresAt))
		return r, level
	}
	if via != "" {
		r.Text = fmt.Sprintf("%s alcanza con la %s de %s, y no está cargada", capitalize(via), label, e.name)
		r.ClientText = fmt.Sprintf("%s alcanza con la %s de %s, que se tramita antes de viajar", via, label, e.name)
		return r, level
	}
	r.Text = fmt.Sprintf("%s pide %s y no está cargada", e.name, label)
	r.ClientText = fmt.Sprintf("para %s necesita %s, que se tramita antes de viajar", e.name, label)
	return r, level
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

func (e evaluator) estadia(level reglas.Visa) (Requirement, bool) {
	f := e.f.EstadiaMaxDias
	if f.Value == nil || (level != reglas.VisaNoRequiere && level != reglas.VisaALaLlegada) ||
		e.arrive == nil || e.leave == nil {
		return Requirement{}, false
	}
	days := int(e.leave.Sub(*e.arrive).Hours()/24) + 1
	if days <= *f.Value {
		return Requirement{}, false
	}
	r := e.req(KeyEstadia, Falta, fmt.Sprintf("Se queda %d días y %s permite %d sin visa", days, e.name, *f.Value))
	r.ClientText = fmt.Sprintf("la estadía en %s es de %d días y el máximo sin visa es %d", e.name, days, *f.Value)
	r.Sources = f.Sources
	return r, true
}

func (e evaluator) vacunas() []Requirement {
	f := e.f.Vacunas
	if f.Verify || f.Value == nil {
		return []Requirement{e.verify(KeyVacunas, fmt.Sprintf("No se sabe qué vacunas pide %s: %s", e.name, f.Reason), f)}
	}
	var out []Requirement
	for _, v := range *f.Value {
		key := vacunaPrefix + v.Name
		r := e.req(key, Falta, fmt.Sprintf("%s pide la vacuna de %s", e.name, v.Name))
		r.Confirmable, r.FieldHash, r.Sources = true, hash(key, v), f.Sources
		r.ClientText = fmt.Sprintf("para %s necesita la vacuna de %s, con su certificado", e.name, v.Name)
		if v.Scope == reglas.VacunaDesdePaisRiesgo {
			r.Status = Verificar
			r.Text = fmt.Sprintf("%s pide la vacuna de %s a quienes llegan desde países de riesgo; la OMS considera de riesgo parte de la Argentina",
				e.name, v.Name)
		}
		out = append(out, r)
	}
	return out
}

func (e evaluator) transito() Requirement {
	f := e.f.Visa
	if f.Verify || f.Value == nil {
		return e.verify(KeyTransito, fmt.Sprintf("No se sabe si %s pide visa de tránsito: %s", e.name, f.Reason), f)
	}
	if *f.Value == reglas.VisaNoRequiere || *f.Value == reglas.VisaALaLlegada {
		r := e.req(KeyTransito, Cumple, fmt.Sprintf("%s no pide visa: la escala no necesita visa de tránsito", e.name))
		r.Sources = f.Sources
		return r
	}
	return e.verify(KeyTransito, fmt.Sprintf("%s pide visa a esta nacionalidad: puede hacer falta visa de tránsito para la escala", e.name), f)
}

func countryName(code string) string {
	if c, ok := reglas.CountryByCode(code); ok {
		return c.Name
	}
	return code
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
