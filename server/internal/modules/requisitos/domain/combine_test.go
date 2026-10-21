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
		Visa:      Extracted[Visa]{Value: ptr(VisaNoRequiere), Quote: "{{flag|Canada}}"},
		Estadia:   Extracted[Estadia]{Value: &Estadia{90, "dias"}, Quote: "Visa required"},
		Documento: Extracted[Documento]{Value: ptr(DocumentoDNIOPasaporte), Quote: "Visa required"},
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

const vigenciaPasaporte = `Countries requiring passports to be valid at least 6 months on arrival include [[Afghanistan]], [[Algeria]].

Other countries require either a passport valid on arrival or passport valid throughout the period of intended stay.`

func TestLaVigenciaSaleDeLaSeccionDePasaportes(t *testing.T) {
	seisMeses := FromWikipedia(Extraction{Vigencia: Extracted[Vigencia]{
		Value: &Vigencia{Amount: 6, Unit: "meses", Base: "llegada"},
		Quote: "Countries requiring passports to be valid at least 6 months on arrival include",
	}}, vigenciaPasaporte, wiki)
	if seisMeses.Vigencia.Verify || seisMeses.Vigencia.Value == nil || seisMeses.Vigencia.Value.Amount != 6 {
		t.Errorf("vigencia = %+v, se esperaban 6 meses", seisMeses.Vigencia)
	}

	// «Durante toda la estadía» es cero días desde la salida del país.
	estadia := FromWikipedia(Extraction{Vigencia: Extracted[Vigencia]{
		Value: &Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"},
		Quote: "passport valid throughout the period of intended stay",
	}}, vigenciaPasaporte, wiki)
	if estadia.Vigencia.Verify || estadia.Vigencia.Value == nil || estadia.Vigencia.Value.Base != "salida_del_pais" {
		t.Errorf("vigencia = %+v, se esperaba toda la estadía", estadia.Vigencia)
	}

	// Cero días con una cita que no habla de la estadía no vale.
	sinSosten := FromWikipedia(Extraction{Vigencia: Extracted[Vigencia]{
		Value: &Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"},
		Quote: "Other countries require either a passport valid on arrival",
	}}, vigenciaPasaporte, wiki)
	if !sinSosten.Vigencia.Verify {
		t.Errorf("vigencia = %+v, la cita no dice «toda la estadía»", sinSosten.Vigencia)
	}
}

// La columna de estadía dice «3 months», «6 weeks» o «1 year» en muchas filas:
// se pasa a días y se dice de dónde salió la cuenta.
func TestLaEstadiaEnMesesSePasaADias(t *testing.T) {
	fila := "| {{flag|Bahamas}}\n| {{yes|Visa not required}}\n| 3 months\n|\n| {{flag|Kenya}}\n| 6 weeks"
	casos := []struct {
		estadia Estadia
		quote   string
		dias    int
	}{
		{Estadia{3, "meses"}, "3 months", 90},
		{Estadia{6, "semanas"}, "6 weeks", 42},
		{Estadia{90, "dias"}, "Visa not required", 0},
	}
	for _, c := range casos {
		got := FromWikipedia(Extraction{Estadia: Extracted[Estadia]{Value: &c.estadia, Quote: c.quote}}, fila, wiki)
		if c.dias == 0 {
			if !got.EstadiaMaxDias.Verify {
				t.Errorf("%+v con %q: la cita no tiene el número, debería quedar en verificar", c.estadia, c.quote)
			}
			continue
		}
		if got.EstadiaMaxDias.Value == nil || *got.EstadiaMaxDias.Value != c.dias {
			t.Errorf("%+v = %+v, se esperaban %d días", c.estadia, got.EstadiaMaxDias, c.dias)
		}
		if !strings.Contains(got.EstadiaMaxDias.Reason, c.quote) {
			t.Errorf("motivo = %q, debería decir de dónde sale la cuenta", got.EstadiaMaxDias.Reason)
		}
	}
}

const filaCuba = `| {{flag|Cuba}}
| {{yes2|Tourist card required}}
| 30 days
|
* Cuba requires travelers to purchase a tourist card prior to arrival, which can be obtained from the airline or a Cuban diplomatic mission.
* Cuba requires travelers to have health insurance valid in Cuba, which can be purchased from the airline.
* Registration with the local police within 24 hours of arrival is mandatory.`

func TestLasCondicionesDeLasNotasSeGuardanConSuCita(t *testing.T) {
	ex := Extraction{Condiciones: []CondicionCitada{
		{Condicion: Condicion{Kind: CondicionSeguro, Text: "Tener seguro médico válido en Cuba"},
			Quote: "Cuba requires travelers to have health insurance valid in Cuba"},
		{Condicion: Condicion{Kind: CondicionTarjetaTurista, Text: "Comprar la tarjeta turista antes de llegar"},
			Quote: "purchase a tourist card prior to arrival"},
		{Condicion: Condicion{Kind: CondicionRegistro, Text: "Registrarse en la policía dentro de las 24 horas de llegar"},
			Quote: "Registration with the local police within 24 hours of arrival is mandatory."},
	}}

	got := FromWikipedia(ex, filaCuba, wiki)

	if got.Condiciones.Verify || got.Condiciones.Value == nil || len(*got.Condiciones.Value) != 3 {
		t.Fatalf("condiciones = %+v", got.Condiciones)
	}
	if len(got.Condiciones.Sources) != 3 || got.Condiciones.Sources[1].Quote != "purchase a tourist card prior to arrival" {
		t.Errorf("fuentes = %+v, una por condición y en orden", got.Condiciones.Sources)
	}
}

// Una condición exige algo: si su cita falla, el campo queda en verificar,
// como las vacunas.
func TestUnaCondicionSinSostenDejaLasCondicionesEnVerificar(t *testing.T) {
	casos := map[string]CondicionCitada{
		"cita inventada": {Condicion: Condicion{Kind: CondicionSeguro, Text: "Tener seguro"},
			Quote: "travel insurance is mandatory"},
		"la cita no habla de eso": {Condicion: Condicion{Kind: CondicionFondos, Text: "Mostrar fondos"},
			Quote: "Registration with the local police"},
		"texto con un enlace": {Condicion: Condicion{Kind: CondicionOtra, Text: "Pagar en https://example.com"},
			Quote: "Registration with the local police"},
	}
	for name, c := range casos {
		got := FromWikipedia(Extraction{Condiciones: []CondicionCitada{c}}, filaCuba, wiki)
		if !got.Condiciones.Verify {
			t.Errorf("%s: condiciones = %+v, debería quedar en verificar", name, got.Condiciones)
		}
	}
}

func TestLaVacunaParaQuienesVienenDePaisesEndemicosEsPorPaisDeRiesgo(t *testing.T) {
	quote := "Travellers from polio-endemic countries (Pakistan) need [[Carte Jaune]] proof of polio vaccination"
	got := FromWikipedia(Extraction{Vacunas: []VacunaCitada{{
		Vacuna: Vacuna{Name: "poliomielitis", Scope: VacunaDesdePaisRiesgo}, Quote: quote}}}, quote, wiki)
	if got.Vacunas.Verify {
		t.Fatalf("vacunas = %+v", got.Vacunas)
	}
}

func TestUnaVacunaSinSostenNoSeLlevaLasCondiciones(t *testing.T) {
	ex := Extraction{
		Vacunas: []VacunaCitada{{Vacuna: Vacuna{Name: "fiebre amarilla", Scope: VacunaParaTodos}, Quote: "inventada"}},
		Condiciones: []CondicionCitada{{Condicion: Condicion{Kind: CondicionSeguro, Text: "Tener seguro médico válido en Cuba"},
			Quote: "Cuba requires travelers to have health insurance valid in Cuba"}},
	}
	got := FromWikipedia(ex, filaCuba, wiki)
	if !got.Vacunas.Verify || got.Condiciones.Value == nil || len(*got.Condiciones.Value) != 1 {
		t.Fatalf("vacunas = %+v, condiciones = %+v", got.Vacunas, got.Condiciones)
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
	if got.Exenciones.Value == nil || len(*got.Exenciones.Value) != 0 || got.Vacunas.Value == nil || len(*got.Vacunas.Value) != 0 ||
		got.Condiciones.Value == nil || len(*got.Condiciones.Value) != 0 {
		t.Errorf("listas vacías son «no hay»: %+v / %+v / %+v", got.Exenciones, got.Vacunas, got.Condiciones)
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

// Estados Unidos pide 6 meses de vigencia después de la estadía, salvo a los
// países de la lista de la CBP, que solo necesitan el pasaporte vigente
// durante la estadía. Es una regla fija, con su fuente.
func TestEstadosUnidosPideSeisMesesSalvoALaListaDeExceptuados(t *testing.T) {
	ar, _ := Combine(Pair{Nationality: "AR", Country: "US"}, Fields{Vigencia: verify[Vigencia]("x")}, nil)
	v := ar.Vigencia
	if v.Verify || v.Value == nil || v.Value.Amount != 0 || v.Value.Base != "salida_del_pais" ||
		len(v.Sources) != 1 || v.Sources[0].Source != SourceConstante || !strings.Contains(v.Reason, "exceptuad") {
		t.Fatalf("argentino: %+v", v)
	}

	cn, _ := Combine(Pair{Nationality: "CN", Country: "US"}, Fields{}, nil)
	if v := cn.Vigencia; v.Value == nil || v.Value.Amount != 6 || v.Value.Unit != "meses" || v.Value.Base != "salida_del_pais" {
		t.Fatalf("chino: %+v", v)
	}

	// A otro destino no se aplica.
	if br, _ := Combine(Pair{Nationality: "CN", Country: "BR"}, Fields{Vigencia: verify[Vigencia]("x")}, nil); !br.Vigencia.Verify {
		t.Fatalf("Brasil: %+v", br.Vigencia)
	}
}
