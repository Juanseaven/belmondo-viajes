package domain_test

import (
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
)

// La plantilla es el proceso de la agencia transcrito: las dos planillas de
// papel, renglón por renglón. Su composición es la afirmación central del
// diseño —11 renglones que el sistema cierra solo, 8 que se cierran mandando un
// mensaje y 12 que una persona confirma a mano— así que se verifica.
func TestPlantillaTranscribeLasDosPlanillas(t *testing.T) {
	items := domain.Checklist()

	if len(items) != 31 {
		t.Fatalf("la plantilla tiene %d renglones, se esperaban 31", len(items))
	}

	porEtapa := map[domain.Stage]int{}
	porCierre := map[domain.Closing]int{}
	for _, item := range items {
		porEtapa[item.Stage]++
		porCierre[item.Closing]++
	}

	if porEtapa[domain.StageCarga] != 14 {
		t.Errorf("hoja de carga: %d renglones, se esperaban 14", porEtapa[domain.StageCarga])
	}
	if porEtapa[domain.StageControl] != 17 {
		t.Errorf("hoja de control: %d renglones, se esperaban 17", porEtapa[domain.StageControl])
	}

	if porCierre[domain.ClosingDerivado] != 11 {
		t.Errorf("cierre derivado: %d renglones, se esperaban 11", porCierre[domain.ClosingDerivado])
	}
	if porCierre[domain.ClosingAccion] != 8 {
		t.Errorf("cierre por acción: %d renglones, se esperaban 8", porCierre[domain.ClosingAccion])
	}
	if porCierre[domain.ClosingManual] != 12 {
		t.Errorf("cierre manual: %d renglones, se esperaban 12", porCierre[domain.ClosingManual])
	}
}

// El id de cada renglón viaja a la base: es la clave con la que cada viaje
// guarda el estado de su copia. Uno repetido haría que dos controles distintos
// compartieran estado sin que nada lo avise.
func TestCadaRenglonTieneUnIdPropio(t *testing.T) {
	vistos := map[string]bool{}
	for _, item := range domain.Checklist() {
		if item.ID == "" {
			t.Errorf("renglón sin id: %q", item.Title)
			continue
		}
		if vistos[item.ID] {
			t.Errorf("id repetido: %q", item.ID)
		}
		vistos[item.ID] = true
	}
}

// Cada forma de cierre necesita una pieza distinta para poder dibujarse: el
// derivado, qué dato lo cumple; el de acción, el botón y el mensaje que
// precarga; el manual, la pregunta contra la que comparar. Sin esa pieza el
// renglón llega a la pantalla mudo y quien lo mira no sabe qué hacer.
func TestCadaRenglonTraeLoQueSuCierreNecesita(t *testing.T) {
	for _, item := range domain.Checklist() {
		switch item.Closing {
		case domain.ClosingDerivado:
			if item.Derives == "" {
				t.Errorf("%s: derivado sin el dato que lo cumple", item.ID)
			}
		case domain.ClosingAccion:
			if item.ActionLabel == "" || item.Message == "" {
				t.Errorf("%s: por acción sin botón o sin mensaje", item.ID)
			}
		case domain.ClosingManual:
			if item.Prompt == "" {
				t.Errorf("%s: manual sin la pregunta que hay que responder", item.ID)
			}
		default:
			t.Errorf("%s: forma de cierre desconocida %q", item.ID, item.Closing)
		}
	}
}
