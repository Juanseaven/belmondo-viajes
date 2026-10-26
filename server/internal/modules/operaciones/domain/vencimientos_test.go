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

func TestLaPlantillaNoInventaVencimientos(t *testing.T) {
	alta := fecha("2026-10-22")
	salida := fecha("2026-12-10")
	regreso := fecha("2026-12-24")

	for _, item := range domain.Checklist() {
		if due, ok := item.DueAt(alta, salida, regreso); ok {
			t.Errorf("%s inventó el vencimiento %s", item.ID, due.Format("2006-01-02"))
		}
	}
}

// Crítico no es «importante»: es lo que, si falla, arruina el viaje o deja a
// la agencia expuesta. Quitar fechas inventadas no cambia esa clasificación.
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
