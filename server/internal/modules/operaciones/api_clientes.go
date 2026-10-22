package operaciones

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/auth"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// La pantalla de Clientes: la lista, la página de cada cliente, el alta a mano
// y los vínculos con los chats.

var customerChips = map[string]store.CustomerChip{
	"": store.ChipTodos, "todos": store.ChipTodos, "con-viaje": store.ChipConViaje,
	"sin-viajes": store.ChipSinViajes, "sin-whatsapp": store.ChipSinWhatsApp,
}

func (m *Module) listCustomers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	chip, ok := customerChips[query.Get("filtro")]
	if !ok {
		httpx.BadRequest(w, "filtro desconocido: "+query.Get("filtro"))
		return
	}
	f := store.CustomerFilter{Query: strings.TrimSpace(query.Get("q")), Chip: chip, Cursor: query.Get("cursor")}
	switch orden := store.CustomerSort(query.Get("orden")); orden {
	case "", store.SortMovimiento, store.SortNombre:
		f.Sort = orden
	default:
		httpx.BadRequest(w, "orden desconocido: "+string(orden))
		return
	}
	if limit := query.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > 100 {
			httpx.BadRequest(w, "limit entre 1 y 100")
			return
		}
		f.Limit = n
	}
	page, err := m.repo.ListCustomers(r.Context(), f)
	if err != nil {
		m.storeError(w, "listando clientes", err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

type customerBody struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	PhoneNumber    string `json:"phoneNumber"`
	DocumentType   string `json:"documentType"`
	DocumentNumber string `json:"documentNumber"`
}

func (m *Module) createCustomer(w http.ResponseWriter, r *http.Request) {
	var body customerBody
	if !decode(w, r, &body) {
		return
	}
	n := store.NewCustomer{
		Name: strings.TrimSpace(body.Name), Email: strings.TrimSpace(body.Email),
		PhoneNumber: strings.TrimSpace(body.PhoneNumber), DocumentType: strings.TrimSpace(body.DocumentType),
		DocumentNumber: strings.TrimSpace(body.DocumentNumber), CreatedBy: auth.UserID(r.Context()),
	}
	if n.Name == "" {
		httpx.BadRequest(w, "el nombre es obligatorio")
		return
	}
	customer, err := m.repo.CreateCustomer(r.Context(), n)
	if err != nil {
		m.storeError(w, "creando el cliente", err)
		return
	}
	m.emit("customer.created", map[string]any{"customerId": customer.ID})
	httpx.JSON(w, http.StatusCreated, customer)
}

// similarCustomers es el aviso de posible repetido mientras se carga un
// cliente. No impide crear: avisa.
func (m *Module) similarCustomers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	matches, err := m.repo.SimilarCustomers(r.Context(), store.SimilarQuery{
		Name: query.Get("nombre"), PhoneNumber: query.Get("telefono"),
		Email: query.Get("mail"), DocumentNumber: query.Get("documento"),
	})
	if err != nil {
		m.storeError(w, "buscando clientes parecidos", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": matches})
}

func (m *Module) getCustomer(w http.ResponseWriter, r *http.Request) {
	detail, err := m.repo.CustomerDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		m.storeError(w, "leyendo el cliente", err)
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

// freeContacts busca los chats que se pueden vincular a un cliente: contactos
// sin cliente con una conversación individual. Con cliente, primero los que
// coinciden con su teléfono.
func (m *Module) freeContacts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	items, err := m.repo.FreeContacts(r.Context(), query.Get("q"), query.Get("cliente"))
	if err != nil {
		m.storeError(w, "buscando contactos", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// contactCustomers dice de qué cliente es un chat. Si todavía no es de nadie,
// sugiere los clientes que podrían serlo, por los últimos dígitos del teléfono.
func (m *Module) contactCustomers(w http.ResponseWriter, r *http.Request) {
	contactID := r.PathValue("id")
	customerID, err := m.repo.ContactCustomer(r.Context(), contactID)
	if err != nil {
		m.storeError(w, "buscando el cliente del contacto", err)
		return
	}
	items := []store.CustomerMatch{}
	if customerID == "" {
		if items, err = m.repo.SuggestedCustomers(r.Context(), contactID); err != nil {
			m.storeError(w, "buscando clientes para el contacto", err)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"customerId": customerID, "items": items})
}

func (m *Module) tripCompanions(w http.ResponseWriter, r *http.Request) {
	items, err := m.repo.TripCompanions(r.Context(), r.PathValue("id"))
	if err != nil {
		m.storeError(w, "leyendo los pasajeros para traer", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) copyPassengers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PassengerIDs []string `json:"passengerIds"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.PassengerIDs) == 0 {
		httpx.BadRequest(w, "no hay pasajeros para traer")
		return
	}
	tripID := r.PathValue("id")
	items, err := m.repo.CopyPassengers(r.Context(), tripID, body.PassengerIDs, auth.UserID(r.Context()))
	if err != nil {
		m.storeError(w, "trayendo pasajeros", err)
		return
	}
	m.emit("passenger.changed", map[string]any{"tripId": tripID})
	m.afterChange(r.Context(), tripID)
	httpx.JSON(w, http.StatusCreated, map[string]any{"items": items})
}
