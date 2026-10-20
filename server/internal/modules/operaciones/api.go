package operaciones

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/auth"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

type handler func(http.ResponseWriter, *http.Request)

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h(w, r) }

func jsonBytes(value any) (json.RawMessage, error) { return json.Marshal(value) }
func nowUTC() time.Time                            { return time.Now().UTC() }

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.BadRequest(w, "JSON inválido")
		return false
	}
	return true
}

func parseDate(value string) (time.Time, error) { return time.Parse("2006-01-02", value) }

// agencyZone es la hora de la agencia. «Hoy» en la lista de pendientes es el
// día de Buenos Aires: a las 22 h ya es mañana en UTC.
var agencyZone = mustLoadZone("America/Argentina/Buenos_Aires")

func mustLoadZone(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("operaciones: huso horario %s: %v", name, err))
	}
	return zone
}

// today es el día calendario de la agencia, a la medianoche UTC: la misma forma
// en que Postgres devuelve una columna date, para poder compararlos.
func (m *Module) today() time.Time {
	now := m.clock().In(agencyZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func (m *Module) pending(w http.ResponseWriter, r *http.Request) {
	items, err := m.repo.Pending(r.Context(), m.today())
	if err != nil {
		m.storeError(w, "armando los pendientes", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

var tripStatuses = map[string]domain.TripStatus{
	"consulta": domain.TripConsulta, "cotizado": domain.TripCotizado, "confirmado": domain.TripConfirmado,
	"en viaje": domain.TripEnViaje, "cerrado": domain.TripCerrado,
}

// tripFilter traduce los parámetros de «Todos los viajes». Sin estado, se ven
// los abiertos; «todos» quita el filtro.
func (m *Module) tripFilter(r *http.Request) (store.TripFilter, error) {
	query := r.URL.Query()
	f := store.TripFilter{
		Query: strings.TrimSpace(query.Get("q")), Cursor: query.Get("cursor"),
		LateOnly: query.Get("atrasados") == "1", Today: m.today(),
	}
	switch estado := query.Get("estado"); estado {
	case "", "abiertos":
		f.OpenOnly = true
	case "todos":
	default:
		for _, value := range strings.Split(estado, ",") {
			status, ok := tripStatuses[strings.TrimSpace(value)]
			if !ok {
				return f, fmt.Errorf("estado desconocido: %s", value)
			}
			f.Statuses = append(f.Statuses, status)
		}
	}
	for param, dst := range map[string]**time.Time{"salidaDesde": &f.DepartFrom, "salidaHasta": &f.DepartTo} {
		if value := query.Get(param); value != "" {
			date, err := parseDate(value)
			if err != nil {
				return f, fmt.Errorf("fecha inválida en %s", param)
			}
			*dst = &date
		}
	}
	switch vendedor := query.Get("vendedor"); vendedor {
	case "":
	case "ninguno":
		f.SellerNone = true
	default:
		f.SellerID = vendedor
	}
	switch orden := store.TripSort(query.Get("orden")); orden {
	case "", store.SortSalida, store.SortNumero, store.SortTotal:
		f.Sort = orden
	default:
		return f, fmt.Errorf("orden desconocido: %s", orden)
	}
	if limit := query.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > 100 {
			return f, fmt.Errorf("limit entre 1 y 100")
		}
		f.Limit = n
	}
	return f, nil
}

// listTrips responde dos preguntas. Con contacto, los viajes de ese contacto
// para la ficha; sin él, la consulta de «Todos los viajes».
func (m *Module) listTrips(w http.ResponseWriter, r *http.Request) {
	contactID := r.URL.Query().Get("contacto")
	if contactID == "" {
		filter, err := m.tripFilter(r)
		if err != nil {
			httpx.BadRequest(w, err.Error())
			return
		}
		page, err := m.repo.ListTrips(r.Context(), filter)
		if err != nil {
			m.storeError(w, "listando viajes", err)
			return
		}
		httpx.JSON(w, http.StatusOK, page)
		return
	}
	trips, err := m.repo.TripsByContact(r.Context(), contactID)
	if err != nil {
		m.storeError(w, "listando viajes", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": trips})
}

func (m *Module) createTrip(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ContactID     string     `json:"contactId"`
		Destination   string     `json:"destination"`
		OriginCountry string     `json:"originCountry"`
		Itinerary     []stopBody `json:"itinerary"`
		DepartAt      string     `json:"departAt"`
		ReturnAt      string     `json:"returnAt"`
	}
	if !decode(w, r, &body) {
		return
	}
	stops, ok := parseStops(w, body.Itinerary)
	if !ok {
		return
	}
	var departAt, returnAt time.Time
	var err error
	if body.DepartAt != "" {
		departAt, err = parseDate(body.DepartAt)
		if err != nil {
			httpx.BadRequest(w, "fecha de salida inválida")
			return
		}
	}
	if body.ReturnAt != "" {
		returnAt, err = parseDate(body.ReturnAt)
		if err != nil || (!departAt.IsZero() && returnAt.Before(departAt)) {
			httpx.BadRequest(w, "fecha de regreso inválida")
			return
		}
	}
	if strings.TrimSpace(body.ContactID) == "" {
		httpx.BadRequest(w, "el contacto es obligatorio")
		return
	}
	trip, err := m.repo.CreateTrip(r.Context(), store.NewTrip{
		ContactID: body.ContactID, Destination: strings.TrimSpace(body.Destination),
		Origin: countryCode(body.OriginCountry), Itinerary: stops,
		DepartAt: departAt, ReturnAt: returnAt, CreatedBy: auth.UserID(r.Context()),
	})
	if err != nil {
		m.storeError(w, "creando el viaje", err)
		return
	}
	m.emit("trip.created", map[string]any{"tripId": trip.ID, "contactIds": []string{body.ContactID}})
	m.afterChange(r.Context(), trip.ID)
	httpx.JSON(w, http.StatusCreated, trip)
}

func (m *Module) getTrip(w http.ResponseWriter, r *http.Request) {
	detail, err := m.repo.TripDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		m.storeError(w, "leyendo el viaje", err)
		return
	}
	withNotice(&detail)
	m.withPreview(r.Context(), &detail)
	httpx.JSON(w, http.StatusOK, detail)
}

func (m *Module) updateTrip(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Destination  *string            `json:"destination"`
		DepartAt     *string            `json:"departAt"`
		ReturnAt     *string            `json:"returnAt"`
		Status       *domain.TripStatus `json:"status"`
		SellerUserID *string            `json:"sellerUserId"`
	}
	if !decode(w, r, &body) {
		return
	}
	patch := store.TripPatch{Destination: body.Destination, Status: body.Status, SellerUserID: body.SellerUserID}
	if body.DepartAt != nil {
		date, err := parseDate(*body.DepartAt)
		if err != nil {
			httpx.BadRequest(w, "fecha de salida inválida")
			return
		}
		patch.DepartAt = &date
	}
	if body.ReturnAt != nil {
		date, err := parseDate(*body.ReturnAt)
		if err != nil {
			httpx.BadRequest(w, "fecha de regreso inválida")
			return
		}
		patch.ReturnAt = &date
	}
	trip, err := m.repo.UpdateTrip(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		m.storeError(w, "actualizando el viaje", err)
		return
	}
	m.emit("trip.updated", map[string]any{"tripId": trip.ID})
	m.afterChange(r.Context(), trip.ID)
	httpx.JSON(w, http.StatusOK, trip)
}

func (m *Module) addPassenger(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FirstName         string `json:"firstName"`
		LastName          string `json:"lastName"`
		Kind              string `json:"kind"`
		IsHolder          bool   `json:"isHolder"`
		BirthDate         string `json:"birthDate"`
		ResidenceCountry  string `json:"residenceCountry"`
		DocumentType      string `json:"documentType"`
		DocumentCountry   string `json:"documentCountry"`
		DocumentNumber    string `json:"documentNumber"`
		DocumentExpiresAt string `json:"documentExpiresAt"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Kind == "" {
		body.Kind = "adulto"
	}
	n := store.NewPassenger{
		TripID: r.PathValue("id"), FirstName: strings.TrimSpace(body.FirstName),
		LastName: strings.TrimSpace(body.LastName), Kind: body.Kind, IsHolder: body.IsHolder,
		ResidenceCountry: strings.ToUpper(strings.TrimSpace(body.ResidenceCountry)),
		DocumentType:     body.DocumentType, DocumentCountry: strings.ToUpper(strings.TrimSpace(body.DocumentCountry)),
		DocumentNumber: body.DocumentNumber, CreatedBy: auth.UserID(r.Context()),
	}
	if name, ok := optionalDates(map[string]dateField{
		"nacimiento": {body.BirthDate, &n.BirthDate}, "vencimiento del documento": {body.DocumentExpiresAt, &n.DocumentExpiresAt},
	}); !ok {
		httpx.BadRequest(w, "fecha de "+name+" inválida")
		return
	}
	passenger, err := m.repo.AddPassenger(r.Context(), n)
	if err != nil {
		m.storeError(w, "creando el pasajero", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": passenger.TripID, "passengerId": passenger.ID})
	m.afterChange(r.Context(), passenger.TripID)
	httpx.JSON(w, http.StatusCreated, passenger)
}

func (m *Module) updatePassenger(w http.ResponseWriter, r *http.Request) {
	var patch store.PassengerPatch
	if !decode(w, r, &patch) {
		return
	}
	passenger, err := m.repo.UpdatePassenger(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		m.storeError(w, "actualizando el pasajero", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": passenger.TripID, "passengerId": passenger.ID})
	m.afterChange(r.Context(), passenger.TripID)
	httpx.JSON(w, http.StatusOK, passenger)
}

func (m *Module) deletePassenger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tripID, err := m.repo.DeletePassenger(r.Context(), id)
	if err != nil {
		m.storeError(w, "borrando el pasajero", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": tripID, "passengerId": id})
	m.afterChange(r.Context(), tripID)
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) putPayment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TotalUSD         *float64 `json:"totalUsd"`
		Rate             *float64 `json:"rate"`
		TotalARS         *float64 `json:"totalArs"`
		Method           string   `json:"method"`
		DepositUSD       *float64 `json:"depositUsd"`
		DepositDueAt     string   `json:"depositDueAt"`
		DepositPaid      bool     `json:"depositPaid"`
		NonRefundableUSD *float64 `json:"nonRefundableUsd"`
	}
	if !decode(w, r, &body) {
		return
	}
	payment := store.Payment{TripID: r.PathValue("id"), TotalUSD: body.TotalUSD,
		Rate: body.Rate, TotalARS: body.TotalARS, Method: body.Method,
		DepositUSD: body.DepositUSD, NonRefundableUSD: body.NonRefundableUSD,
		UpdatedBy: auth.UserID(r.Context())}
	if body.DepositDueAt != "" {
		date, err := parseDate(body.DepositDueAt)
		if err != nil {
			httpx.BadRequest(w, "vencimiento de seña inválido")
			return
		}
		payment.DepositDueAt = &date
	}
	if body.DepositPaid {
		paid := nowUTC()
		payment.DepositPaidAt = &paid
	}
	saved, err := m.repo.PutPayment(r.Context(), payment)
	if err != nil {
		m.storeError(w, "guardando el pago", err)
		return
	}
	m.emit("payment.changed", map[string]any{"tripId": payment.TripID})
	httpx.JSON(w, http.StatusOK, saved)
}

func (m *Module) closeControl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MessageID string `json:"messageId"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := r.PathValue("id")
	if err := m.repo.CloseControl(r.Context(), store.CloseControl{
		ControlID: id, UserID: auth.UserID(r.Context()), MessageID: body.MessageID,
	}); err != nil {
		m.storeError(w, "cerrando el control", err)
		return
	}
	m.emit("control.changed", map[string]any{"controlId": id})
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) reopenControl(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := m.repo.ReopenControl(r.Context(), id); err != nil {
		m.storeError(w, "reabriendo el control", err)
		return
	}
	m.emit("control.changed", map[string]any{"controlId": id})
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) getCustomer(w http.ResponseWriter, r *http.Request) {
	customer, err := m.repo.Customer(r.Context(), r.PathValue("id"))
	if err != nil {
		m.storeError(w, "leyendo el cliente", err)
		return
	}
	httpx.JSON(w, http.StatusOK, customer)
}

func (m *Module) updateCustomer(w http.ResponseWriter, r *http.Request) {
	var patch store.CustomerPatch
	if !decode(w, r, &patch) {
		return
	}
	customer, err := m.repo.UpdateCustomer(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		m.storeError(w, "actualizando el cliente", err)
		return
	}
	m.emit("customer.changed", map[string]any{"customerId": customer.ID, "contactIds": customer.ContactIDs})
	httpx.JSON(w, http.StatusOK, customer)
}

func (m *Module) linkContact(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ContactID string `json:"contactId"`
	}
	if !decode(w, r, &body) {
		return
	}
	customerID := r.PathValue("id")
	if err := m.repo.LinkContact(r.Context(), customerID, body.ContactID, auth.UserID(r.Context())); err != nil {
		m.storeError(w, "vinculando el contacto", err)
		return
	}
	m.emit("customer.changed", map[string]any{"customerId": customerID, "contactIds": []string{body.ContactID}})
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) inbox(w http.ResponseWriter, r *http.Request) {
	raw := strings.Split(r.URL.Query().Get("contactos"), ",")
	ids := make([]string, 0, len(raw))
	for _, id := range raw {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > 100 {
		httpx.BadRequest(w, "máximo 100 contactos")
		return
	}
	items, err := m.repo.Inbox(r.Context(), ids)
	if err != nil {
		m.storeError(w, "leyendo la bandeja de viajes", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// sellers alimenta el selector de vendedor. Lo sirve operaciones y no el kernel
// porque /api/users es de administración: listar quién vende no tiene por qué
// exigir rol admin.
func (m *Module) sellers(w http.ResponseWriter, r *http.Request) {
	items, err := m.repo.Sellers(r.Context())
	if err != nil {
		m.storeError(w, "listando vendedores", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) storeError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrControlInexistente):
		httpx.NotFound(w, "no encontrado")
	case errors.Is(err, store.ErrConflict):
		httpx.Error(w, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, store.ErrBadCursor), errors.Is(err, store.ErrInvalid),
		errors.Is(err, store.ErrQuedaPorVerificar), errors.Is(err, store.ErrNoConfirmable):
		httpx.BadRequest(w, err.Error())
	case errors.Is(err, store.ErrEvidenciaRequerida), errors.Is(err, store.ErrEvidenciaAjena),
		errors.Is(err, store.ErrDerivadoNoSeCierraAMano):
		httpx.BadRequest(w, err.Error())
	default:
		httpx.Internal(w, m.log, action, err)
	}
}
