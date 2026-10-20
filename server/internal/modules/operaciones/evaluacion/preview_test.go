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
