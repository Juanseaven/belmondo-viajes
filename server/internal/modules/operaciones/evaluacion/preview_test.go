package evaluacion

import (
	"strings"
	"testing"

	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

func TestSinPasajerosSePidenLasReglasDelPasaporteArgentino(t *testing.T) {
	trip := Trip{Itinerary: []Stop{{Country: "PA", Role: "escala"}, {Country: "MX", Role: "destino"}}}
	pairs := Pairs(trip)
	if len(pairs) != 2 || pairs[0] != (reglas.Pair{Nationality: "AR", Country: "PA"}) ||
		pairs[1] != (reglas.Pair{Nationality: "AR", Country: "MX"}) {
		t.Fatalf("pares = %+v", pairs)
	}

	// Con pasajeros, son las de sus documentos.
	trip.Passengers = []Passenger{ana(&Document{Type: "Pasaporte", Country: "IT"})}
	for _, pair := range Pairs(trip) {
		if pair.Nationality != "IT" {
			t.Fatalf("con un pasajero italiano: %+v", Pairs(trip))
		}
	}
}

func lineFor(t *testing.T, lines []PreviewLine, country, key string) PreviewLine {
	t.Helper()
	for _, l := range lines {
		if l.Country == country && l.Key == key {
			return l
		}
	}
	t.Fatalf("no hay línea %s/%s en %+v", country, key, lines)
	return PreviewLine{}
}

func TestLaVistaPreviaDiceLoQuePideElPaisSinCumpleNiFalta(t *testing.T) {
	rules := merge(
		regla("AR", "CA", func(f *reglas.Fields) {
			f.Visa = field(reglas.VisaConsular)
			f.Exenciones = field([]reglas.Exencion{{PermitCountry: "US", Visa: reglas.VisaAutorizacionElectronica}})
			f.Vigencia = field(reglas.Vigencia{Amount: 6, Unit: "meses", Base: "llegada"})
		}),
		regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
	)
	trip := Trip{Itinerary: []Stop{{Country: "US", Role: "escala"}, {Country: "CA", Role: "destino"}}}
	lines := Preview(trip, rules)

	if l := lineFor(t, lines, "CA", KeyVisa); l.Verify || !strings.Contains(l.Text, "pide visa") || len(l.Sources) == 0 {
		t.Fatalf("visa: %+v", l)
	}
	var exencion bool
	for _, l := range lines {
		if l.Key == KeyVisa && strings.Contains(l.Text, "Estados Unidos") && strings.Contains(l.Text, "autorización electrónica") {
			exencion = true
		}
	}
	if !exencion {
		t.Errorf("falta la exención por visa de EE.UU.: %+v", lines)
	}
	if l := lineFor(t, lines, "CA", KeyVigencia); !strings.Contains(l.Text, "6 meses desde la llegada") {
		t.Errorf("vigencia: %+v", l)
	}
	if l := lineFor(t, lines, "CA", KeyDocumento); !strings.Contains(l.Text, "pasaporte") {
		t.Errorf("documento: %+v", l)
	}
	if l := lineFor(t, lines, "US", KeyTransito); !l.Verify || !strings.Contains(l.Text, "tránsito") {
		t.Errorf("escala: %+v", l)
	}
}

func TestLaVistaPreviaDiceCuandoAlcanzaConQueDureLaEstadia(t *testing.T) {
	rules := merge(
		regla("AR", "US", func(f *reglas.Fields) {
			f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"})
		}),
		regla("AR", "MX", func(f *reglas.Fields) {
			f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "llegada"})
		}),
	)
	lines := Preview(Trip{Itinerary: []Stop{{Country: "US", Role: "destino"}, {Country: "MX", Role: "destino"}}}, rules)

	if l := lineFor(t, lines, "US", KeyVigencia); l.Text != "El pasaporte tiene que estar vigente toda la estadía en Estados Unidos" {
		t.Errorf("vigencia: %+v", l)
	}
	if l := lineFor(t, lines, "MX", KeyVigencia); l.Text != "El pasaporte tiene que estar vigente al llegar a México" {
		t.Errorf("vigencia: %+v", l)
	}
}

func TestLaVistaPreviaDiceLasCondiciones(t *testing.T) {
	rules := regla("AR", "CU", func(f *reglas.Fields) {
		f.Condiciones = condiciones(reglas.Condicion{Kind: reglas.CondicionSeguro, Text: "Tener seguro médico válido en Cuba"})
	})
	lines := Preview(Trip{Itinerary: []Stop{{Country: "CU", Role: "destino"}}}, rules)

	if l := lineFor(t, lines, "CU", "condicion:seguro"); l.Text != "Para Cuba hay que tener seguro médico válido en Cuba" || l.Verify {
		t.Errorf("condición: %+v", l)
	}
}

func TestLaVistaPreviaMarcaLoQueNoSeSabeYLoQueSeBusca(t *testing.T) {
	trip := Trip{Itinerary: []Stop{{Country: "JP", Role: "destino"}, {Country: "ES", Role: "destino"}}}
	rules := regla("AR", "ES", nil)
	lines := Preview(trip, rules)

	if l := lineFor(t, lines, "JP", "buscando"); !l.Searching || !strings.Contains(l.Text, "Japón") {
		t.Fatalf("Japón sin regla: %+v", l)
	}
	if l := lineFor(t, lines, "ES", KeyVigencia); !l.Verify {
		t.Errorf("una vigencia en verificar se marca: %+v", l)
	}
	if l := lineFor(t, lines, "ES", KeyVisa); l.Verify {
		t.Errorf("una visa conocida no: %+v", l)
	}
}

func TestLaVistaPreviaEsSoloSinPasajeros(t *testing.T) {
	trip := viaje("ES", ana(pasaporteAR))
	if lines := Preview(trip, regla("AR", "ES", nil)); lines != nil {
		t.Fatalf("con pasajeros no hay vista previa: %+v", lines)
	}
}

func TestLaVistaPreviaSumaElDetalleDeLasPaginasOficiales(t *testing.T) {
	rules := conDetalle(regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
		detalleOficial(reglas.DetallesListo, map[string]reglas.Grupo{
			reglas.GrupoVisa: {
				Pide:    []reglas.Frase{{Text: "Para turismo es la visa B1/B2.", Source: travelState}},
				Tramite: []reglas.Frase{{Text: "Se completa el formulario DS-160 online.", Source: travelState}},
			},
		}))
	lines := Preview(Trip{Itinerary: []Stop{{Country: "US", Role: "destino"}}}, rules)

	visa := lineFor(t, lines, "US", KeyVisa)
	if len(visa.Detail.Pide) != 1 || len(visa.Detail.Tramite) != 1 || visa.Detail.Tramite[0].Source == nil {
		t.Fatalf("detalle de la visa: %+v", visa.Detail)
	}
	if !hasSource(visa.Sources, travelState.URL) {
		t.Errorf("la página oficial va entre las fuentes: %+v", visa.Sources)
	}

	buscando := conDetalle(regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
		detalleOficial(reglas.DetallesBuscando, nil))
	lines = Preview(Trip{Itinerary: []Stop{{Country: "US", Role: "destino"}}}, buscando)
	if l := lineFor(t, lines, "US", KeyVisa); l.Detail.Oficial != reglas.DetallesBuscando {
		t.Errorf("buscando el detalle: %+v", l.Detail)
	}
}

// La línea de vigencia ya dice la regla: lo oficial queda como fuente y no se
// repite en el detalle.
func TestLaVistaPreviaNoRepiteLaVigencia(t *testing.T) {
	rules := conDetalle(regla("AR", "ES", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 3, Unit: "meses", Base: "salida_del_pais"})
	}), detalleOficial(reglas.DetallesListo, map[string]reglas.Grupo{
		reglas.GrupoVigencia: {Pide: []reglas.Frase{
			{Text: "El pasaporte tiene que ser válido hasta 3 meses después de salir del espacio Schengen.", Source: travelState},
		}},
	}))
	vig := lineFor(t, Preview(Trip{Itinerary: []Stop{{Country: "ES", Role: "destino"}}}, rules), "ES", KeyVigencia)
	if len(vig.Detail.Pide) != 0 {
		t.Errorf("la vigencia no repite lo oficial: %+v", vig.Detail.Pide)
	}
	if !hasSource(vig.Sources, travelState.URL) {
		t.Errorf("la página oficial queda citada: %+v", vig.Sources)
	}
}
