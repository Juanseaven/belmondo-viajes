package domain_test

import (
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
)

func fecha(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// Los renglones de carga cuentan desde que se abre el viaje; los de control,
// hacia atrás desde la salida. Que el plazo sea un dato de la plantilla y no una
// fecha que alguien tipea es lo que permite que mover la salida corra todos los
// vencimientos sin intervención.
func TestVencimientoSeCalculaDesdeLasFechasDelViaje(t *testing.T) {
	alta := fecha("2026-08-20")
	salida := fecha("2026-12-10")
	regreso := fecha("2026-12-24")

	casos := []struct {
		id       string
		esperado string
	}{
		{"carga-preexistencia", "2026-08-22"}, // alta + 2
		{"carga-comprobante", "2026-08-23"},   // alta + 3
		{"carga-acepto", "2026-08-25"},        // alta + 5
		{"carga-vouchers", "2026-12-03"},      // salida - 7
		{"control-visa", "2026-10-26"},        // salida - 45
		{"control-vto-doc", "2026-10-11"},     // salida - 60
		{"control-checkin", "2026-12-08"},     // salida - 2
		{"control-checkout", "2026-12-22"},    // regreso - 2
		{"control-traslados", "2026-11-25"},   // salida - 15
	}

	for _, caso := range casos {
		item := renglon(t, caso.id)
		got, ok := item.DueAt(alta, salida, regreso)
		if !ok {
			t.Errorf("%s: no devolvió vencimiento", caso.id)
			continue
		}
		if got.Format("2006-01-02") != caso.esperado {
			t.Errorf("%s: vence %s, se esperaba %s", caso.id, got.Format("2006-01-02"), caso.esperado)
		}
	}
}

// Un control que el sistema cierra solo no tiene nada que vencer: no hay nadie a
// quien apurar. Darle fecha lo pondría a competir en el chip de la bandeja con
// los que sí necesitan que alguien haga algo.
func TestLosDerivadosNoVencen(t *testing.T) {
	alta, salida, regreso := fecha("2026-08-20"), fecha("2026-12-10"), fecha("2026-12-24")

	for _, item := range domain.Checklist() {
		if item.Closing != domain.ClosingDerivado {
			continue
		}
		if _, ok := item.DueAt(alta, salida, regreso); ok {
			t.Errorf("%s: un derivado no debería vencer", item.ID)
		}
	}
}

func TestLosControlesDeViajeEsperanHastaTenerSuFechaBase(t *testing.T) {
	alta := fecha("2026-08-20")
	for _, id := range []string{"control-visa", "control-checkin", "control-checkout"} {
		item := renglon(t, id)
		if due, ok := item.DueAt(alta, time.Time{}, time.Time{}); ok {
			t.Fatalf("%s calculó %s sin fecha de viaje", id, due.Format("2006-01-02"))
		}
	}
}

// Crítico no es «importante»: es lo que, si falla, arruina el viaje o deja a la
// agencia expuesta. Un pasaporte vencido no tiene arreglo en el aeropuerto y un
// nombre mal escrito en un ticket aéreo no se corrige.
func TestLosCriticosSonLosQueArruinanElViaje(t *testing.T) {
	esperados := map[string]bool{
		"carga-acepto":            true,
		"control-orden":           true,
		"control-fechas-vuelo":    true,
		"control-destino":         true,
		"control-visa":            true,
		"control-vto-doc":         true,
		"control-no-reembolsable": true,
	}

	for _, item := range domain.Checklist() {
		if item.Critical != esperados[item.ID] {
			t.Errorf("%s: crítico=%v, se esperaba %v", item.ID, item.Critical, esperados[item.ID])
		}
	}
}

func renglon(t *testing.T, id string) domain.ChecklistItem {
	t.Helper()
	for _, item := range domain.Checklist() {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("no existe el renglón %q en la plantilla", id)
	return domain.ChecklistItem{}
}
