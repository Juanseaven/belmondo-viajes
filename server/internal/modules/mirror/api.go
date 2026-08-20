package mirror

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// Este archivo es la superficie HTTP del módulo. Los handlers no llevan lógica:
// traducen parámetros de la URL a una consulta, y errores del store a códigos.

const (
	inboxPageDefault  = 30
	inboxPageMax      = 100
	threadPageDefault = 50
	threadPageMax     = 200
)

// listLines alimenta el filtro por línea, con el contador de sin-revisar de cada
// una.
func (m *Module) listLines(w http.ResponseWriter, r *http.Request) {
	lines, err := m.read.Lines(r.Context())
	if err != nil {
		httpx.Internal(w, m.log, "listando líneas", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"lines": lines})
}

// listConversations devuelve la bandeja. Sin `instance` es la unificada de toda
// la agencia; con él, la de una línea.
func (m *Module) listConversations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	cursor, err := store.ParseCursor(q.Get("cursor"))
	if err != nil {
		httpx.BadRequest(w, "cursor inválido")
		return
	}

	limit := store.ClampLimit(q.Get("limit"), inboxPageDefault, inboxPageMax)
	rows, err := m.read.Inbox(r.Context(), store.InboxQuery{
		InstanceID: q.Get("instance"),
		Search:     q.Get("q"),
		NeedsReply: q.Get("status") == "unanswered",
		Cursor:     cursor,
		Limit:      limit,
	})
	if err != nil {
		httpx.Internal(w, m.log, "listando la bandeja", err)
		return
	}

	httpx.JSON(w, http.StatusOK,
		store.NewPage(rows, limit, store.InboxItem.Cursor))
}

func (m *Module) getConversation(w http.ResponseWriter, r *http.Request) {
	conv, err := m.read.Conversation(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.NotFound(w, "conversación no encontrada")
		return
	}
	if err != nil {
		httpx.Internal(w, m.log, "leyendo la conversación", err)
		return
	}
	httpx.JSON(w, http.StatusOK, conv)
}

// listMessages devuelve una ventana del hilo.
//
// Dos modos: `before` es el scroll normal hacia atrás; `around` centra la ventana
// en un mensaje, que es lo que necesita un resultado de búsqueda para abrir el
// hilo en el sitio correcto.
func (m *Module) listMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	before, err := store.ParseCursor(q.Get("before"))
	if err != nil {
		httpx.BadRequest(w, "cursor inválido")
		return
	}

	limit := store.ClampLimit(q.Get("limit"), threadPageDefault, threadPageMax)
	msgs, err := m.read.Thread(r.Context(), store.ThreadQuery{
		ConversationID: r.PathValue("id"),
		Before:         before,
		AroundID:       q.Get("around"),
		Limit:          limit,
	})
	if err != nil {
		httpx.Internal(w, m.log, "leyendo el hilo", err)
		return
	}

	// En modo `around` la ventana ya está completa por definición: ofrecer un
	// cursor de continuación sería mentir sobre lo que hay a cada lado.
	if q.Get("around") != "" {
		httpx.JSON(w, http.StatusOK, store.Page[store.ThreadMessage]{Items: msgs})
		return
	}
	httpx.JSON(w, http.StatusOK,
		store.NewPage(msgs, limit, store.ThreadMessage.Cursor))
}

// markRead pone el contador de la conversación a cero y avisa al resto de
// pestañas.
//
// El contador es global (una decisión del spec de arquitectura): si alguien abre
// un chat, deja de figurar como pendiente para todo el equipo. El evento evita
// que las demás pestañas sigan mostrando un badge de algo ya revisado.
func (m *Module) markRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := m.read.MarkRead(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.NotFound(w, "conversación no encontrada")
		return
	}
	if err != nil {
		httpx.Internal(w, m.log, "marcando como leída", err)
		return
	}

	m.broadcast("conversation.read", map[string]any{"conversationId": id})
	w.WriteHeader(http.StatusNoContent)
}

// searchMessages busca dentro del texto conversado.
func (m *Module) searchMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	cursor, err := store.ParseCursor(q.Get("cursor"))
	if err != nil {
		httpx.BadRequest(w, "cursor inválido")
		return
	}

	limit := store.ClampLimit(q.Get("limit"), inboxPageDefault, inboxPageMax)
	hits, err := m.read.SearchMessages(r.Context(), store.SearchQuery{
		Text:       q.Get("q"),
		InstanceID: q.Get("instance"),
		Cursor:     cursor,
		Limit:      limit,
	})

	switch {
	case errors.Is(err, store.ErrSearchTooShort):
		httpx.BadRequest(w, "escribí al menos 3 caracteres para buscar")
		return
	case errors.Is(err, store.ErrSearchTooBroad):
		// 400 y no 500: la consulta es válida, pero demasiado amplia para
		// resolverse. Quien puede arreglarlo es el usuario.
		httpx.Error(w, http.StatusBadRequest, "SEARCH_TOO_BROAD",
			"la búsqueda devuelve demasiados resultados, probá con algo más específico")
		return
	case err != nil:
		httpx.Internal(w, m.log, "buscando mensajes", err)
		return
	}

	httpx.JSON(w, http.StatusOK,
		store.NewPage(hits, limit, store.SearchHit.Cursor))
}

// broadcast emite un evento del módulo por el hub compartido.
func (m *Module) broadcast(evType string, payload map[string]any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		m.log.Error("no se pudo serializar el evento ws", "type", evType, "error", err)
		return
	}
	m.k.Hub.Broadcast(ws.Event{
		Module:  domain.ModuleName,
		Type:    evType,
		Payload: raw,
		At:      time.Now().UTC(),
	})
}
