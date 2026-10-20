package evaluacion

import (
	"testing"

	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

var (
	dniAR      = Document{ID: "dni", Type: "DNI", Country: "AR", ExpiresAt: d("2030-06-19")}
	pasAR      = Document{ID: "pas", Type: "Pasaporte", Country: "AR", ExpiresAt: d("2031-01-01")}
	pasVencido = Document{ID: "pas-viejo", Type: "Pasaporte", Country: "AR", ExpiresAt: d("2026-10-24")}
	pasIT      = Document{ID: "pas-it", Type: "Pasaporte", Country: "IT", ExpiresAt: d("2031-01-01")}
)

func conDocs(current *Document, docs ...Document) Passenger {
	p := ana(current)
	p.Docs = docs
	return p
}

func elegido(t *testing.T, trip Trip, rules map[reglas.Pair]reglas.Rule) string {
	t.Helper()
	return ChooseDocuments(trip, rules).Passengers[0].Doc.ID
}

func TestAUnPaisQueAceptaDNIViajaConDNI(t *testing.T) {
	rules := regla("AR", "BR", func(f *reglas.Fields) { f.Documento = field(reglas.DocumentoDNIOPasaporte) })
	trip := viaje("BR", conDocs(&pasAR, pasAR, dniAR))
	if got := elegido(t, trip, rules); got != "dni" {
		t.Fatalf("a Brasil eligió %s", got)
	}
}

func TestAUnPaisQuePidePasaporteViajaConPasaporte(t *testing.T) {
	rules := regla("AR", "US", func(f *reglas.Fields) { f.Documento = field(reglas.DocumentoPasaporte) })
	trip := viaje("US", conDocs(&dniAR, dniAR, pasAR))
	if got := elegido(t, trip, rules); got != "pas" {
		t.Fatalf("a Estados Unidos eligió %s", got)
	}
}

func TestEntreDosPasaportesGanaElQueCumpleLaVigencia(t *testing.T) {
	rules := regla("AR", "ES", func(f *reglas.Fields) {
		f.Vigencia = field(reglas.Vigencia{Amount: 3, Unit: "meses", Base: "salida_del_pais"})
	})
	trip := viaje("ES", conDocs(&pasVencido, pasVencido, pasAR))
	if got := elegido(t, trip, rules); got != "pas" {
		t.Fatalf("eligió %s, que vence antes de lo que pide España", got)
	}
}

func TestLaDobleNacionalidadEligeElPasaporteQueNoPideVisa(t *testing.T) {
	rules := merge(
		regla("AR", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
		regla("IT", "US", func(f *reglas.Fields) { f.Visa = field(reglas.VisaAutorizacionElectronica) }),
	)
	trip := viaje("US", conDocs(&pasAR, pasAR, pasIT))
	// Los dos piden algo; el italiano pide la ESTA, el argentino la visa: ninguno
	// tiene el permiso cargado, así que empatan en «falta» y queda el que tenía.
	if got := elegido(t, trip, rules); got != "pas" {
		t.Fatalf("en empate eligió %s", got)
	}

	rules = merge(
		regla("AR", "ES", func(f *reglas.Fields) { f.Visa = field(reglas.VisaConsular) }),
		regla("IT", "ES", nil),
	)
	if got := elegido(t, viaje("ES", conDocs(&pasAR, pasAR, pasIT)), rules); got != "pas-it" {
		t.Fatalf("a España eligió %s y no el italiano, que no pide visa", got)
	}
}

func TestElElegidoAManoNoSeToca(t *testing.T) {
	rules := regla("AR", "BR", func(f *reglas.Fields) { f.Documento = field(reglas.DocumentoDNIOPasaporte) })
	p := conDocs(&pasAR, pasAR, dniAR)
	p.DocManual = true
	if got := elegido(t, viaje("BR", p), rules); got != "pas" {
		t.Fatalf("cambió el documento elegido a mano: %s", got)
	}
}

func TestParaElegirSePidenLasReglasDeTodosLosDocumentos(t *testing.T) {
	trip := viaje("US", conDocs(&pasAR, pasAR, pasIT))
	pairs := Pairs(trip)
	if len(pairs) != 2 {
		t.Fatalf("pares = %+v", pairs)
	}
	manual := conDocs(&pasAR, pasAR, pasIT)
	manual.DocManual = true
	if pairs := Pairs(viaje("US", manual)); len(pairs) != 1 || pairs[0].Nationality != "AR" {
		t.Fatalf("elegido a mano, solo su regla: %+v", pairs)
	}
}
