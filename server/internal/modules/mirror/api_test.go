package mirror

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/kernel"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/config"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// fakeRead sustituye a Postgres. Lo que se prueba acá es la traducción de la URL
// a una consulta y de los errores del store a códigos HTTP, no el SQL.
type fakeRead struct {
	inbox     []store.InboxItem
	thread    []store.ThreadMessage
	hits      []store.SearchHit
	conv      store.InboxItem
	convErr   error
	searchErr error
	markErr   error

	// Última consulta recibida, para comprobar qué se le pidió al store.
	lastInbox  store.InboxQuery
	lastThread store.ThreadQuery
	lastSearch store.SearchQuery
	marked     []string
}

func (f *fakeRead) Lines(context.Context) ([]store.Line, error) { return []store.Line{}, nil }

func (f *fakeRead) Inbox(_ context.Context, q store.InboxQuery) ([]store.InboxItem, error) {
	f.lastInbox = q
	return f.inbox, nil
}

func (f *fakeRead) Conversation(context.Context, string) (store.InboxItem, error) {
	return f.conv, f.convErr
}

func (f *fakeRead) Thread(_ context.Context, q store.ThreadQuery) ([]store.ThreadMessage, error) {
	f.lastThread = q
	return f.thread, nil
}

func (f *fakeRead) MarkRead(_ context.Context, id string) error {
	f.marked = append(f.marked, id)
	return f.markErr
}

func (f *fakeRead) SearchMessages(_ context.Context, q store.SearchQuery) ([]store.SearchHit, error) {
	f.lastSearch = q
	return f.hits, f.searchErr
}

var _ store.ReadStore = (*fakeRead)(nil)

func newTestModule(read *fakeRead) *Module {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Module{
		k:    &kernel.Kernel{Cfg: &config.Config{}, Hub: ws.NewHub(log), Log: log},
		read: read,
		log:  log,
	}
}

// do ejecuta un handler y devuelve la respuesta. `pathValues` rellena los
// comodines de la ruta, que normalmente pone el ServeMux.
func do(h http.HandlerFunc, target string, pathValues map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("cuerpo de error ilegible: %s", rec.Body)
	}
	return body.Error.Code
}

// Un cursor manipulado es una petición mal formada del cliente, no un fallo del
// servidor: tiene que ser 400 y no 500.
func TestCursorInvalidoEs400(t *testing.T) {
	m := newTestModule(&fakeRead{})

	// El punto no está en el alfabeto base64url, así que es un token válido en una
	// URL e inválido como cursor. Con un escape porcentual roto (`%%%`) no
	// serviría: url.Query() descarta en silencio lo que no sabe parsear y el
	// handler recibiría un cursor vacío, que es una primera página legítima.
	const roto = "basura.invalida"

	cases := []struct {
		name    string
		handler http.HandlerFunc
		target  string
	}{
		{"bandeja", m.listConversations, "/?cursor=" + roto},
		{"hilo", m.listMessages, "/?before=" + roto},
		{"búsqueda", m.searchMessages, "/?q=reserva&cursor=" + roto},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(c.handler, c.target, map[string]string{"id": "8f14e45f-ceea-467a-9575-1e1e6b46f14e"})

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba 400", rec.Code)
			}
			if got := errCode(t, rec); got != "INVALID_INPUT" {
				t.Fatalf("code = %q, se esperaba INVALID_INPUT", got)
			}
		})
	}
}

func TestBusquedaCortaEs400(t *testing.T) {
	m := newTestModule(&fakeRead{searchErr: store.ErrSearchTooShort})

	rec := do(m.searchMessages, "/?q=ab", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400", rec.Code)
	}
}

// Una búsqueda que agota su tiempo es 400 y no 500: la consulta es válida, pero
// demasiado amplia, y quien puede arreglarlo es el usuario.
func TestBusquedaDemasiadoAmpliaEs400ConSuCodigo(t *testing.T) {
	m := newTestModule(&fakeRead{searchErr: store.ErrSearchTooBroad})

	rec := do(m.searchMessages, "/?q=hola", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400", rec.Code)
	}
	if got := errCode(t, rec); got != "SEARCH_TOO_BROAD" {
		t.Fatalf("code = %q, se esperaba SEARCH_TOO_BROAD", got)
	}
}

func TestConversacionInexistenteEs404(t *testing.T) {
	m := newTestModule(&fakeRead{convErr: store.ErrNotFound})

	rec := do(m.getConversation, "/", map[string]string{"id": "no-es-un-uuid"})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, se esperaba 404", rec.Code)
	}
}

// El frontend llama a esto cada vez que se abre un hilo, así que repetirlo no
// puede ser un error.
func TestMarkReadEsIdempotente(t *testing.T) {
	read := &fakeRead{}
	m := newTestModule(read)
	const id = "8f14e45f-ceea-467a-9575-1e1e6b46f14e"

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()

		m.markRead(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("llamada %d: status = %d, se esperaba 204", i+1, rec.Code)
		}
	}
	if len(read.marked) != 3 {
		t.Fatalf("el store recibió %d llamadas, se esperaban 3", len(read.marked))
	}
}

// El techo de `limit` es lo que impide convertir un listado en una denegación de
// servicio de una sola petición.
func TestLimitSeAcota(t *testing.T) {
	read := &fakeRead{}
	m := newTestModule(read)

	do(m.listConversations, "/?limit=100000", nil)
	if read.lastInbox.Limit != inboxPageMax {
		t.Fatalf("límite de bandeja = %d, se esperaba %d", read.lastInbox.Limit, inboxPageMax)
	}

	do(m.listMessages, "/?limit=100000", map[string]string{"id": "x"})
	if read.lastThread.Limit != threadPageMax {
		t.Fatalf("límite de hilo = %d, se esperaba %d", read.lastThread.Limit, threadPageMax)
	}
}

func TestFiltrosLleganAlStore(t *testing.T) {
	read := &fakeRead{}
	m := newTestModule(read)

	do(m.listConversations, "/?instance=linea-1&q=ana", nil)

	if read.lastInbox.InstanceID != "linea-1" {
		t.Fatalf("instance = %q, se esperaba linea-1", read.lastInbox.InstanceID)
	}
	if read.lastInbox.Search != "ana" {
		t.Fatalf("search = %q, se esperaba ana", read.lastInbox.Search)
	}
}

func TestFiltroSinResponderLlegaAlStore(t *testing.T) {
	read := &fakeRead{}
	m := newTestModule(read)

	do(m.listConversations, "/?status=unanswered", nil)

	if !read.lastInbox.NeedsReply {
		t.Fatal("NeedsReply = false, se esperaba true")
	}
}

func TestSendMessageJSONAceptaReplyToMessageID(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	out.quote = store.QuoteTarget{WAMessageID: "wa-original", Participant: "5491111111111@s.whatsapp.net"}
	evo := &fakeEvolution{}
	m := newTestModule(&fakeRead{})
	m.send = newSendService(out, evo, &fakePanel{marks: map[string]string{}}, &fakeBlobs{put: map[string][]byte{}})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(
		`{"body":"respuesta","idempotencyKey":"key-json","replyToMessageId":"msg-interno"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	m.sendMessage(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, cuerpo = %s", rec.Code, rec.Body.String())
	}
	if out.quoteMessageID != "msg-interno" || evo.lastQuote == nil || evo.lastQuote.MessageID != "wa-original" {
		t.Fatalf("mensaje resuelto = %q; cita enviada = %+v", out.quoteMessageID, evo.lastQuote)
	}
}

func TestSendMessageMultipartAceptaReplyToMessageID(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	out.quote = store.QuoteTarget{WAMessageID: "wa-original", Participant: "5491111111111@s.whatsapp.net"}
	evo := &fakeEvolution{}
	m := newTestModule(&fakeRead{})
	m.send = newSendService(out, evo, &fakePanel{marks: map[string]string{}}, &fakeBlobs{put: map[string][]byte{}})

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("body", "respuesta con archivo")
	_ = form.WriteField("idempotencyKey", "key-media")
	_ = form.WriteField("replyToMessageId", "msg-interno")
	file, err := form.CreateFormFile("file", "reserva.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = file.Write([]byte("pdf"))
	_ = form.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	m.sendMessage(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, cuerpo = %s", rec.Code, rec.Body.String())
	}
	if evo.mediaCalls != 1 || evo.lastQuote == nil || evo.lastQuote.MessageID != "wa-original" {
		t.Fatalf("mediaCalls = %d; cita enviada = %+v", evo.mediaCalls, evo.lastQuote)
	}
}

func TestSendMessageDevuelve404SiLaCitaNoPerteneceALaConversacion(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	out.quoteErr = store.ErrNotFound
	evo := &fakeEvolution{}
	m := newTestModule(&fakeRead{})
	m.send = newSendService(out, evo, &fakePanel{marks: map[string]string{}}, &fakeBlobs{put: map[string][]byte{}})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(
		`{"body":"respuesta","idempotencyKey":"key-json","replyToMessageId":"msg-ajeno"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	m.sendMessage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, cuerpo = %s", rec.Code, rec.Body.String())
	}
	if evo.textCalls != 0 || evo.mediaCalls != 0 {
		t.Fatal("se despachó un mensaje con una cita ajena")
	}
}

// En modo `around` la ventana ya está completa: ofrecer cursor de continuación
// sería mentir sobre lo que hay a cada lado del mensaje.
func TestModoAroundNoDevuelveCursor(t *testing.T) {
	read := &fakeRead{thread: make([]store.ThreadMessage, 300)}
	m := newTestModule(read)

	rec := do(m.listMessages, "/?around=msg-1", map[string]string{"id": "conv-1"})

	var page store.Page[store.ThreadMessage]
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if page.NextCursor != "" {
		t.Fatalf("NextCursor = %q, se esperaba vacío en modo around", page.NextCursor)
	}
	if read.lastThread.AroundID != "msg-1" {
		t.Fatalf("AroundID = %q, se esperaba msg-1", read.lastThread.AroundID)
	}
}

// Una lista vacía tiene que serializarse como [] y no como null: un null obliga
// al frontend a defenderse en cada punto donde recorre la lista.
func TestListaVaciaSerializaComoArray(t *testing.T) {
	m := newTestModule(&fakeRead{inbox: []store.InboxItem{}})

	rec := do(m.listConversations, "/", nil)

	if body := rec.Body.String(); !contains(body, `"items":[]`) {
		t.Fatalf("cuerpo = %s, se esperaba items como array vacío", body)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && stringIndex(haystack, needle) >= 0
}

func stringIndex(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
