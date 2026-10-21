package domain

import (
	"strings"
	"testing"
)

const stateURL = "https://travel.state.gov/content/travel/en/us-visas/tourism-visit/visitor.html"

var statePage = `<html><head><script>var x = "Visa Waiver Program";</script></head><body>
<h1>Visitor Visa</h1>
<p>Visitor visas are nonimmigrant visas for persons who want to enter the United States temporarily
for tourism (visa category B-2).</p>
<p>You must complete the Online Nonimmigrant Visa Application, <b>Form DS-160</b>.</p>
<p>Interviews are generally required for visa applicants from age 14 through 79.</p>
<p>Your passport must be valid for at least six months beyond your period of stay in the United States
unless exempt by country-specific agreements.</p>
</body></html>`

func frase(text, quote string) FraseBuscada {
	return FraseBuscada{Text: text, URL: stateURL, Quote: quote}
}

func TestSoloSobrevivenLasFrasesCuyaCitaEstaEnLaPagina(t *testing.T) {
	out := ResearchOutput{Grupos: []GrupoBuscado{{
		Grupo: GrupoVisa,
		Tramite: []FraseBuscada{
			frase("Se completa el formulario DS-160 online.", "complete the Online Nonimmigrant Visa Application, Form DS-160"),
			frase("Hay entrevista para quienes tienen entre 14 y 79 años.", "Interviews are generally required for visa applicants from age 14 through 79"),
			// Inventada: la página no lo dice.
			frase("El turno sale en dos semanas.", "appointments are available within two weeks"),
			// La cifra no está en la cita.
			frase("Hay entrevista desde los 12 años.", "Interviews are generally required"),
			// Una URL que no se leyó.
			{Text: "Se paga online.", URL: "https://blog.example.com", Quote: "pay online"},
			// Un enlace en la frase.
			frase("Ver https://ceac.state.gov.", "Form DS-160"),
		},
	}, {
		Grupo: "inventado",
		Pide:  []FraseBuscada{frase("Algo.", "Visitor Visa")},
	}}}

	r := CheckResearch(out, map[string]string{stateURL: PageText(statePage)}, "2026-10-21")
	g, ok := r.Grupos[GrupoVisa]
	if !ok || len(g.Tramite) != 2 {
		t.Fatalf("trámite = %+v", r.Grupos)
	}
	if g.Tramite[0].Source.Source != SourceOficial || g.Tramite[0].Source.URL != stateURL || g.Tramite[0].Source.ReadAt != "2026-10-21" {
		t.Errorf("fuente = %+v", g.Tramite[0].Source)
	}
	if _, ok := r.Grupos["inventado"]; ok {
		t.Error("un grupo que no existe no se guarda")
	}
}

// El texto de la página es el que se ve, no el código: lo que está en un
// script no sirve de cita.
func TestLaCitaNoPuedeSalirDeUnScript(t *testing.T) {
	out := ResearchOutput{Grupos: []GrupoBuscado{{Grupo: GrupoVisa, Pide: []FraseBuscada{
		frase("Entra con el programa de exención de visa.", "Visa Waiver Program"),
	}}}}
	if r := CheckResearch(out, map[string]string{stateURL: PageText(statePage)}, "2026-10-21"); len(r.Grupos) != 0 {
		t.Fatalf("grupos = %+v", r.Grupos)
	}
}

func TestElControlSeComprueba(t *testing.T) {
	vigencia := Vigencia{Amount: 6, Unit: "meses", Base: "salida_del_pais"}
	consular := VisaConsular
	treinta := 30
	out := ResearchOutput{Control: ControlBuscado{
		Visa:     &Buscado[Visa]{Value: &consular, URL: stateURL, Quote: "visa category B-2"},
		Vigencia: &Buscado[Vigencia]{Value: &vigencia, URL: stateURL, Quote: "valid for at least six months beyond your period of stay"},
		// La página no dice 30 días.
		EstadiaMaxDias: &Buscado[int]{Value: &treinta, URL: stateURL, Quote: "temporarily"},
	}}
	r := CheckResearch(out, map[string]string{stateURL: PageText(statePage)}, "2026-10-21")
	if r.Control.Visa == nil || r.Control.Visa.Value != VisaConsular {
		t.Errorf("visa = %+v", r.Control.Visa)
	}
	// «six» no es «6»: sin la cifra en la cita, no vale.
	if r.Control.Vigencia != nil {
		t.Errorf("vigencia = %+v", r.Control.Vigencia)
	}
	if r.Control.EstadiaMaxDias != nil {
		t.Errorf("estadía = %+v", r.Control.EstadiaMaxDias)
	}
}

func oficial(quote string) Citation {
	return Citation{Source: SourceOficial, URL: stateURL, Quote: quote, ReadAt: "2026-10-21"}
}

func deWiki[T any](v T) Field[T] {
	return known(v, Citation{Source: SourceWikipedia, Quote: "cita", URL: "https://wiki"})
}

func investigada(base Fields, mod func(*Research)) *Research {
	r := &Research{ReadAt: "2026-10-21", Seen: FieldPrints(base), Grupos: map[string]Grupo{
		GrupoVisa: {Tramite: []Frase{{Text: "Se completa el DS-160.", Source: oficial("Form DS-160")}}},
	}}
	if mod != nil {
		mod(r)
	}
	return r
}

var argentinaUS = Pair{Nationality: "AR", Country: "US"}

func TestLaBusquedaQueCoincideSumaSusCitas(t *testing.T) {
	base := Fields{Visa: deWiki(VisaConsular), Documento: known(DocumentoPasaporte)}
	r := investigada(base, func(r *Research) {
		r.Control.Visa = &Controlled[Visa]{Value: VisaConsular, Source: oficial("visa category B-2")}
	})
	f, grupos, conflicts, stale := ApplyResearch(argentinaUS, base, r)
	if f.Visa.Verify || *f.Visa.Value != VisaConsular || len(f.Visa.Sources) != 2 || len(conflicts) != 0 || stale {
		t.Fatalf("visa = %+v, conflictos %v, vieja %v", f.Visa, conflicts, stale)
	}
	if len(grupos[GrupoVisa].Tramite) != 1 {
		t.Errorf("grupos = %+v", grupos)
	}
}

func TestLaBusquedaQueContradiceDejaElCampoEnVerificar(t *testing.T) {
	base := Fields{Visa: deWiki(VisaConsular), Documento: known(DocumentoPasaporte)}
	r := investigada(base, func(r *Research) {
		r.Control.Visa = &Controlled[Visa]{Value: VisaNoRequiere, Source: oficial("visa not required")}
	})
	f, _, conflicts, _ := ApplyResearch(argentinaUS, base, r)
	if !f.Visa.Verify || !strings.Contains(f.Visa.Reason, "no coinciden") || !strings.Contains(f.Visa.Reason, "travel.state.gov") {
		t.Fatalf("visa = %+v", f.Visa)
	}
	if len(conflicts) != 1 || conflicts[0].Source != SourceOficial || conflicts[0].Other != string(VisaNoRequiere) {
		t.Fatalf("conflictos = %+v", conflicts)
	}
}

// La búsqueda no completa campos: lo que está en verificar sigue así.
func TestLaBusquedaNoCompletaUnCampoEnVerificar(t *testing.T) {
	base := Fields{Visa: verify[Visa]("las fuentes no coinciden"), Documento: known(DocumentoPasaporte)}
	r := investigada(base, func(r *Research) {
		r.Control.Visa = &Controlled[Visa]{Value: VisaConsular, Source: oficial("visa category B-2")}
	})
	if f, _, _, _ := ApplyResearch(argentinaUS, base, r); !f.Visa.Verify || f.Visa.Reason != "las fuentes no coinciden" {
		t.Fatalf("visa = %+v", f.Visa)
	}
}

// Contra un valor por defecto, sin fuente, la búsqueda solo puede endurecer.
func TestContraUnValorPorDefectoLaBusquedaSoloEndurece(t *testing.T) {
	base := Fields{Visa: deWiki(VisaNoRequiere), Documento: known(DocumentoPasaporte)}

	dni := investigada(base, func(r *Research) {
		r.Control.Documento = &Controlled[Documento]{Value: DocumentoDNIOPasaporte, Source: oficial("national ID card")}
	})
	if f, _, conflicts, _ := ApplyResearch(argentinaUS, base, dni); f.Documento.Verify || *f.Documento.Value != DocumentoPasaporte ||
		len(f.Documento.Sources) != 0 || len(conflicts) != 0 {
		t.Fatalf("más permisiva: %+v", f.Documento)
	}

	corta := investigada(base, func(r *Research) {
		r.Control.EstadiaMaxDias = &Controlled[int]{Value: 30, Source: oficial("up to 30 days")}
	})
	if f, _, _, _ := ApplyResearch(argentinaUS, base, corta); !f.EstadiaMaxDias.Verify {
		t.Fatalf("una estadía máxima donde no había ninguna: %+v", f.EstadiaMaxDias)
	}
}

// Las constantes ganan: la búsqueda no las pone en verificar.
func TestLaBusquedaNoTocaLasConstantes(t *testing.T) {
	es := Pair{Nationality: "AR", Country: "ES"}
	base, _ := Combine(es, Fields{Visa: deWiki(VisaNoRequiere), Vigencia: verify[Vigencia]("x")}, nil)
	r := investigada(base, func(r *Research) {
		r.Control.Vigencia = &Controlled[Vigencia]{Value: Vigencia{Amount: 6, Unit: "meses", Base: "llegada"}, Source: oficial("6 months")}
	})
	if f, _, _, _ := ApplyResearch(es, base, r); f.Vigencia.Verify || f.Vigencia.Value.Amount != 3 {
		t.Fatalf("vigencia = %+v", f.Vigencia)
	}
}

// Si la regla cambió después de la búsqueda, lo que la búsqueda dijo de ese
// campo ya no vale: ni el detalle ni el control.
func TestUnCampoQueCambioDespuesDeLaBusquedaLaDejaVieja(t *testing.T) {
	antes := Fields{Visa: deWiki(VisaConsular), Documento: known(DocumentoPasaporte)}
	r := investigada(antes, func(r *Research) {
		r.Control.Visa = &Controlled[Visa]{Value: VisaConsular, Source: oficial("visa category B-2")}
		r.Grupos[GrupoDocumento] = Grupo{Pide: []Frase{{Text: "Pide pasaporte.", Source: oficial("passport")}}}
	})
	ahora := antes
	ahora.Visa = deWiki(VisaElectronica)

	f, grupos, conflicts, stale := ApplyResearch(argentinaUS, ahora, r)
	if !stale {
		t.Error("la búsqueda quedó vieja")
	}
	if _, ok := grupos[GrupoVisa]; ok {
		t.Errorf("el detalle de la visa vieja no se muestra: %+v", grupos)
	}
	if _, ok := grupos[GrupoDocumento]; !ok {
		t.Errorf("el de un campo que no cambió, sí: %+v", grupos)
	}
	if f.Visa.Verify || len(conflicts) != 0 {
		t.Errorf("el control viejo no se usa: %+v %v", f.Visa, conflicts)
	}
}

// El texto de la herramienta de lectura viene en markdown: una cita sin las
// marcas, o con ellas, vale igual.
func TestLasCitasIgnoranLasMarcasDeMarkdown(t *testing.T) {
	page := "Bring the **Nonimmigrant Visa Application,** **[Form DS-160](#ds160 \"Form DS-160\") confirmation page.**"
	out := ResearchOutput{Grupos: []GrupoBuscado{{Grupo: GrupoVisa, Tramite: []FraseBuscada{
		frase("Hay que llevar la confirmación del DS-160.", "Nonimmigrant Visa Application, Form DS-160 confirmation page."),
		frase("Hay que llevar la confirmación del DS-160.", "Nonimmigrant Visa Application,** **[Form DS-160](#ds160 \"Form DS-160\") confirmation page."),
	}}}}
	r := CheckResearch(out, map[string]string{stateURL: page}, "2026-10-21")
	if len(r.Grupos[GrupoVisa].Tramite) != 2 {
		t.Fatalf("trámite = %+v", r.Grupos)
	}
}

func TestSoloLosDestinosConTramiteSeBuscan(t *testing.T) {
	for visa, want := range map[Visa]bool{VisaNoRequiere: false, VisaNoAdmitido: false, VisaALaLlegada: true,
		VisaAutorizacionElectronica: true, VisaElectronica: true, VisaConsular: true} {
		if got := NeedsResearch(Fields{Visa: deWiki(visa)}); got != want {
			t.Errorf("%s: %v", visa, got)
		}
	}
	if NeedsResearch(Fields{Visa: verify[Visa]("no se sabe")}) {
		t.Error("con la visa en verificar no se busca")
	}
}
