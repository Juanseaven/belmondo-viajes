package evaluacion

import (
	"strings"
	"testing"

	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// said junta todo lo que dice un renglón: el título y el detalle.
func said(r Requirement) string {
	parts := []string{r.Text}
	for _, l := range r.Detail.Pide {
		parts = append(parts, l.Text)
	}
	for _, l := range r.Detail.Tramite {
		parts = append(parts, l.Text)
	}
	parts = append(parts, r.Detail.Tiene...)
	parts = append(parts, r.Detail.Hacer...)
	return strings.Join(parts, "\n")
}

func joined(lines []string) string { return strings.Join(lines, "\n") }

func pide(r Requirement) string {
	var out []string
	for _, l := range r.Detail.Pide {
		out = append(out, l.Text)
	}
	return joined(out)
}

var pasaporteBeto = &Document{Type: "Pasaporte", Country: "AR", Number: "AAB123456",
	IssuedAt: d("2021-05-12"), ExpiresAt: d("2031-05-12")}

func TestCadaRenglonTieneElNombreDelRequisitoYExplicaConLosDatosDelPasajero(t *testing.T) {
	rules := regla("AR", "US", func(f *reglas.Fields) {
		f.Visa = field(reglas.VisaConsular)
		f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"})
	})
	reqs := Evaluate(viaje("US", ana(pasaporteBeto)), rules, nil)

	doc := find(t, reqs, "ana", "US", KeyDocumento)
	if doc.Text != "Documento" || doc.Status != Cumple {
		t.Errorf("documento: %+v", doc)
	}
	if !strings.Contains(pide(doc), "pasaporte") || !strings.Contains(pide(doc), "DNI") {
		t.Errorf("el documento dice qué pide el país y por qué: %q", pide(doc))
	}
	if tiene := joined(doc.Detail.Tiene); !strings.Contains(tiene, "AAB123456") || !strings.Contains(tiene, "12/05/2031") ||
		!strings.Contains(tiene, "12/05/2021") {
		t.Errorf("el documento dice con qué viaja: %q", tiene)
	}
	if len(doc.Detail.Hacer) != 0 {
		t.Errorf("lo que cumple no tiene nada que hacer: %q", doc.Detail.Hacer)
	}

	vig := find(t, reqs, "ana", "US", KeyVigencia)
	if vig.Text != "Vigencia del pasaporte" || !strings.Contains(pide(vig), "toda la estadía") ||
		!strings.Contains(pide(vig), "24/12/2026") {
		t.Errorf("vigencia: título %q, pide %q", vig.Text, pide(vig))
	}

	visa := find(t, reqs, "ana", "US", KeyVisa)
	if visa.Text != "Visa" || visa.Status != Falta {
		t.Errorf("visa: %+v", visa)
	}
	if !strings.Contains(pide(visa), "visa consular") {
		t.Errorf("la visa dice qué pide: %q", pide(visa))
	}
	if !strings.Contains(joined(visa.Detail.Tiene), "No tiene visa de Estados Unidos cargada") {
		t.Errorf("la visa dice qué tiene: %q", visa.Detail.Tiene)
	}
	if hacer := joined(visa.Detail.Hacer); !strings.Contains(hacer, "cargarla") || !strings.Contains(hacer, "tramitarla") {
		t.Errorf("la visa dice qué hacer: %q", hacer)
	}
}

// Sin fechas no se puede medir la vigencia, pero sí decir hasta cuándo cumple.
func TestLaVigenciaSinFechasDiceHastaCuandoCumpliria(t *testing.T) {
	rules := regla("AR", "US", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"})
	})
	trip := viaje("US", ana(pasaporteBeto))
	trip.DepartAt, trip.ReturnAt = nil, nil

	r := find(t, Evaluate(trip, rules, nil), "ana", "US", KeyVigencia)
	if r.Status != Falta || !r.MissingData {
		t.Fatalf("sin fechas: %+v", r)
	}
	if !strings.Contains(joined(r.Detail.Tiene), "12/05/2031") || !strings.Contains(joined(r.Detail.Tiene), "fechas") {
		t.Errorf("tiene: %q", r.Detail.Tiene)
	}
	if hacer := joined(r.Detail.Hacer); !strings.Contains(hacer, "fechas del viaje") || !strings.Contains(hacer, "12/05/2031") {
		t.Errorf("hacer: %q", hacer)
	}

	seisMeses := regla("AR", "TH", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 6, Unit: "meses", Base: "llegada"})
	})
	trip = viaje("TH", ana(pasaporteBeto))
	trip.DepartAt, trip.ReturnAt = nil, nil
	r = find(t, Evaluate(trip, seisMeses, nil), "ana", "TH", KeyVigencia)
	if hacer := joined(r.Detail.Hacer); !strings.Contains(hacer, "12/11/2030") {
		t.Errorf("con 6 meses desde la llegada, cumple si llega hasta el 12/11/2030: %q", hacer)
	}
}

func TestRecomiendaRenovarElPasaporteQueVenceAMenosDeSeisMesesDeLaVuelta(t *testing.T) {
	rules := regla("AR", "BR", nil)
	pronto := &Document{Type: "Pasaporte", Country: "AR", ExpiresAt: d("2027-04-24")}

	reqs := Evaluate(viaje("BR", ana(pronto)), rules, nil)
	r := find(t, reqs, "ana", "", KeyRenovar)
	if r.Status != Aviso || r.Confirmable || r.Text != "Renovación del pasaporte" {
		t.Fatalf("vence 4 meses después de la vuelta: %+v", r)
	}
	if !strings.Contains(said(r), "24/04/2027") || !strings.Contains(said(r), "24/12/2026") {
		t.Errorf("dice las dos fechas: %q", said(r))
	}

	lejos := &Document{Type: "Pasaporte", Country: "AR", ExpiresAt: d("2027-07-25")}
	if has(Evaluate(viaje("BR", ana(lejos)), rules, nil), "ana", "", KeyRenovar) {
		t.Error("vence 7 meses después de la vuelta: no hace falta recomendar")
	}

	sinVuelta := viaje("BR", ana(pronto))
	sinVuelta.ReturnAt = nil
	if has(Evaluate(sinVuelta, rules, nil), "ana", "", KeyRenovar) {
		t.Error("sin fecha de regreso no se calcula")
	}

	// Si ya no cumple la vigencia de un país, ese renglón está en falta: la
	// recomendación sobra.
	schengen := regla("AR", "ES", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 3, Unit: "meses", Base: "salida_del_pais"})
	})
	corto := &Document{Type: "Pasaporte", Country: "AR", ExpiresAt: d("2027-02-01")}
	if has(Evaluate(viaje("ES", ana(corto)), schengen, nil), "ana", "", KeyRenovar) {
		t.Error("con la vigencia en falta no se agrega la recomendación")
	}
}

// La recomendación no es un faltante: el aviso dice que está todo en orden y
// la recomienda igual.
func TestLaRecomendacionVaAlAvisoAunqueTodoEsteEnOrden(t *testing.T) {
	rules := regla("AR", "BR", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"})
	})
	pronto := &Document{Type: "Pasaporte", Country: "AR", ExpiresAt: d("2027-04-24")}
	trip := viaje("BR", ana(pronto))

	msg := Notice("Ana", trip, Evaluate(trip, rules, nil))
	if !strings.Contains(msg, "en orden") || !strings.Contains(msg, "recomendamos renovar el pasaporte de Ana") ||
		!strings.Contains(msg, "24/04/2027") {
		t.Errorf("aviso:\n%s", msg)
	}
}

func detalleOficial(status reglas.DetallesStatus, grupos map[string]reglas.Grupo) func(*reglas.Rule) {
	return func(r *reglas.Rule) { r.Details = reglas.Detalles{Status: status, Grupos: grupos} }
}

func conDetalle(rules map[reglas.Pair]reglas.Rule, mod func(*reglas.Rule)) map[reglas.Pair]reglas.Rule {
	for k, r := range rules {
		mod(&r)
		rules[k] = r
	}
	return rules
}

var travelState = reglas.Citation{Source: reglas.SourceOficial, URL: "https://travel.state.gov/visa", Quote: "B-2 visa",
	ReadAt: "2026-10-21"}

func TestElDetalleDeLasPaginasOficialesSeSumaAlRenglon(t *testing.T) {
	rules := conDetalle(regla("AR", "US", func(f *reglas.Fields) {
		f.Visa = field(reglas.VisaConsular)
		f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"})
	}), detalleOficial(reglas.DetallesListo, map[string]reglas.Grupo{
		reglas.GrupoVisa: {Tramite: []reglas.Frase{
			{Text: "Para turismo es la visa B1/B2: se completa el formulario DS-160 online y hay una entrevista en la embajada.",
				Source: travelState},
		}},
		reglas.GrupoVigencia: {Pide: []reglas.Frase{
			{Text: "En general pide 6 meses, pero la Argentina está exceptuada.", Source: travelState},
		}},
	}))
	trip := viaje("US", ana(pasaporteBeto))
	reqs := Evaluate(trip, rules, nil)

	visa := find(t, reqs, "ana", "US", KeyVisa)
	if len(visa.Detail.Tramite) != 1 || visa.Detail.Tramite[0].Source == nil ||
		visa.Detail.Tramite[0].Source.URL != travelState.URL {
		t.Fatalf("trámite: %+v", visa.Detail.Tramite)
	}
	if !hasSource(visa.Sources, travelState.URL) {
		t.Errorf("la página oficial va entre las fuentes del renglón: %+v", visa.Sources)
	}
	vig := find(t, reqs, "ana", "US", KeyVigencia)
	if !strings.Contains(pide(vig), "exceptuada") {
		t.Errorf("la vigencia suma la explicación oficial: %q", pide(vig))
	}
	if vig.Detail.Oficial != "" {
		t.Errorf("con el detalle listo no se avisa nada: %q", vig.Detail.Oficial)
	}

	// Al cliente le llega cómo se tramita.
	if msg := Notice("Ana", trip, reqs); !strings.Contains(msg, "DS-160") {
		t.Errorf("el aviso suma el trámite:\n%s", msg)
	}
}

func TestSinElDetalleOficialElRenglonEstaCompletoYDiceQueSeBusca(t *testing.T) {
	rules := conDetalle(regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
		detalleOficial(reglas.DetallesBuscando, nil))
	visa := find(t, Evaluate(viaje("US", ana(pasaporteBeto)), rules, nil), "ana", "US", KeyVisa)
	if visa.Detail.Oficial != reglas.DetallesBuscando || len(visa.Detail.Tramite) != 0 {
		t.Fatalf("buscando: %+v", visa.Detail)
	}
	if len(visa.Detail.Pide) == 0 || len(visa.Detail.Tiene) == 0 || len(visa.Detail.Hacer) == 0 {
		t.Errorf("el porqué no depende de la búsqueda: %+v", visa.Detail)
	}

	sinDatos := conDetalle(regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
		detalleOficial(reglas.DetallesSinDatos, nil))
	if r := find(t, Evaluate(viaje("US", ana(pasaporteBeto)), sinDatos, nil), "ana", "US", KeyVisa); r.Detail.Oficial != reglas.DetallesSinDatos {
		t.Errorf("sin datos: %+v", r.Detail)
	}
}

// Una regla que todavía se busca marca sus renglones, para que la ficha diga
// «buscando» sin leer la frase.
func TestLosRenglonesDeUnaReglaQueSeBuscaLoDicen(t *testing.T) {
	reqs := Evaluate(viaje("JP", ana(pasaporteAR)), map[reglas.Pair]reglas.Rule{}, nil)
	if r := find(t, reqs, "ana", "JP", KeyVisa); !r.Searching {
		t.Fatalf("sin regla: %+v", r)
	}
	if r := find(t, Evaluate(viaje("JP", ana(pasaporteAR)), regla("AR", "JP", nil), nil), "ana", "JP", KeyVisa); r.Searching {
		t.Fatalf("con regla: %+v", r)
	}
}

func TestLaEstadiaSeMuestraAunqueCumplaYQuedaEnVerificarSiNoSeSabe(t *testing.T) {
	rules := regla("AR", "TH", func(f *reglas.Fields) { f.EstadiaMaxDias = field(30) })
	r := find(t, Evaluate(viaje("TH", ana(pasaporteAR)), rules, nil), "ana", "TH", KeyEstadia)
	if r.Status != Cumple || r.Text != "Estadía" || !strings.Contains(said(r), "15 días") || !strings.Contains(said(r), "30") {
		t.Fatalf("estadía corta: %+v", r)
	}

	dudosa := regla("AR", "TH", func(f *reglas.Fields) {
		f.EstadiaMaxDias = verificar[int]("las fuentes no coinciden")
	})
	r = find(t, Evaluate(viaje("TH", ana(pasaporteAR)), dudosa, nil), "ana", "TH", KeyEstadia)
	if r.Status != Verificar || !r.Confirmable || !strings.Contains(said(r), "no coinciden") {
		t.Fatalf("estadía en verificar: %+v", r)
	}
}

func hasSource(sources []reglas.Citation, url string) bool {
	for _, s := range sources {
		if s.URL == url {
			return true
		}
	}
	return false
}

// Una regla fija explica de dónde sale el valor: el renglón lo dice.
func TestLaVigenciaDiceElMotivoDeLaRegla(t *testing.T) {
	rules := regla("AR", "US", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"})
		f.Vigencia.Reason = "En general pide 6 meses; la Argentina está exceptuada"
	})
	r := find(t, Evaluate(viaje("US", ana(pasaporteBeto)), rules, nil), "ana", "US", KeyVigencia)
	if !strings.Contains(pide(r), "Argentina está exceptuada") {
		t.Fatalf("pide: %q", pide(r))
	}
}
