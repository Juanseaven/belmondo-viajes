package evaluacion

import (
	"fmt"
	"strings"

	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// PreviewLine es una línea de lo que pide un país, sin pasajero contra el cual
// medirla: no dice cumple ni falta. Verify marca lo que no se sabe; Searching,
// que la regla todavía se está buscando.
type PreviewLine struct {
	Country   string            `json:"country"`
	Role      string            `json:"role"`
	Key       string            `json:"key"`
	Text      string            `json:"text"`
	Verify    bool              `json:"verify,omitempty"`
	Searching bool              `json:"searching,omitempty"`
	Sources   []reglas.Citation `json:"sources,omitempty"`
	// Detail lleva lo que suman las páginas oficiales: la regla explicada y el
	// trámite. No tiene «tiene» ni «hacer»: no hay pasajero.
	Detail Detail `json:"detail"`
}

// Preview dice qué piden los países del itinerario a un pasaporte argentino.
// Es para el momento en que llega la consulta y todavía no hay pasajeros: con
// el primero, el panel pasa a la evaluación de siempre. No se guarda ni cierra
// controles.
func Preview(t Trip, rules map[reglas.Pair]reglas.Rule) []PreviewLine {
	if len(t.Passengers) > 0 {
		return nil
	}
	var out []PreviewLine
	for _, stop := range t.Itinerary {
		p := previewer{stop: stop, name: countryName(stop.Country), origin: t.origin()}
		if stop.Country == PreviewNationality {
			out = append(out, p.line(KeyPaisPropio, "Es su país: no necesita nada para entrar", false, nil))
			continue
		}
		rule, ok := rules[reglas.Pair{Nationality: PreviewNationality, Country: stop.Country}]
		if !ok || rule.Status == reglas.RuleBuscando {
			out = append(out, PreviewLine{Country: stop.Country, Role: stop.Role, Key: "buscando", Searching: true,
				Text: fmt.Sprintf("Buscando los requisitos de %s", p.name)})
			continue
		}
		p.f = rule.Effective()
		var lines []PreviewLine
		if stop.Role == "escala" {
			lines = append(lines, p.transito())
		} else {
			lines = append(lines, p.documento()...)
			lines = append(lines, p.visa()...)
			lines = append(lines, p.estadia()...)
			lines = append(lines, withReason(p.vigencia(), p.f.Vigencia.Reason))
			lines = append(lines, p.antiguedad()...)
			lines = append(lines, p.vacunas()...)
			lines = append(lines, p.condiciones()...)
		}
		out = append(out, withOfficial(lines, rule.Details)...)
	}
	return out
}

// withOfficial suma a cada línea el detalle de las páginas oficiales de su
// grupo. La visa puede tener varias líneas (una por exención): el detalle va
// en la primera.
func withOfficial(lines []PreviewLine, det reglas.Detalles) []PreviewLine {
	done := map[string]bool{}
	for i := range lines {
		l := &lines[i]
		group := groupFor(l.Key)
		if group == "" || (done[group] && l.Key == KeyVisa) {
			continue
		}
		done[group] = true
		g, ok := det.Grupos[group]
		if !ok {
			if det.Status == reglas.DetallesBuscando || det.Status == reglas.DetallesSinDatos {
				l.Detail.Oficial = det.Status
			}
			continue
		}
		for _, fr := range g.Pide {
			l.Detail.Pide = append(l.Detail.Pide, sourced(fr))
			l.Sources = appendSource(l.Sources, fr.Source)
		}
		for _, fr := range g.Tramite {
			l.Detail.Tramite = append(l.Detail.Tramite, sourced(fr))
			l.Sources = appendSource(l.Sources, fr.Source)
		}
	}
	return lines
}

// groupFor dice de qué grupo del detalle oficial es una clave de requisito.
func groupFor(key string) string {
	switch {
	case key == KeyDocumento:
		return reglas.GrupoDocumento
	case key == KeyVisa:
		return reglas.GrupoVisa
	case key == KeyVigencia:
		return reglas.GrupoVigencia
	case key == KeyAntiguedad:
		return reglas.GrupoAntiguedad
	case key == KeyEstadia:
		return reglas.GrupoEstadia
	case key == KeyTransito:
		return reglas.GrupoTransito
	case strings.HasPrefix(key, vacunaPrefix):
		return reglas.GrupoVacunas
	case strings.HasPrefix(key, condicionPrefix):
		return key
	}
	return ""
}

type previewer struct {
	stop   Stop
	name   string
	origin string
	f      reglas.Fields
}

func (p previewer) line(key, text string, verify bool, sources []reglas.Citation) PreviewLine {
	return PreviewLine{Country: p.stop.Country, Role: p.stop.Role, Key: key, Text: text, Verify: verify, Sources: sources}
}

func (p previewer) documento() []PreviewLine {
	f := p.f.Documento
	switch {
	case f.Verify:
		return []PreviewLine{p.line(KeyDocumento, fmt.Sprintf("No se sabe con qué documento se entra a %s: %s", p.name, f.Reason), true, f.Sources)}
	case f.Value == nil:
		return nil
	case *f.Value == reglas.DocumentoDNIOPasaporte:
		return []PreviewLine{p.line(KeyDocumento, fmt.Sprintf("A %s se entra con DNI o pasaporte", p.name), false, f.Sources)}
	}
	return []PreviewLine{p.line(KeyDocumento, fmt.Sprintf("%s pide pasaporte: no alcanza con el DNI", p.name), false, f.Sources)}
}

func (p previewer) visa() []PreviewLine {
	f := p.f.Visa
	if f.Verify || f.Value == nil {
		return []PreviewLine{p.line(KeyVisa, fmt.Sprintf("No se sabe qué visa pide %s: %s", p.name, f.Reason), true, f.Sources)}
	}
	var text string
	switch *f.Value {
	case reglas.VisaNoRequiere:
		text = fmt.Sprintf("%s no pide visa", p.name)
	case reglas.VisaALaLlegada:
		text = fmt.Sprintf("La visa de %s se tramita al llegar", p.name)
	case reglas.VisaNoAdmitido:
		text = fmt.Sprintf("%s no admite pasajeros con pasaporte argentino", p.name)
	default:
		text = fmt.Sprintf("%s pide %s, que se tramita antes de viajar", p.name, visaLabels[*f.Value])
	}
	out := []PreviewLine{p.line(KeyVisa, text, false, f.Sources)}
	if *f.Value == reglas.VisaNoRequiere || *f.Value == reglas.VisaALaLlegada || *f.Value == reglas.VisaNoAdmitido ||
		p.f.Exenciones.Value == nil {
		return out
	}
	for _, ex := range *p.f.Exenciones.Value {
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
		out = append(out, p.line(KeyVisa, fmt.Sprintf("Con %s de %s vigente, %s", kinds, countryName(ex.PermitCountry), needs),
			false, p.f.Exenciones.Sources))
	}
	return out
}

func (p previewer) estadia() []PreviewLine {
	f, visa := p.f.EstadiaMaxDias, p.f.Visa.Value
	if f.Value == nil || visa == nil || (*visa != reglas.VisaNoRequiere && *visa != reglas.VisaALaLlegada) {
		return nil
	}
	return []PreviewLine{p.line(KeyEstadia, fmt.Sprintf("Permite quedarse hasta %d días sin visa", *f.Value), false, f.Sources)}
}

func (p previewer) vigencia() PreviewLine {
	f := p.f.Vigencia
	if f.Verify || f.Value == nil {
		reason := f.Reason
		if reason == "" {
			reason = "ninguna fuente lo dice"
		}
		return p.line(KeyVigencia, fmt.Sprintf("No se sabe cuánta vigencia del pasaporte pide %s: %s", p.name, reason), true, f.Sources)
	}
	v := *f.Value
	if v.Amount == 0 {
		if v.Base == "salida_del_pais" {
			return p.line(KeyVigencia, fmt.Sprintf("El pasaporte tiene que estar vigente toda la estadía en %s", p.name), false, f.Sources)
		}
		return p.line(KeyVigencia, fmt.Sprintf("El pasaporte tiene que estar vigente al llegar a %s", p.name), false, f.Sources)
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
		from = fmt.Sprintf("después de salir de %s", p.name)
		if reglas.InSchengen(p.stop.Country) {
			from = "después de salir del espacio Schengen"
		}
	}
	return p.line(KeyVigencia, fmt.Sprintf("El pasaporte tiene que durar %d %s %s", v.Amount, unit, from), false, f.Sources)
}

// withReason suma a la línea el motivo de una regla fija: de dónde sale el
// valor («Argentina está en la lista de exceptuados»).
func withReason(l PreviewLine, reason string) PreviewLine {
	if reason != "" {
		l.Detail.Pide = append(l.Detail.Pide, Line{Text: capitalize(reason) + "."})
	}
	return l
}

func (p previewer) antiguedad() []PreviewLine {
	f := p.f.EmisionMaxAnios
	switch {
	case f.Verify:
		return []PreviewLine{p.line(KeyAntiguedad, fmt.Sprintf("No se sabe si %s pide un pasaporte reciente: %s", p.name, f.Reason), true, f.Sources)}
	case f.Value == nil:
		return nil
	}
	return []PreviewLine{p.line(KeyAntiguedad, fmt.Sprintf("El pasaporte tiene que tener menos de %d años", *f.Value), false, f.Sources)}
}

func (p previewer) vacunas() []PreviewLine {
	f := p.f.Vacunas
	if f.Verify || f.Value == nil {
		return []PreviewLine{p.line(KeyVacunas, fmt.Sprintf("No se sabe qué vacunas pide %s: %s", p.name, f.Reason), true, f.Sources)}
	}
	if len(*f.Value) == 0 {
		return []PreviewLine{p.line(KeyVacunas, fmt.Sprintf("%s no pide vacunas", p.name), false, f.Sources)}
	}
	var out []PreviewLine
	for _, v := range *f.Value {
		if v.Scope == reglas.VacunaDesdePaisRiesgo {
			out = append(out, p.line(vacunaPrefix+v.Name, fmt.Sprintf("%s pide la vacuna de %s a quienes llegan desde países de riesgo; %s",
				p.name, v.Name, riskOrigin(p.origin)), true, f.Sources))
			continue
		}
		out = append(out, p.line(vacunaPrefix+v.Name, fmt.Sprintf("%s pide la vacuna de %s, con su certificado", p.name, v.Name), false, f.Sources))
	}
	return out
}

func (p previewer) condiciones() []PreviewLine {
	f := p.f.Condiciones
	if f.Verify {
		return []PreviewLine{p.line(KeyCondiciones, fmt.Sprintf("No se sabe si %s pide otras condiciones para entrar: %s", p.name, f.Reason), true, f.Sources)}
	}
	if f.Value == nil {
		return nil
	}
	var out []PreviewLine
	for i, key := range condicionKeys(*f.Value) {
		out = append(out, p.line(key, condicionText(p.name, (*f.Value)[i]), false, condicionSources(f, i)))
	}
	return out
}

func (p previewer) transito() PreviewLine {
	f := p.f.Visa
	switch {
	case f.Verify || f.Value == nil:
		return p.line(KeyTransito, fmt.Sprintf("No se sabe si %s pide visa de tránsito: %s", p.name, f.Reason), true, f.Sources)
	case *f.Value == reglas.VisaNoRequiere || *f.Value == reglas.VisaALaLlegada:
		return p.line(KeyTransito, fmt.Sprintf("%s no pide visa: la escala no necesita visa de tránsito", p.name), false, f.Sources)
	}
	return p.line(KeyTransito, fmt.Sprintf("%s pide visa a pasaportes argentinos: puede hacer falta visa de tránsito para la escala", p.name), true, f.Sources)
}
