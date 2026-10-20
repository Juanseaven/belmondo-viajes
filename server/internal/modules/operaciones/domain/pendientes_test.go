package domain_test

import (
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
)

var hoy = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

func dia(offset int) time.Time   { return hoy.AddDate(0, 0, offset) }
func ptr(t time.Time) *time.Time { return &t }

func control(id string, offset int, critical bool) domain.PendingItem {
	return domain.PendingItem{Kind: domain.PendingControl, ID: id, Task: "tarea " + id, DueAt: dia(offset), Critical: critical}
}

func TestUnPlazoVencidoPoneElViajeEnAtrasado(t *testing.T) {
	got, ok := domain.ClassifyPending(hoy, nil, []domain.PendingItem{control("a", -1, false)})
	if !ok || got.Group != domain.GroupAtrasado || got.Main.ID != "a" {
		t.Fatalf("clasificación = %#v, %v", got, ok)
	}
}

func TestLoQueVenceEnLaSemanaVaEnEstaSemana(t *testing.T) {
	for _, offset := range []int{0, 3, 7} {
		got, ok := domain.ClassifyPending(hoy, nil, []domain.PendingItem{control("a", offset, false)})
		if !ok || got.Group != domain.GroupSemana {
			t.Errorf("vence en %d días: %#v, %v", offset, got, ok)
		}
	}
}

func TestLoQueVenceMasAdelanteNoAparece(t *testing.T) {
	if got, ok := domain.ClassifyPending(hoy, nil, []domain.PendingItem{control("a", 8, true)}); ok {
		t.Fatalf("un plazo a 8 días no pide nada esta semana: %#v", got)
	}
}

// Un viaje aparece en un solo grupo: el más urgente que le toque. La tarea
// principal sale de ese grupo, y «y N cosas más» cuenta lo atrasado y lo de
// la semana, no lo que vence más adelante.
func TestUnViajeConCosasDeDosGruposQuedaEnElMasUrgente(t *testing.T) {
	items := []domain.PendingItem{control("semana", 2, true), control("atrasado", -3, false), control("lejos", 30, true)}
	got, ok := domain.ClassifyPending(hoy, ptr(dia(5)), items)
	if !ok || got.Group != domain.GroupAtrasado || got.Main.ID != "atrasado" || got.More != 1 {
		t.Fatalf("clasificación = %#v, %v", got, ok)
	}
}

// La misma regla que «Lo que sigue» en la ficha: crítico primero, después el
// plazo más cercano.
func TestLaTareaPrincipalEsLaCriticaYDespuesLaMasProxima(t *testing.T) {
	items := []domain.PendingItem{control("normal-viejo", -10, false), control("critico", -1, true), control("normal", -5, false)}
	got, _ := domain.ClassifyPending(hoy, nil, items)
	if got.Main.ID != "critico" || got.More != 2 {
		t.Fatalf("principal = %s, más = %d", got.Main.ID, got.More)
	}
}

func TestUnViajeAlDiaQueSaleProntoApareceConTodoListo(t *testing.T) {
	got, ok := domain.ClassifyPending(hoy, ptr(dia(10)), nil)
	if !ok || got.Group != domain.GroupSalen || got.Main != nil {
		t.Fatalf("clasificación = %#v, %v", got, ok)
	}
	if _, ok := domain.ClassifyPending(hoy, ptr(dia(15)), nil); ok {
		t.Fatalf("un viaje que sale en 15 días no está en los próximos 14")
	}
	if _, ok := domain.ClassifyPending(hoy, ptr(dia(-1)), nil); ok {
		t.Fatalf("un viaje que ya salió no está por salir")
	}
}

func TestUnViajeQueSaleProntoMuestraLoProximoQueVence(t *testing.T) {
	got, ok := domain.ClassifyPending(hoy, ptr(dia(12)), []domain.PendingItem{control("b", 11, false), control("a", 9, false)})
	if !ok || got.Group != domain.GroupSalen || got.Main == nil || got.Main.ID != "a" || got.More != 0 {
		t.Fatalf("clasificación = %#v, %v", got, ok)
	}
}

func TestLaSenaVencidaCuentaComoCritica(t *testing.T) {
	sena := domain.DepositItem(hoy.AddDate(0, 0, -1), float64ptr(800))
	got, _ := domain.ClassifyPending(hoy, nil, []domain.PendingItem{control("a", -5, false), sena})
	if got.Main.Kind != domain.PendingDeposit || got.Main.Task != "Cobrar la seña de USD 800" {
		t.Fatalf("principal = %#v", got.Main)
	}
}

func TestLaSenaSinMontoSeDiceIgual(t *testing.T) {
	if task := domain.DepositItem(hoy, nil).Task; task != "Cobrar la seña" {
		t.Fatalf("tarea = %q", task)
	}
	if task := domain.DepositItem(hoy, float64ptr(5900.5)).Task; task != "Cobrar la seña de USD 5.900,50" {
		t.Fatalf("tarea = %q", task)
	}
}

func TestLoQueSigueEsLaMasUrgenteSinImportarLaVentana(t *testing.T) {
	next := domain.NextPending([]domain.PendingItem{control("lejos", 40, false), control("critico-lejos", 60, true)})
	if next == nil || next.ID != "critico-lejos" {
		t.Fatalf("lo que sigue = %#v", next)
	}
	if domain.NextPending(nil) != nil {
		t.Fatalf("sin pendientes no hay nada que seguir")
	}
}

func float64ptr(v float64) *float64 { return &v }
