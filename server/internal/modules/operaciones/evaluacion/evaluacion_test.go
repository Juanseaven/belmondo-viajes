package evaluacion

import (
	"strings"
	"testing"
	"time"

	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

func d(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}

func p[T any](v T) *T { return &v }

func field[T any](v T) reglas.Field[T] {
	return reglas.Field[T]{Value: &v, Sources: []reglas.Citation{{Source: reglas.SourceWikipedia, Quote: "cita", URL: "https://wiki"}}}
}

func verificar[T any](reason string) reglas.Field[T] {
	return reglas.Field[T]{Verify: true, Reason: reason}
}

// regla arma una regla vigente con lo que se le pase y el resto «no exigido».
func regla(nat, country string, mod func(*reglas.Fields)) map[reglas.Pair]reglas.Rule {
	f := reglas.Fields{
		Documento:  field(reglas.DocumentoPasaporte),
		Visa:       field(reglas.VisaNoRequiere),
		Vigencia:   verificar[reglas.Vigencia]("Wikipedia no dice la vigencia del pasaporte"),
		Exenciones: field([]reglas.Exencion{}),
		Vacunas:    field([]reglas.Vacuna{}),
	}
	if mod != nil {
		mod(&f)
	}
	pair := reglas.Pair{Nationality: nat, Country: country}
	return map[reglas.Pair]reglas.Rule{pair: {Pair: pair, Status: reglas.RuleVigente, Fields: f, Version: 1}}
}

func merge(ms ...map[reglas.Pair]reglas.Rule) map[reglas.Pair]reglas.Rule {
	out := map[reglas.Pair]reglas.Rule{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func viaje(country string, pax ...Passenger) Trip {
	return Trip{DepartAt: d("2026-12-10"), ReturnAt: d("2026-12-24"),
		Itinerary: []Stop{{Country: country, Role: "destino"}}, Passengers: pax}
}

func ana(doc *Document, permits ...Permit) Passenger {
	return Passenger{ID: "ana", Name: "Ana", ResidenceCountry: "AR", Doc: doc, Permits: permits}
}

var pasaporteAR = &Document{Type: "Pasaporte", Country: "AR", ExpiresAt: d("2031-01-01")}

func find(t *testing.T, reqs []Requirement, pax, country, key string) Requirement {
	t.Helper()
	for _, r := range reqs {
		if r.PassengerID == pax && r.Country == country && r.Key == key {
			return r
		}
	}
	t.Fatalf("no hay requisito %s/%s/%s en %+v", pax, country, key, reqs)
	return Requirement{}
}

func has(reqs []Requirement, pax, country, key string) bool {
	for _, r := range reqs {
		if r.PassengerID == pax && r.Country == country && r.Key == key {
			return true
		}
	}
	return false
}

func TestElDNIArgentinoCumpleEnBrasilYFaltaEnEstadosUnidos(t *testing.T) {
	dni := &Document{Type: "DNI", Country: "AR", ExpiresAt: d("2030-01-01")}
	rules := merge(
		regla("AR", "BR", func(f *reglas.Fields) { f.Documento = field(reglas.DocumentoDNIOPasaporte) }),
		regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
	)

	br := Evaluate(viaje("BR", ana(dni)), rules, nil)
	if r := find(t, br, "ana", "BR", KeyDocumento); r.Status != Cumple {
		t.Errorf("Brasil con DNI: %+v", r)
	}

	us := Evaluate(viaje("US", ana(dni)), rules, nil)
	r := find(t, us, "ana", "US", KeyDocumento)
	if r.Status != Falta || !strings.Contains(said(r), "pasaporte") || r.Confirmable {
		t.Errorf("EE.UU. con DNI: %+v", r)
	}
}

func TestCanadaPideVisaSalvoExencionPorVisaDeEstadosUnidos(t *testing.T) {
	rules := regla("AR", "CA", func(f *reglas.Fields) {
		f.Visa = field(reglas.VisaConsular)
		f.Exenciones = field([]reglas.Exencion{{PermitKinds: []string{"visa"}, PermitCountry: "US",
			Visa: reglas.VisaAutorizacionElectronica}})
	})

	sinNada := Evaluate(viaje("CA", ana(pasaporteAR)), rules, nil)
	if r := find(t, sinNada, "ana", "CA", KeyVisa); r.Status != Falta || r.Confirmable {
		t.Fatalf("sin permisos: %+v", r)
	}

	visaUS := Permit{Kind: "visa", Country: "US", ExpiresAt: d("2032-01-01")}
	conVisaUS := Evaluate(viaje("CA", ana(pasaporteAR, visaUS)), rules, nil)
	r := find(t, conVisaUS, "ana", "CA", KeyVisa)
	if r.Status != Falta || !strings.Contains(said(r), "autorización electrónica") {
		t.Fatalf("con visa de EE.UU. y sin eTA: %+v, debería pedir la autorización electrónica", r)
	}

	eta := Permit{Kind: "autorizacion_electronica", Country: "CA", ExpiresAt: d("2030-01-01")}
	conTodo := Evaluate(viaje("CA", ana(pasaporteAR, visaUS, eta)), rules, nil)
	if r := find(t, conTodo, "ana", "CA", KeyVisa); r.Status != Cumple {
		t.Fatalf("con visa de EE.UU. y eTA: %+v", r)
	}

	vencida := Permit{Kind: "visa", Country: "US", ExpiresAt: d("2025-01-01")}
	conVencida := Evaluate(viaje("CA", ana(pasaporteAR, vencida, eta)), rules, nil)
	if r := find(t, conVencida, "ana", "CA", KeyVisa); r.Status != Falta {
		t.Fatalf("con visa de EE.UU. vencida: %+v, la exención no aplica", r)
	}
}

func TestUnaVisaQueVenceDuranteElViajeNoAlcanza(t *testing.T) {
	rules := regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) })
	visa := Permit{Kind: "visa", Country: "US", ExpiresAt: d("2026-12-15")}

	r := find(t, Evaluate(viaje("US", ana(pasaporteAR, visa)), rules, nil), "ana", "US", KeyVisa)
	if r.Status != Falta || !strings.Contains(said(r), "15/12/2026") {
		t.Fatalf("visa que vence en el viaje: %+v", r)
	}
}

// Quien tiene doble nacionalidad se evalúa con el documento con el que viaja.
func TestLaDobleNacionalidadSeEvaluaConElDocumentoDeViaje(t *testing.T) {
	rules := merge(
		regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
		regla("IT", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaAutorizacionElectronica) }),
	)
	italiano := &Document{Type: "Pasaporte", Country: "IT", ExpiresAt: d("2031-01-01")}

	trip := viaje("US", ana(italiano))
	if pairs := Pairs(trip); len(pairs) != 1 || pairs[0] != (reglas.Pair{Nationality: "IT", Country: "US"}) {
		t.Fatalf("combinaciones = %v", pairs)
	}
	r := find(t, Evaluate(trip, rules, nil), "ana", "US", KeyVisa)
	if r.Status != Falta || !strings.Contains(said(r), "autorización electrónica") {
		t.Fatalf("italiano a EE.UU.: %+v", r)
	}
}

func TestSchengenMideVigenciaYAntiguedadDelPasaporte(t *testing.T) {
	rules := regla("AR", "ES", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 3, Unit: "meses", Base: "salida_del_pais"})
		f.EmisionMaxAnios = field(10)
	})

	corto := &Document{Type: "Pasaporte", Country: "AR", IssuedAt: d("2020-01-01"), ExpiresAt: d("2027-02-20")}
	r := find(t, Evaluate(viaje("ES", ana(corto)), rules, nil), "ana", "ES", KeyVigencia)
	if r.Status != Falta || !strings.Contains(said(r), "24/03/2027") {
		t.Fatalf("vence 2 meses después de salir: %+v", r)
	}

	viejo := &Document{Type: "Pasaporte", Country: "AR", IssuedAt: d("2015-11-01"), ExpiresAt: d("2031-01-01")}
	if r := find(t, Evaluate(viaje("ES", ana(viejo)), rules, nil), "ana", "ES", KeyAntiguedad); r.Status != Falta {
		t.Fatalf("emitido hace 11 años: %+v", r)
	}

	sinEmision := &Document{Type: "Pasaporte", Country: "AR", ExpiresAt: d("2031-01-01")}
	r = find(t, Evaluate(viaje("ES", ana(sinEmision)), rules, nil), "ana", "ES", KeyAntiguedad)
	if r.Status != Falta || r.Confirmable || !strings.Contains(said(r), "emisión") {
		t.Fatalf("sin fecha de emisión: %+v", r)
	}

	bien := &Document{Type: "Pasaporte", Country: "AR", IssuedAt: d("2020-01-01"), ExpiresAt: d("2031-01-01")}
	reqs := Evaluate(viaje("ES", ana(bien)), rules, nil)
	if find(t, reqs, "ana", "ES", KeyVigencia).Status != Cumple || find(t, reqs, "ana", "ES", KeyAntiguedad).Status != Cumple {
		t.Fatalf("pasaporte en regla: %+v", reqs)
	}
}

func TestUnCampoEnVerificarNuncaCumple(t *testing.T) {
	rules := regla("AR", "JP", func(f *reglas.Fields) { f.Visa = verificar[reglas.Visa]("las fuentes no coinciden") })

	r := find(t, Evaluate(viaje("JP", ana(pasaporteAR)), rules, nil), "ana", "JP", KeyVisa)
	if r.Status != Verificar || !r.Confirmable || !strings.Contains(said(r), "no coinciden") {
		t.Fatalf("visa en verificar: %+v", r)
	}
}

// Una regla que todavía se busca no sabe nada: todo queda en verificar.
func TestSinReglaTodoQuedaEnVerificar(t *testing.T) {
	reqs := Evaluate(viaje("JP", ana(pasaporteAR)), map[reglas.Pair]reglas.Rule{}, nil)
	if r := find(t, reqs, "ana", "JP", KeyVisa); r.Status != Verificar || !strings.Contains(said(r), "buscando") {
		t.Fatalf("sin regla: %+v", r)
	}
}

func TestQuienCumple18DuranteElViajeSaleSiendoMenor(t *testing.T) {
	menor := ana(pasaporteAR)
	menor.Minor = true
	r := find(t, Evaluate(viaje("BR", menor), regla("AR", "BR", nil), nil), "ana", "", KeyMenor)
	if r.Status != Falta || !r.Confirmable || !strings.Contains(said(r), "autorización") {
		t.Fatalf("menor: %+v", r)
	}
	if has(Evaluate(viaje("BR", ana(pasaporteAR)), regla("AR", "BR", nil), nil), "ana", "", KeyMenor) {
		t.Error("un adulto no necesita autorización de menor")
	}
}

// La autorización de salida de menores es una regla argentina: un viaje que
// sale de otro país no la pide.
func TestUnMenorQueSaleDeChileNoNecesitaLaAutorizacionArgentina(t *testing.T) {
	menor := ana(pasaporteAR)
	menor.Minor = true
	trip := viaje("BR", menor)
	trip.Origin = "CL"
	if reqs := Evaluate(trip, regla("AR", "BR", nil), nil); has(reqs, "ana", "", KeyMenor) {
		t.Fatalf("saliendo de Chile no se pide la autorización argentina: %+v", reqs)
	}
}

func TestUnaEscalaEnUnPaisQuePideVisaQuedaEnVerificar(t *testing.T) {
	rules := merge(regla("AR", "ES", nil), regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }))
	trip := viaje("ES", ana(pasaporteAR))
	trip.Itinerary = append(trip.Itinerary, Stop{Country: "US", Role: "escala"})

	reqs := Evaluate(trip, rules, nil)
	r := find(t, reqs, "ana", "US", KeyTransito)
	if r.Status != Verificar || !strings.Contains(said(r), "tránsito") {
		t.Fatalf("escala en EE.UU.: %+v", r)
	}
	// En una escala solo se mira el tránsito.
	if has(reqs, "ana", "US", KeyVisa) || has(reqs, "ana", "US", KeyVigencia) {
		t.Error("una escala no evalúa visa ni vigencia de estadía")
	}

	sinVisa := Evaluate(Trip{DepartAt: trip.DepartAt, ReturnAt: trip.ReturnAt, Passengers: trip.Passengers,
		Itinerary: []Stop{{Country: "ES", Role: "escala"}}}, rules, nil)
	if r := find(t, sinVisa, "ana", "ES", KeyTransito); r.Status != Cumple {
		t.Fatalf("escala sin visa: %+v", r)
	}
}

func TestConfirmarVaHastaQueLaReglaCambieEseCampo(t *testing.T) {
	rules := regla("AR", "JP", func(f *reglas.Fields) { f.Visa = verificar[reglas.Visa]("las fuentes no coinciden") })
	trip := viaje("JP", ana(pasaporteAR))
	antes := find(t, Evaluate(trip, rules, nil), "ana", "JP", KeyVisa)

	confirmada := []Confirmation{{PassengerID: "ana", Country: "JP", Key: KeyVisa, FieldHash: antes.FieldHash}}
	if r := find(t, Evaluate(trip, rules, confirmada), "ana", "JP", KeyVisa); r.Status != Cumple || !r.Confirmed {
		t.Fatalf("confirmada: %+v", r)
	}

	cambiada := regla("AR", "JP", func(f *reglas.Fields) {
		f.Visa = verificar[reglas.Visa]("las fuentes no coinciden: ahora por otra cosa")
	})
	if r := find(t, Evaluate(trip, cambiada, confirmada), "ana", "JP", KeyVisa); r.Status != Verificar || r.Confirmed {
		t.Fatalf("la regla cambió el campo: %+v, la confirmación no debería valer", r)
	}
}

func TestUnFaltaPorDatoAusenteNoSeConfirma(t *testing.T) {
	sinVencimiento := &Document{Type: "Pasaporte", Country: "AR"}
	rules := regla("AR", "ES", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 3, Unit: "meses", Base: "salida_del_pais"})
	})
	r := find(t, Evaluate(viaje("ES", ana(sinVencimiento)), rules, nil), "ana", "ES", KeyVigencia)
	if r.Status != Falta || r.Confirmable {
		t.Fatalf("sin vencimiento: %+v", r)
	}
	// Ni aunque alguien lo haya «confirmado».
	conf := []Confirmation{{PassengerID: "ana", Country: "ES", Key: KeyVigencia, FieldHash: r.FieldHash}}
	if r := find(t, Evaluate(viaje("ES", ana(sinVencimiento)), rules, conf), "ana", "ES", KeyVigencia); r.Status != Falta {
		t.Fatalf("un dato ausente no se confirma: %+v", r)
	}
}

func TestSinItinerarioNiDocumentoFaltaCargarlos(t *testing.T) {
	trip := Trip{DepartAt: d("2026-12-10"), ReturnAt: d("2026-12-24"), Passengers: []Passenger{ana(nil)}}
	reqs := Evaluate(trip, nil, nil)
	if r := find(t, reqs, "", "", KeyItinerario); r.Status != Falta {
		t.Fatalf("sin itinerario: %+v", r)
	}

	trip.Itinerary = []Stop{{Country: "BR", Role: "destino"}}
	reqs = Evaluate(trip, regla("AR", "BR", nil), nil)
	if r := find(t, reqs, "ana", "", KeyDocumentoDeViaje); r.Status != Falta {
		t.Fatalf("sin documento: %+v", r)
	}
}

func TestVisaALaLlegadaCumpleYNoAdmitidoFalta(t *testing.T) {
	llegada := regla("AR", "EG", func(f *reglas.Fields) { f.Visa = field(reglas.VisaALaLlegada) })
	if r := find(t, Evaluate(viaje("EG", ana(pasaporteAR)), llegada, nil), "ana", "EG", KeyVisa); r.Status != Cumple ||
		!strings.Contains(said(r), "al llegar") {
		t.Fatalf("visa a la llegada: %+v", r)
	}

	prohibido := regla("AR", "KP", func(f *reglas.Fields) { f.Visa = field(reglas.VisaNoAdmitido) })
	r := find(t, Evaluate(viaje("KP", ana(pasaporteAR)), prohibido, nil), "ana", "KP", KeyVisa)
	if r.Status != Falta || r.Confirmable || !strings.Contains(said(r), "no admite") {
		t.Fatalf("no admitido: %+v", r)
	}
}

// Un argentino no necesita nada para entrar a la Argentina. Queda una línea que
// lo dice, para que un viaje dentro del país también pueda estar al día.
func TestElPaisDelDocumentoNoPideNada(t *testing.T) {
	reqs := Evaluate(viaje("AR", ana(pasaporteAR)), nil, nil)
	if len(reqs) != 1 || reqs[0].Key != KeyPaisPropio || reqs[0].Status != Cumple {
		t.Fatalf("requisitos = %+v", reqs)
	}
}

func TestLaEstadiaQueSePasaDelMaximoFalta(t *testing.T) {
	rules := regla("AR", "TH", func(f *reglas.Fields) { f.EstadiaMaxDias = field(30) })
	trip := viaje("TH", ana(pasaporteAR))
	trip.ReturnAt = d("2027-02-10")

	r := find(t, Evaluate(trip, rules, nil), "ana", "TH", KeyEstadia)
	if r.Status != Falta || !strings.Contains(said(r), "30") {
		t.Fatalf("estadía larga: %+v", r)
	}
}

func condiciones(cs ...reglas.Condicion) reglas.Field[[]reglas.Condicion] {
	f := field(cs)
	f.Sources = nil
	for i := range cs {
		f.Sources = append(f.Sources, reglas.Citation{Source: reglas.SourceWikipedia, Quote: "cita " + cs[i].Kind})
	}
	return f
}

// Las condiciones de las notas: lo que el pasajero tiene que tener o hacer
// falta hasta que alguien lo confirme y va al aviso del cliente; el pasaje de
// regreso cumple si el viaje tiene regreso; el puerto de entrada lo revisa la
// agencia contra los vuelos.
func TestLasCondicionesDelPaisSeEvaluanSegunQueSon(t *testing.T) {
	rules := regla("AR", "CU", func(f *reglas.Fields) {
		f.Condiciones = condiciones(
			reglas.Condicion{Kind: reglas.CondicionSeguro, Text: "Tener seguro médico válido en Cuba"},
			reglas.Condicion{Kind: reglas.CondicionPasajeRegreso, Text: "Mostrar el pasaje de regreso"},
			reglas.Condicion{Kind: reglas.CondicionPuertoDeEntrada, Text: "Entrar por el aeropuerto de La Habana"},
			reglas.Condicion{Kind: reglas.CondicionOtra, Text: "Declarar la moneda extranjera"},
			reglas.Condicion{Kind: reglas.CondicionOtra, Text: "Llevar el formulario D'Viajeros"},
		)
	})
	reqs := Evaluate(viaje("CU", ana(pasaporteAR)), rules, nil)

	otra := find(t, reqs, "ana", "CU", "condicion:otra")
	if otra.Status != Falta || !otra.Confirmable || !strings.Contains(said(otra), "Para Cuba hay que declarar la moneda extranjera") ||
		!strings.Contains(otra.ClientText, "declarar la moneda") {
		t.Errorf("otra: %+v", otra)
	}
	if len(otra.Sources) != 1 || otra.Sources[0].Quote != "cita otra" {
		t.Errorf("cada condición lleva su cita: %+v", otra.Sources)
	}
	// La agencia emite asistencia médica en todos los viajes: el seguro es
	// tarea suya, no del cliente.
	if r := find(t, reqs, "ana", "CU", "condicion:seguro"); r.Status != Verificar || !r.Confirmable || r.ClientText != "" ||
		!strings.Contains(said(r), "asistencia") {
		t.Errorf("seguro: %+v", r)
	}
	if r := find(t, reqs, "ana", "CU", "condicion:pasaje_regreso"); r.Status != Cumple || !strings.Contains(said(r), "24/12/2026") {
		t.Errorf("pasaje de regreso: %+v", r)
	}
	if r := find(t, reqs, "ana", "CU", "condicion:puerto_de_entrada"); r.Status != Verificar || !r.Confirmable || r.ClientText != "" {
		t.Errorf("puerto de entrada: %+v", r)
	}
	// Dos condiciones del mismo tipo no comparten clave.
	if !has(reqs, "ana", "CU", "condicion:otra") || !has(reqs, "ana", "CU", "condicion:otra:2") {
		t.Errorf("las dos «otra»: %+v", reqs)
	}

	sinRegreso := viaje("CU", ana(pasaporteAR))
	sinRegreso.ReturnAt = nil
	if r := find(t, Evaluate(sinRegreso, rules, nil), "ana", "CU", "condicion:pasaje_regreso"); r.Status != Falta {
		t.Errorf("sin regreso cargado: %+v", r)
	}
}

func TestCondicionesQueNoSeSabenQuedanEnVerificar(t *testing.T) {
	rules := regla("AR", "CU", func(f *reglas.Fields) {
		f.Condiciones = verificar[[]reglas.Condicion]("la cita no aparece en Wikipedia")
	})
	r := find(t, Evaluate(viaje("CU", ana(pasaporteAR)), rules, nil), "ana", "CU", KeyCondiciones)
	if r.Status != Verificar || !strings.Contains(said(r), "la cita no aparece") {
		t.Fatalf("condiciones: %+v", r)
	}
}

func TestLasVacunasExigidasSeConfirman(t *testing.T) {
	rules := regla("AR", "AO", func(f *reglas.Fields) {
		f.Vacunas = field([]reglas.Vacuna{{Name: "fiebre amarilla", Scope: reglas.VacunaParaTodos},
			{Name: "poliomielitis", Scope: reglas.VacunaDesdePaisRiesgo}})
	})
	reqs := Evaluate(viaje("AO", ana(pasaporteAR)), rules, nil)
	fa := find(t, reqs, "ana", "AO", "vacuna:fiebre amarilla")
	polio := find(t, reqs, "ana", "AO", "vacuna:poliomielitis")
	if fa.Status != Falta || !fa.Confirmable || polio.Status != Verificar || !polio.Confirmable {
		t.Fatalf("vacunas: %+v / %+v", fa, polio)
	}
	if !strings.Contains(said(polio), "sale de Argentina") {
		t.Errorf("la vacuna por país de riesgo nombra el origen: %q", said(polio))
	}

	trip := viaje("AO", ana(pasaporteAR))
	trip.Origin = "PE"
	polio = find(t, Evaluate(trip, rules, nil), "ana", "AO", "vacuna:poliomielitis")
	if polio.Status != Verificar || !strings.Contains(said(polio), "sale de Perú") {
		t.Fatalf("saliendo de Perú: %+v", polio)
	}
}

func TestElAvisoDiceLoQueFaltaPorPasajeroYNuncaTodoEnOrdenConDudas(t *testing.T) {
	rules := regla("AR", "CA", func(f *reglas.Fields) {
		f.Visa = field(reglas.VisaConsular)
		f.Exenciones = field([]reglas.Exencion{{PermitCountry: "US", Visa: reglas.VisaAutorizacionElectronica}})
	})
	maria := Passenger{ID: "maria", Name: "María", ResidenceCountry: "AR", Doc: pasaporteAR,
		Permits: []Permit{{Kind: "visa", Country: "US", ExpiresAt: d("2032-01-01")}}}
	tomas := Passenger{ID: "tomas", Name: "Tomás", Minor: true, ResidenceCountry: "AR", Doc: pasaporteAR,
		Permits: []Permit{{Kind: "visa", Country: "CA", ExpiresAt: d("2032-01-01")}}}
	trip := viaje("CA", maria, tomas)

	msg := Notice("Juan", trip, Evaluate(trip, rules, nil))
	for _, want := range []string{"Juan", "Canadá", "María", "autorización electrónica", "Tomás", "autorización de viaje"} {
		if !strings.Contains(msg, want) {
			t.Errorf("el aviso no dice %q:\n%s", want, msg)
		}
	}

	conVigencia := regla("AR", "BR", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"})
	})
	bien := Evaluate(viaje("BR", ana(pasaporteAR)), conVigencia, nil)
	if msg := Notice("Ana", viaje("BR", ana(pasaporteAR)), bien); !strings.Contains(msg, "en orden") {
		t.Errorf("sin faltantes ni dudas:\n%s", msg)
	}
	// Una vigencia que la regla no sabe la gobierna el control del documento:
	// no impide decir que está todo en orden.
	sinVigencia := Evaluate(viaje("BR", ana(pasaporteAR)), regla("AR", "BR", nil), nil)
	if msg := Notice("Ana", viaje("BR", ana(pasaporteAR)), sinVigencia); !strings.Contains(msg, "en orden") {
		t.Errorf("vigencia en verificar:\n%s", msg)
	}

	dudas := regla("AR", "JP", func(f *reglas.Fields) { f.Visa = verificar[reglas.Visa]("x") })
	trip = viaje("JP", ana(pasaporteAR))
	msg = Notice("Ana", trip, Evaluate(trip, dudas, nil))
	if strings.Contains(msg, "en orden") || !strings.Contains(msg, "en breve") {
		t.Errorf("con dudas no hay «todo en orden»:\n%s", msg)
	}
}

// Si el país pide pasaporte y el pasajero viaja con DNI, el documento falta y
// la vigencia del DNI no se mide: no dice nada y confundía con un «Cumple».
func TestConUnDocumentoQueElPaisNoAceptaNoSeMideSuVigencia(t *testing.T) {
	rules := regla("AR", "US", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"})
		f.EmisionMaxAnios = field(10)
	})
	dni := &Document{Type: "DNI", Country: "AR", ExpiresAt: d("2029-10-27")}
	reqs := Evaluate(viaje("US", ana(dni)), rules, nil)

	if doc := find(t, reqs, "ana", "US", KeyDocumento); doc.Status != Falta {
		t.Errorf("el documento falta: %+v", doc)
	}
	if has(reqs, "ana", "US", KeyVigencia) || has(reqs, "ana", "US", KeyAntiguedad) {
		t.Error("con DNI no se mide la vigencia ni la antigüedad para un país que pide pasaporte")
	}
}
