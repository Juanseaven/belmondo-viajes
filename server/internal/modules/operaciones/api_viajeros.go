package operaciones

import (
	"net/http"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Los datos del viaje y de sus pasajeros que el motor de requisitos lee:
// itinerario, documentos y permisos.

// dateField es una fecha opcional del cuerpo y dónde guardarla.
type dateField struct {
	raw string
	dst **time.Time
}

// optionalDates traduce fechas opcionales del cuerpo. Devuelve el nombre del
// primer campo inválido para que el error diga cuál.
func optionalDates(fields map[string]dateField) (string, bool) {
	for name, f := range fields {
		if f.raw == "" {
			continue
		}
		date, err := parseDate(f.raw)
		if err != nil {
			return name, false
		}
		*f.dst = &date
	}
	return "", true
}

func (m *Module) putItinerary(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Itinerary []struct {
			Country  string `json:"country"`
			Role     string `json:"role"`
			ArriveAt string `json:"arriveAt"`
			LeaveAt  string `json:"leaveAt"`
		} `json:"itinerary"`
	}
	if !decode(w, r, &body) {
		return
	}
	stops := make([]store.Stop, len(body.Itinerary))
	for i, raw := range body.Itinerary {
		stops[i] = store.Stop{Country: strings.ToUpper(strings.TrimSpace(raw.Country)), Role: raw.Role}
		if name, ok := optionalDates(map[string]dateField{
			"llegada": {raw.ArriveAt, &stops[i].ArriveAt}, "salida": {raw.LeaveAt, &stops[i].LeaveAt},
		}); !ok {
			httpx.BadRequest(w, "fecha de "+name+" inválida")
			return
		}
	}
	tripID := r.PathValue("id")
	if err := m.repo.PutItinerary(r.Context(), tripID, stops); err != nil {
		m.storeError(w, "guardando el itinerario", err)
		return
	}
	m.emit("trip.updated", map[string]any{"tripId": tripID})
	m.afterChange(r.Context(), tripID)
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) addDocument(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type      string `json:"type"`
		Country   string `json:"country"`
		Number    string `json:"number"`
		IssuedAt  string `json:"issuedAt"`
		ExpiresAt string `json:"expiresAt"`
	}
	if !decode(w, r, &body) {
		return
	}
	n := store.NewDocument{PassengerID: r.PathValue("id"), Type: body.Type,
		Country: strings.ToUpper(strings.TrimSpace(body.Country)), Number: strings.TrimSpace(body.Number)}
	if name, ok := optionalDates(map[string]dateField{
		"emisión": {body.IssuedAt, &n.IssuedAt}, "vencimiento": {body.ExpiresAt, &n.ExpiresAt},
	}); !ok {
		httpx.BadRequest(w, "fecha de "+name+" inválida")
		return
	}
	d, err := m.repo.AddDocument(r.Context(), n)
	if err != nil {
		m.storeError(w, "creando el documento", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": d.TripID, "passengerId": d.PassengerID})
	m.afterChange(r.Context(), d.TripID)
	httpx.JSON(w, http.StatusCreated, d)
}

func (m *Module) updateDocument(w http.ResponseWriter, r *http.Request) {
	var patch store.DocumentPatch
	if !decode(w, r, &patch) {
		return
	}
	d, err := m.repo.UpdateDocument(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		m.storeError(w, "actualizando el documento", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": d.TripID, "passengerId": d.PassengerID})
	m.afterChange(r.Context(), d.TripID)
	httpx.JSON(w, http.StatusOK, d)
}

func (m *Module) deleteDocument(w http.ResponseWriter, r *http.Request) {
	tripID, err := m.repo.DeleteDocument(r.Context(), r.PathValue("id"))
	if err != nil {
		m.storeError(w, "borrando el documento", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": tripID})
	m.afterChange(r.Context(), tripID)
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) addPermit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind      string `json:"kind"`
		Country   string `json:"country"`
		Label     string `json:"label"`
		ValidFrom string `json:"validFrom"`
		ExpiresAt string `json:"expiresAt"`
	}
	if !decode(w, r, &body) {
		return
	}
	n := store.NewPermit{PassengerID: r.PathValue("id"), Kind: body.Kind,
		Country: strings.ToUpper(strings.TrimSpace(body.Country)), Label: strings.TrimSpace(body.Label)}
	if name, ok := optionalDates(map[string]dateField{
		"inicio": {body.ValidFrom, &n.ValidFrom}, "vencimiento": {body.ExpiresAt, &n.ExpiresAt},
	}); !ok {
		httpx.BadRequest(w, "fecha de "+name+" inválida")
		return
	}
	pm, err := m.repo.AddPermit(r.Context(), n)
	if err != nil {
		m.storeError(w, "creando el permiso", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": pm.TripID, "passengerId": pm.PassengerID})
	m.afterChange(r.Context(), pm.TripID)
	httpx.JSON(w, http.StatusCreated, pm)
}

func (m *Module) updatePermit(w http.ResponseWriter, r *http.Request) {
	var patch store.PermitPatch
	if !decode(w, r, &patch) {
		return
	}
	pm, err := m.repo.UpdatePermit(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		m.storeError(w, "actualizando el permiso", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": pm.TripID, "passengerId": pm.PassengerID})
	m.afterChange(r.Context(), pm.TripID)
	httpx.JSON(w, http.StatusOK, pm)
}

func (m *Module) deletePermit(w http.ResponseWriter, r *http.Request) {
	tripID, err := m.repo.DeletePermit(r.Context(), r.PathValue("id"))
	if err != nil {
		m.storeError(w, "borrando el permiso", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": tripID})
	m.afterChange(r.Context(), tripID)
	w.WriteHeader(http.StatusNoContent)
}
