package operaciones

import (
	"context"
	"net/http"
	"strings"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/auth"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/evaluacion"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store/postgres"
	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// ruleSource es lo que operaciones necesita del módulo requisitos: las reglas
// de un conjunto de combinaciones. Las que no existen nacen en «buscando».
type ruleSource interface {
	Rules(ctx context.Context, pairs []reglas.Pair) (map[reglas.Pair]reglas.Rule, error)
}

// reevaluate cruza el viaje con las reglas y guarda el resultado. Sin módulo de
// requisitos, todo queda en verificar: la evaluación nunca inventa un «OK».
func (m *Module) reevaluate(ctx context.Context, tripID string) error {
	in, err := m.repo.EvaluationInput(ctx, tripID)
	if err != nil {
		return err
	}
	rules := map[reglas.Pair]reglas.Rule{}
	if pairs := evaluacion.Pairs(in.Trip); m.rules != nil && len(pairs) > 0 {
		if rules, err = m.rules.Rules(ctx, pairs); err != nil {
			return err
		}
	}
	return m.repo.SaveRequirements(ctx, tripID, evaluacion.Evaluate(in.Trip, rules, in.Confirmations))
}

// afterChange re-evalúa después de una escritura que puede cambiar los
// requisitos. Un error acá no deshace lo que se guardó: queda en el log y la
// próxima escritura, o el próximo cambio de regla, vuelve a evaluar.
func (m *Module) afterChange(ctx context.Context, tripID string) {
	if tripID == "" {
		return
	}
	if err := m.reevaluate(ctx, tripID); err != nil {
		m.log.Error("no se pudo evaluar los requisitos del viaje", "trip", tripID, "error", err)
		return
	}
	m.emit("requirements.changed", map[string]any{"tripId": tripID})
}

// OnRulesChanged re-evalúa los viajes abiertos que usan las reglas que
// cambiaron. Lo conecta cmd/api/main.go con el gancho del módulo requisitos.
func (m *Module) OnRulesChanged(ctx context.Context, pairs []reglas.Pair) {
	ids, err := m.repo.OpenTripsUsing(ctx, pairs)
	if err != nil {
		m.log.Error("no se pudo buscar los viajes de las reglas que cambiaron", "error", err)
		return
	}
	for _, id := range ids {
		m.afterChange(ctx, id)
	}
}

// evaluateOpenTrips evalúa todos los viajes abiertos al arrancar: los que
// existían antes de los requisitos, y los que cambiaron mientras el servidor
// estuvo apagado.
func (m *Module) evaluateOpenTrips(ctx context.Context) {
	ids, err := m.repo.OpenTrips(ctx)
	if err != nil {
		m.log.Error("no se pudo listar los viajes abiertos", "error", err)
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		m.afterChange(ctx, id)
	}
}

func (m *Module) confirmRequirement(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PassengerID string `json:"passengerId"`
		Country     string `json:"country"`
		Key         string `json:"key"`
		MessageID   string `json:"messageId"`
	}
	if !decode(w, r, &body) {
		return
	}
	tripID := r.PathValue("id")
	if err := m.repo.Confirm(r.Context(), store.NewConfirmation{TripID: tripID, PassengerID: body.PassengerID,
		Country: strings.ToUpper(body.Country), Key: body.Key, UserID: auth.UserID(r.Context()),
		MessageID: body.MessageID}); err != nil {
		m.storeError(w, "confirmando el requisito", err)
		return
	}
	m.afterChange(r.Context(), tripID)
	w.WriteHeader(http.StatusNoContent)
}

// withPreview agrega, a un viaje con países y sin pasajeros, lo que esos países
// piden a un pasaporte argentino. Pedir las reglas también las pone a buscar si
// no existían. Si falla, la ficha sale igual, sin la vista previa.
func (m *Module) withPreview(ctx context.Context, d *store.TripDetailData) {
	if m.rules == nil || len(d.Passengers) > 0 || len(d.Itinerary) == 0 {
		return
	}
	trip := evaluacion.Trip{Origin: d.OriginCountry}
	for _, s := range d.Itinerary {
		trip.Itinerary = append(trip.Itinerary, evaluacion.Stop{Country: s.Country, Role: s.Role})
	}
	rules := map[reglas.Pair]reglas.Rule{}
	if pairs := evaluacion.Pairs(trip); len(pairs) > 0 {
		var err error
		if rules, err = m.rules.Rules(ctx, pairs); err != nil {
			m.log.Error("no se pudo leer las reglas de la vista previa", "trip", d.ID, "error", err)
			return
		}
	}
	d.Preview = evaluacion.Preview(trip, rules)
}

// withNotice arma el mensaje de «Requisitos migratorios» con lo que falta de
// cada pasajero, en lugar del texto genérico de la plantilla.
func withNotice(d *store.TripDetailData) {
	trip := evaluacion.Trip{DepartAt: d.DepartAt, ReturnAt: d.ReturnAt}
	for _, s := range d.Itinerary {
		trip.Itinerary = append(trip.Itinerary, evaluacion.Stop{Country: s.Country, Role: s.Role})
	}
	holder := d.Customer.Name
	for _, p := range d.Passengers {
		trip.Passengers = append(trip.Passengers, postgres.ToEvaluation(p))
		if p.IsHolder {
			holder = p.FirstName
		}
	}
	reqs := make([]evaluacion.Requirement, len(d.Requirements))
	for i, r := range d.Requirements {
		reqs[i] = r.Requirement
	}
	for i := range d.Controls {
		if d.Controls[i].ItemID == "control-visa" {
			d.Controls[i].Message = evaluacion.Notice(holder, trip, reqs)
		}
	}
}
