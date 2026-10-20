package postgres

import (
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

func TestElNombreDelViajeSaleDeLosDestinos(t *testing.T) {
	casos := map[string][]store.Stop{
		"México":                   {{Country: "PA", Role: "escala"}, {Country: "MX", Role: "destino"}},
		"Italia y Francia":         {{Country: "IT", Role: "destino"}, {Country: "FR", Role: "destino"}},
		"Francia e Italia":         {{Country: "FR", Role: "destino"}, {Country: "IT", Role: "destino"}},
		"Italia, Francia y España": {{Country: "IT", Role: "destino"}, {Country: "FR", Role: "destino"}, {Country: "ES", Role: "destino"}},
		"":                         {{Country: "PA", Role: "escala"}},
	}
	for want, stops := range casos {
		if got := TripName(stops); got != want {
			t.Errorf("TripName(%v) = %q, se esperaba %q", stops, got, want)
		}
	}
}
