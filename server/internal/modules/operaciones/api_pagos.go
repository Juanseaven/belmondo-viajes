package operaciones

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/auth"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Los pagos registrados y los archivos del cliente.

func (m *Module) addPaymentRecord(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Amount   float64  `json:"amount"`
		Currency string   `json:"currency"`
		Rate     *float64 `json:"rate"`
		PaidAt   string   `json:"paidAt"`
		Method   string   `json:"method"`
		Note     string   `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	n := store.NewPaymentRecord{TripID: r.PathValue("id"), Amount: body.Amount,
		Currency: strings.ToUpper(strings.TrimSpace(body.Currency)), Method: body.Method, Note: body.Note,
		CreatedBy: auth.UserID(r.Context()), PaidAt: m.today()}
	if n.Amount <= 0 {
		httpx.BadRequest(w, "el monto tiene que ser mayor que cero")
		return
	}
	switch n.Currency {
	case "USD":
	case "ARS":
		if body.Rate == nil || *body.Rate <= 0 {
			httpx.BadRequest(w, "un pago en pesos necesita el cambio")
			return
		}
		n.Rate = body.Rate
	default:
		httpx.BadRequest(w, "la moneda es USD o ARS")
		return
	}
	if body.PaidAt != "" {
		date, err := parseDate(body.PaidAt)
		if err != nil {
			httpx.BadRequest(w, "fecha del pago inválida")
			return
		}
		n.PaidAt = date
	}
	record, err := m.repo.AddPaymentRecord(r.Context(), n)
	if err != nil {
		m.storeError(w, "registrando el pago", err)
		return
	}
	m.emit("payment.changed", map[string]any{"tripId": record.TripID})
	httpx.JSON(w, http.StatusCreated, record)
}

func (m *Module) deletePaymentRecord(w http.ResponseWriter, r *http.Request) {
	tripID, err := m.repo.DeletePaymentRecord(r.Context(), r.PathValue("id"))
	if err != nil {
		m.storeError(w, "borrando el pago", err)
		return
	}
	m.emit("payment.changed", map[string]any{"tripId": tripID})
	w.WriteHeader(http.StatusNoContent)
}

// customerFiles es una de las dos listas de Archivos: fotos y videos
// (tipo=media, por defecto) o documentos (tipo=documentos).
func (m *Module) customerFiles(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	f := store.FileFilter{Kind: store.FileKind(query.Get("tipo")), Favorites: query.Get("favoritos") == "1", Cursor: query.Get("cursor")}
	switch f.Kind {
	case "":
		f.Kind = store.FilesMedia
	case store.FilesMedia, store.FilesDocuments:
	default:
		httpx.BadRequest(w, "tipo desconocido: "+string(f.Kind))
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
	page, err := m.repo.CustomerFiles(r.Context(), r.PathValue("id"), f)
	if err != nil {
		m.storeError(w, "listando los archivos", err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (m *Module) markFavorite(w http.ResponseWriter, r *http.Request) {
	m.setFavorite(w, r, true)
}

func (m *Module) unmarkFavorite(w http.ResponseWriter, r *http.Request) {
	m.setFavorite(w, r, false)
}

// setFavorite marca o saca la estrella. Es de la agencia: el evento hace que
// el resto la vea al momento.
func (m *Module) setFavorite(w http.ResponseWriter, r *http.Request, on bool) {
	id := r.PathValue("id")
	if err := m.repo.SetFavorite(r.Context(), id, auth.UserID(r.Context()), on); err != nil {
		m.storeError(w, "guardando el favorito", err)
		return
	}
	m.emit("file.favorite", map[string]any{"messageId": id, "favorite": on})
	w.WriteHeader(http.StatusNoContent)
}
