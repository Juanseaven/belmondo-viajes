package domain

import (
	"strings"
	"testing"
)

// Texto tomado de la página de Wikipedia para argentinos (CC BY-SA), recortado.
const filaCanada = `| {{flag|Canada}}
| data-sort-value="4" {{no|Visa required}}<ref>{{Timatic|nationality=AR|destination=CA}}</ref>
|
|
* Argentine citizens who have held a Canadian visa in the last 10 years or who hold a valid United States non-immigrant visa can enter Canada solely with an [[Visa policy of Canada#Electronic travel authorization|eTA]] when arriving by air.`

var wiki = Citation{Source: SourceWikipedia, URL: "https://en.wikipedia.org/wiki/Visa_requirements_for_Argentine_citizens", Revision: "123"}

func ptr[T any](v T) *T { return &v }

func TestUnaCitaQueNoApareceEnLaFuenteNoSostieneElValor(t *testing.T) {
	ex := Extraction{Visa: Extracted[Visa]{Value: ptr(VisaNoRequiere), Quote: "Visa not required"}}

	got := FromWikipedia(ex, filaCanada, wiki)

	if !got.Visa.Verify || got.Visa.Value != nil {
		t.Fatalf("visa = %+v, se esperaba verificar: la cita no está en la fila", got.Visa)
	}
	if !strings.Contains(got.Visa.Reason, "cita") {
		t.Errorf("motivo = %q, debería decir que la cita no aparece", got.Visa.Reason)
	}
}

func TestUnaCitaQueApareceSostieneElValorYGuardaLaFuente(t *testing.T) {
	ex := Extraction{
		Visa: Extracted[Visa]{Value: ptr(VisaConsular), Quote: "Visa required"},
		Exenciones: []ExencionCitada{{
			Exencion: Exencion{PermitKinds: []string{"visa"}, PermitCountry: "US", Visa: VisaAutorizacionElectronica},
			// El texto cambia de línea en otro lugar: los espacios no cuentan.
			Quote: "who hold a valid United States   non-immigrant visa can enter Canada",
		}},
	}

	got := FromWikipedia(ex, filaCanada, wiki)

	if got.Visa.Verify || got.Visa.Value == nil || *got.Visa.Value != VisaConsular {
		t.Fatalf("visa = %+v", got.Visa)
	}
	if len(got.Visa.Sources) != 1 || got.Visa.Sources[0].Quote != "Visa required" || got.Visa.Sources[0].Revision != "123" {
		t.Errorf("fuentes = %+v", got.Visa.Sources)
	}
	if got.Exenciones.Value == nil || len(*got.Exenciones.Value) != 1 || (*got.Exenciones.Value)[0].PermitCountry != "US" {
		t.Fatalf("exenciones = %+v", got.Exenciones)
	}
}

// Una exención facilita el viaje: si su cita no aparece, se descarta. Una
// vacuna lo exige: si su cita no aparece, el campo queda en verificar.
func TestLasCitasFallidasSiempreTiranHaciaLoMasExigente(t *testing.T) {
	ex := Extraction{
		Visa: Extracted[Visa]{Value: ptr(VisaConsular), Quote: "Visa required"},
		Exenciones: []ExencionCitada{{
			Exencion: Exencion{PermitCountry: "GB", Visa: VisaNoRequiere}, Quote: "holders of a British visa",
		}},
		Vacunas: []VacunaCitada{{Vacuna: Vacuna{Name: "fiebre amarilla", Scope: VacunaParaTodos}, Quote: "Yellow fever required"}},
	}

	got := FromWikipedia(ex, filaCanada, wiki)

	if got.Exenciones.Value == nil || len(*got.Exenciones.Value) != 0 {
		t.Errorf("exenciones = %+v, la inventada debería descartarse", got.Exenciones)
	}
	if !got.Vacunas.Verify {
		t.Errorf("vacunas = %+v, debería quedar en verificar", got.Vacunas)
	}
}

// Que la cita aparezca no alcanza: tiene que decir lo que se extrajo. Una cita
// cualquiera de la fila no puede sostener «no requiere visa».
func TestLaCitaTieneQueDecirLoQueSeExtrajo(t *testing.T) {
	ex := Extraction{
		Visa:           Extracted[Visa]{Value: ptr(VisaNoRequiere), Quote: "{{flag|Canada}}"},
		EstadiaMaxDias: Extracted[int]{Value: ptr(90), Quote: "Visa required"},
		Documento:      Extracted[Documento]{Value: ptr(DocumentoDNIOPasaporte), Quote: "Visa required"},
		Exenciones: []ExencionCitada{
			// El país de la exención tiene que estar en la cita.
			{Exencion: Exencion{PermitCountry: "GB", Visa: VisaAutorizacionElectronica},
				Quote: "who hold a valid United States non-immigrant visa can enter Canada"},
			// Y tiene que existir.
			{Exencion: Exencion{PermitCountry: "ZZ", Visa: VisaAutorizacionElectronica},
				Quote: "who hold a valid United States non-immigrant visa can enter Canada"},
		},
	}

	got := FromWikipedia(ex, filaCanada, wiki)

	if !got.Visa.Verify || !strings.Contains(got.Visa.Reason, "no sostiene") {
		t.Errorf("visa = %+v, la cita no dice «no requiere visa»", got.Visa)
	}
	if !got.EstadiaMaxDias.Verify {
		t.Errorf("estadía = %+v, la cita no tiene el número", got.EstadiaMaxDias)
	}
	if got.Documento.Value != nil {
		t.Errorf("documento = %+v, la cita no habla del DNI", got.Documento)
	}
	if len(*got.Exenciones.Value) != 0 {
		t.Errorf("exenciones = %+v, ninguna está sostenida por su cita", got.Exenciones)
	}
}

func TestLasCitasQueSostienenCadaCategoria(t *testing.T) {
	casos := map[Visa]string{
		VisaNoRequiere:              "{{yes|Visa not required}}",
		VisaConsular:                "{{no|Visa required}}",
		VisaALaLlegada:              "{{Optional|Visa on arrival}}",
		VisaElectronica:             "{{Optional|eVisa}}",
		VisaAutorizacionElectronica: "{{Optional|Electronic Travel Authorization}}",
		VisaNoAdmitido:              "{{no|Admission refused}}",
	}
	for visa, quote := range casos {
		got := FromWikipedia(Extraction{Visa: Extracted[Visa]{Value: ptr(visa), Quote: quote}}, quote, wiki)
		if got.Visa.Verify {
			t.Errorf("%s con %q debería valer: %+v", visa, quote, got.Visa)
		}
	}
	// «Visa required» no sostiene «no requiere», aunque «Visa not required» lo
	// contenga casi entero.
	got := FromWikipedia(Extraction{Visa: Extracted[Visa]{Value: ptr(VisaConsular), Quote: "Visa not required"}},
		"Visa not required", wiki)
	if !got.Visa.Verify {
		t.Errorf("«Visa not required» no sostiene visa consular: %+v", got.Visa)
	}
}

func TestLoQueWikipediaNoDiceQuedaEnVerificarSiEsExigible(t *testing.T) {
	got := FromWikipedia(Extraction{}, filaCanada, wiki)

	if !got.Visa.Verify || !got.Vigencia.Verify {
		t.Errorf("visa y vigencia sin dato deberían quedar en verificar: %+v / %+v", got.Visa, got.Vigencia)
	}
	// Sin datos de estadía o antigüedad no hay nada que exigir.
	if got.EstadiaMaxDias.Verify || got.EstadiaMaxDias.Value != nil || got.EmisionMaxAnios.Verify {
		t.Errorf("estadía o emisión sin dato no deberían exigir nada: %+v / %+v", got.EstadiaMaxDias, got.EmisionMaxAnios)
	}
	if got.Exenciones.Value == nil || len(*got.Exenciones.Value) != 0 || got.Vacunas.Value == nil || len(*got.Vacunas.Value) != 0 {
		t.Errorf("listas vacías son «no hay»: %+v / %+v", got.Exenciones, got.Vacunas)
	}
}

func wikiVisa(v Visa) Fields {
	f := FromWikipedia(Extraction{}, "", wiki)
	f.Visa = known(v, Citation{Source: SourceWikipedia, Quote: "x"})
	return f
}

func TestPassportIndexSinLaCombinacionNoEsConflicto(t *testing.T) {
	got, conflicts := Combine(Pair{"AR", "JP"}, wikiVisa(VisaNoRequiere), nil)

	if got.Visa.Verify || *got.Visa.Value != VisaNoRequiere || len(conflicts) != 0 {
		t.Fatalf("visa = %+v, conflictos = %v", got.Visa, conflicts)
	}
}

func TestSiLasFuentesNoCoincidenLaVisaQuedaEnVerificar(t *testing.T) {
	got, conflicts := Combine(Pair{"AR", "JP"}, wikiVisa(VisaNoRequiere),
		&PassportIndexEntry{Requirement: "visa required", DataDate: "2026-02-17"})

	if !got.Visa.Verify || got.Visa.Value != nil {
		t.Fatalf("visa = %+v, se esperaba verificar", got.Visa)
	}
	if !strings.Contains(got.Visa.Reason, "no coinciden") || len(got.Visa.Sources) != 2 {
		t.Errorf("visa = %+v", got.Visa)
	}
	if len(conflicts) != 1 || conflicts[0].Field != "visa" {
		t.Fatalf("conflictos = %+v", conflicts)
	}
}

// Canadá: Passport Index da la regla general y Wikipedia agrega la exención en
// las notas. La categoría coincide; la exención es otro campo.
func TestUnaExencionDeWikipediaNoEsConflictoConLaReglaGeneral(t *testing.T) {
	wikiFields := wikiVisa(VisaConsular)
	wikiFields.Exenciones = known([]Exencion{{PermitCountry: "US", Visa: VisaAutorizacionElectronica}})

	got, conflicts := Combine(Pair{"AR", "CA"}, wikiFields, &PassportIndexEntry{Requirement: "visa required"})

	if got.Visa.Verify || *got.Visa.Value != VisaConsular || len(conflicts) != 0 {
		t.Fatalf("visa = %+v, conflictos = %v", got.Visa, conflicts)
	}
	if len(*got.Exenciones.Value) != 1 {
		t.Fatalf("exenciones = %+v", got.Exenciones)
	}
}

func TestLasCategoriasEquivalentesCoinciden(t *testing.T) {
	casos := map[string]Visa{
		"90": VisaNoRequiere, "visa free": VisaNoRequiere, "eta": VisaAutorizacionElectronica,
		"e-visa": VisaElectronica, "visa on arrival": VisaALaLlegada, "visa required": VisaConsular,
		"no admission": VisaNoAdmitido,
	}
	for requirement, visa := range casos {
		_, conflicts := Combine(Pair{"AR", "XX"}, wikiVisa(visa), &PassportIndexEntry{Requirement: requirement})
		if len(conflicts) != 0 {
			t.Errorf("%q y %q deberían coincidir: %+v", requirement, visa, conflicts)
		}
	}
}

func TestSiWikipediaNoDiceLaVisaValePassportIndex(t *testing.T) {
	got, _ := Combine(Pair{"AR", "GB"}, FromWikipedia(Extraction{}, "", wiki),
		&PassportIndexEntry{Requirement: "eta", DataDate: "2026-02-17"})

	if got.Visa.Verify || *got.Visa.Value != VisaAutorizacionElectronica {
		t.Fatalf("visa = %+v", got.Visa)
	}
	if got.Visa.Sources[0].Source != SourcePassportIndex {
		t.Errorf("fuente = %+v", got.Visa.Sources)
	}
}

func TestEnEstadiaGanaLaFuenteMasExigente(t *testing.T) {
	wikiFields := wikiVisa(VisaNoRequiere)
	wikiFields.EstadiaMaxDias = known(90, Citation{Source: SourceWikipedia, Quote: "90 days"})

	got, _ := Combine(Pair{"AR", "XX"}, wikiFields, &PassportIndexEntry{Requirement: "30"})

	if got.EstadiaMaxDias.Value == nil || *got.EstadiaMaxDias.Value != 30 || got.EstadiaMaxDias.Reason == "" {
		t.Fatalf("estadía = %+v", got.EstadiaMaxDias)
	}

	got, _ = Combine(Pair{"AR", "XX"}, wikiVisa(VisaNoRequiere), &PassportIndexEntry{Requirement: "180"})
	if got.EstadiaMaxDias.Value == nil || *got.EstadiaMaxDias.Value != 180 {
		t.Fatalf("estadía solo de Passport Index = %+v", got.EstadiaMaxDias)
	}
}

func TestSchengenFijaVigenciaYAntiguedadParaNacionalidadesDeAfuera(t *testing.T) {
	got, _ := Combine(Pair{"AR", "ES"}, wikiVisa(VisaNoRequiere), nil)

	v := got.Vigencia.Value
	if v == nil || v.Amount != 3 || v.Unit != "meses" || v.Base != "salida_del_pais" {
		t.Fatalf("vigencia = %+v", got.Vigencia)
	}
	if got.EmisionMaxAnios.Value == nil || *got.EmisionMaxAnios.Value != 10 {
		t.Fatalf("emisión = %+v", got.EmisionMaxAnios)
	}
	if got.Vigencia.Sources[0].Source != SourceConstante {
		t.Errorf("fuente = %+v", got.Vigencia.Sources)
	}

	// Un italiano en España no tiene control de pasaporte Schengen.
	got, _ = Combine(Pair{"IT", "ES"}, wikiVisa(VisaNoRequiere), nil)
	if got.EmisionMaxAnios.Value != nil {
		t.Errorf("italiano en España: emisión = %+v, no debería exigirse", got.EmisionMaxAnios)
	}
}

func TestElDNIArgentinoAlcanzaEnElMercosur(t *testing.T) {
	got, _ := Combine(Pair{"AR", "BR"}, wikiVisa(VisaNoRequiere), nil)
	if got.Documento.Value == nil || *got.Documento.Value != DocumentoDNIOPasaporte {
		t.Fatalf("Brasil: documento = %+v", got.Documento)
	}

	// Donde nadie dice que alcance otro documento, se pide pasaporte: es lo más
	// exigente y nunca deja a nadie varado.
	got, _ = Combine(Pair{"AR", "US"}, wikiVisa(VisaConsular), nil)
	if got.Documento.Value == nil || *got.Documento.Value != DocumentoPasaporte || got.Documento.Verify {
		t.Fatalf("EE.UU.: documento = %+v", got.Documento)
	}
}

func TestBuscandoYSinFuentesNoSabenNada(t *testing.T) {
	for _, status := range []RuleStatus{RuleBuscando, RuleSinFuentes} {
		f := Rule{Status: status, Fields: wikiVisa(VisaNoRequiere)}.Effective()
		if !f.Visa.Verify || !f.Documento.Verify {
			t.Errorf("%s: %+v", status, f)
		}
	}
}
