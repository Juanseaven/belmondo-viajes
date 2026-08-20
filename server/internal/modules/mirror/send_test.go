package mirror

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/evolution"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// --- Dobles -----------------------------------------------------------------

type fakeOutbound struct {
	target    store.SendTarget
	targetErr error
	quote     store.QuoteTarget
	quoteErr  error

	quoteConversationID string
	quoteMessageID      string

	// Simula el índice único: la segunda vez que llega una clave, devuelve la fila
	// existente en vez de crear otra.
	byKey map[string]*store.Outbound

	sent   []string
	failed []string
}

func newFakeOutbound(target store.SendTarget) *fakeOutbound {
	return &fakeOutbound{target: target, byKey: map[string]*store.Outbound{}}
}

func (f *fakeOutbound) Target(context.Context, string) (store.SendTarget, error) {
	return f.target, f.targetErr
}

func (f *fakeOutbound) QuoteTarget(_ context.Context, conversationID, messageID string) (store.QuoteTarget, error) {
	f.quoteConversationID = conversationID
	f.quoteMessageID = messageID
	return f.quote, f.quoteErr
}

func (f *fakeOutbound) Create(_ context.Context, o *store.Outbound) (*store.Outbound, bool, error) {
	if existing, ok := f.byKey[o.IdempotencyKey]; ok {
		return existing, false, nil
	}
	created := *o
	created.ID = "out-" + o.IdempotencyKey
	created.Status = "pending"
	f.byKey[o.IdempotencyKey] = &created
	return &created, true, nil
}

func (f *fakeOutbound) MarkSent(_ context.Context, id, waMessageID string) error {
	f.sent = append(f.sent, id+":"+waMessageID)
	return nil
}

func (f *fakeOutbound) MarkFailed(_ context.Context, id, cause string) error {
	f.failed = append(f.failed, id+":"+cause)
	return nil
}

func (f *fakeOutbound) UserByWAMessageID(context.Context, string, string) (string, bool) {
	return "", false
}

var _ store.OutboundStore = (*fakeOutbound)(nil)

type fakeEvolution struct {
	textCalls  int
	mediaCalls int
	lastMedia  string
	lastType   string
	lastQuote  *evolution.Quote
	err        error
}

func (f *fakeEvolution) SendText(_ context.Context, _, _, _ string, quote *evolution.Quote) (*evolution.SendTextResult, error) {
	f.textCalls++
	f.lastQuote = quote
	if f.err != nil {
		return nil, f.err
	}
	out := &evolution.SendTextResult{}
	out.Key.ID = "wa-generado"
	return out, nil
}

func (f *fakeEvolution) SendMedia(_ context.Context, _, _, mediaURL, mediatype, _, _ string, quote *evolution.Quote) (*evolution.SendTextResult, error) {
	f.mediaCalls++
	f.lastMedia = mediaURL
	f.lastType = mediatype
	f.lastQuote = quote
	if f.err != nil {
		return nil, f.err
	}
	out := &evolution.SendTextResult{}
	out.Key.ID = "wa-generado"
	return out, nil
}

type fakePanel struct{ marks map[string]string }

func (f *fakePanel) Mark(_ context.Context, waMessageID, userID string, _ time.Duration) error {
	f.marks[waMessageID] = userID
	return nil
}

func (f *fakePanel) Claim(context.Context, string) (string, bool) { return "", false }

type fakeBlobs struct {
	put map[string][]byte
	err error
}

func (f *fakeBlobs) Put(_ context.Context, key, _ string, data []byte) error {
	if f.err != nil {
		return f.err
	}
	f.put[key] = data
	return nil
}

func (f *fakeBlobs) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://firmada.example/" + key, nil
}

func (*fakeBlobs) DeletePrefix(context.Context, string) error { return nil }

// --- Andamiaje --------------------------------------------------------------

func lineaConectada() store.SendTarget {
	return store.SendTarget{
		ConversationID: "conv-1",
		InstanceID:     "inst-1",
		InstanceName:   "ana",
		RemoteJID:      "573001112233@s.whatsapp.net",
		LineStatus:     "open",
		LineActive:     true,
	}
}

func newSendService(out *fakeOutbound, evo *fakeEvolution, panel *fakePanel, blobs *fakeBlobs) *SendService {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSendService(out, panel, evo, blobs, time.Minute, log)
}

func nuevoEnvio(key, body string) SendRequest {
	return SendRequest{
		ConversationID: "conv-1",
		UserID:         "user-1",
		IdempotencyKey: key,
		Body:           body,
	}
}

// --- Tests ------------------------------------------------------------------

// La razón de ser de la clave de idempotencia: un doble clic no puede mandar el
// mensaje dos veces.
func TestEnviarDosVecesConLaMismaClaveMandaUnaSola(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	evo := &fakeEvolution{}
	svc := newSendService(out, evo, &fakePanel{marks: map[string]string{}}, &fakeBlobs{put: map[string][]byte{}})

	primero, err := svc.Send(context.Background(), nuevoEnvio("clave-1", "hola"))
	if err != nil {
		t.Fatalf("primer envío: %v", err)
	}
	segundo, err := svc.Send(context.Background(), nuevoEnvio("clave-1", "hola"))
	if err != nil {
		t.Fatalf("segundo envío: %v", err)
	}

	if evo.textCalls != 1 {
		t.Fatalf("se llamó %d veces a Evolution, se esperaba 1", evo.textCalls)
	}
	if primero.ID != segundo.ID {
		t.Fatalf("el reintento devolvió otro envío (%s vs %s)", primero.ID, segundo.ID)
	}
}

// El navegador manda nuestro UUID, no un id de WhatsApp libre. Resolverlo junto
// con la conversación evita que alguien cite por accidente (o a propósito) un
// mensaje de otro chat.
func TestEnvioCitadoPasaUnaCitaValidadaAEvolution(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	out.quote = store.QuoteTarget{
		WAMessageID: "wa-original",
		Participant: "573001112233@s.whatsapp.net",
	}
	evo := &fakeEvolution{}
	svc := newSendService(out, evo, &fakePanel{marks: map[string]string{}}, &fakeBlobs{put: map[string][]byte{}})
	req := nuevoEnvio("clave-citada", "te respondo esto")
	req.ReplyToMessageID = "msg-interno-1"

	if _, err := svc.Send(context.Background(), req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if out.quoteConversationID != "conv-1" || out.quoteMessageID != "msg-interno-1" {
		t.Fatalf("la cita se resolvió con conversación=%q mensaje=%q",
			out.quoteConversationID, out.quoteMessageID)
	}
	if evo.lastQuote == nil || evo.lastQuote.MessageID != "wa-original" ||
		evo.lastQuote.Participant != "573001112233@s.whatsapp.net" {
		t.Fatalf("cita enviada a Evolution = %+v", evo.lastQuote)
	}
}

func TestCitaAjenaNoRegistraNiEnviaElMensaje(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	out.quoteErr = store.ErrNotFound
	evo := &fakeEvolution{}
	svc := newSendService(out, evo, &fakePanel{marks: map[string]string{}}, &fakeBlobs{put: map[string][]byte{}})
	req := nuevoEnvio("clave-citada", "te respondo esto")
	req.ReplyToMessageID = "msg-de-otro-chat"

	_, err := svc.Send(context.Background(), req)

	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v, se esperaba store.ErrNotFound", err)
	}
	if evo.textCalls != 0 || evo.mediaCalls != 0 {
		t.Fatal("se llamó a Evolution con una cita ajena")
	}
	if len(out.byKey) != 0 {
		t.Fatal("se registró el envío pese a que la cita era inválida")
	}
}

// La marca en Redis es lo que hace que el eco del webhook se atribuya a su autor
// en vez de parecer escrito desde el teléfono.
func TestEnvioMarcaElAutorAntesDeDevolver(t *testing.T) {
	panel := &fakePanel{marks: map[string]string{}}
	svc := newSendService(newFakeOutbound(lineaConectada()), &fakeEvolution{},
		panel, &fakeBlobs{put: map[string][]byte{}})

	if _, err := svc.Send(context.Background(), nuevoEnvio("clave-1", "hola")); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got := panel.marks["wa-generado"]; got != "user-1" {
		t.Fatalf("marca = %q, se esperaba user-1: el eco se atribuiría al teléfono", got)
	}
}

// Una línea caída no se puede arreglar reintentando: hay que reescanear el QR. El
// error tiene que distinguirse para que el panel lo diga.
func TestNoEnviaPorUnaLineaCaida(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		target store.SendTarget
	}{
		{"desconectada", store.SendTarget{LineStatus: "close", LineActive: true}},
		{"sin vincular", store.SendTarget{LineStatus: "created", LineActive: true}},
		{"desactivada", store.SendTarget{LineStatus: "open", LineActive: false}},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			evo := &fakeEvolution{}
			svc := newSendService(newFakeOutbound(caso.target), evo,
				&fakePanel{marks: map[string]string{}}, &fakeBlobs{put: map[string][]byte{}})

			_, err := svc.Send(context.Background(), nuevoEnvio("clave-1", "hola"))

			if !errors.Is(err, ErrLineOffline) {
				t.Fatalf("error = %v, se esperaba ErrLineOffline", err)
			}
			if evo.textCalls != 0 {
				t.Fatal("se llamó a Evolution con la línea caída")
			}
		})
	}
}

// Si WhatsApp rechaza, el intento tiene que quedar registrado: es lo único que
// deja rastro, porque un envío fallido nunca genera fila en `messages`.
func TestEnvioFallidoQuedaRegistrado(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	evo := &fakeEvolution{err: errors.New("numero inexistente")}
	svc := newSendService(out, evo, &fakePanel{marks: map[string]string{}}, &fakeBlobs{put: map[string][]byte{}})

	_, err := svc.Send(context.Background(), nuevoEnvio("clave-1", "hola"))

	if !errors.Is(err, ErrSendRejected) {
		t.Fatalf("error = %v, se esperaba ErrSendRejected", err)
	}
	if len(out.failed) != 1 {
		t.Fatalf("se registraron %d fallos, se esperaba 1", len(out.failed))
	}
	if !strings.Contains(out.failed[0], "numero inexistente") {
		t.Fatalf("el fallo registrado no conserva la causa: %s", out.failed[0])
	}
}

// El adjunto se sube y a Evolution se le pasa una URL firmada, no el binario ni
// una URL pública: por ahí viajan pasaportes.
func TestAdjuntoSubeYSeMandaFirmado(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	evo := &fakeEvolution{}
	blobs := &fakeBlobs{put: map[string][]byte{}}
	svc := newSendService(out, evo, &fakePanel{marks: map[string]string{}}, blobs)

	req := nuevoEnvio("clave-1", "acá va tu pasaporte")
	req.Attachment = &Attachment{
		FileName: "pasaporte.jpg",
		MimeType: "image/jpeg",
		Data:     []byte("binario"),
	}

	if _, err := svc.Send(context.Background(), req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(blobs.put) != 1 {
		t.Fatalf("se subieron %d archivos, se esperaba 1", len(blobs.put))
	}
	if evo.mediaCalls != 1 || evo.textCalls != 0 {
		t.Fatalf("llamadas: media=%d texto=%d; se esperaba media=1 texto=0",
			evo.mediaCalls, evo.textCalls)
	}
	if !strings.HasPrefix(evo.lastMedia, "https://firmada.example/") {
		t.Fatalf("a Evolution se le pasó %q, se esperaba una URL firmada", evo.lastMedia)
	}
	if evo.lastType != "image" {
		t.Fatalf("mediatype = %q, se esperaba image", evo.lastType)
	}
}

// Si la subida falla, no puede quedar una fila 'pending' que nadie va a resolver.
func TestAdjuntoQueNoSubeNoRegistraEnvio(t *testing.T) {
	out := newFakeOutbound(lineaConectada())
	evo := &fakeEvolution{}
	blobs := &fakeBlobs{put: map[string][]byte{}, err: errors.New("R2 caído")}
	svc := newSendService(out, evo, &fakePanel{marks: map[string]string{}}, blobs)

	req := nuevoEnvio("clave-1", "")
	req.Attachment = &Attachment{FileName: "x.pdf", MimeType: "application/pdf", Data: []byte("x")}

	if _, err := svc.Send(context.Background(), req); err == nil {
		t.Fatal("Send no devolvió error con la subida caída")
	}
	if len(out.byKey) != 0 {
		t.Fatal("quedó un envío registrado pese a que el adjunto no subió")
	}
	if evo.mediaCalls != 0 {
		t.Fatal("se llamó a Evolution sin el archivo subido")
	}
}

func TestClasificacionDeAdjuntos(t *testing.T) {
	cases := map[string]string{
		"image/jpeg":               "image",
		"video/mp4":                "video",
		"audio/ogg; codecs=opus":   "audio",
		"application/pdf":          "document",
		"application/octet-stream": "document",
	}
	for mimeType, want := range cases {
		if got := evolutionMediaType(mediaType(mimeType)); got != want {
			t.Errorf("%s → %q, se esperaba %q", mimeType, got, want)
		}
	}
}

// El nombre en Evolution es UNIQUE, así que dos personas con el mismo nombre no
// pueden derivar la misma instancia.
func TestNombreDeInstanciaEsUnicoYSeguro(t *testing.T) {
	a := instanceName("Ana María Pérez")
	b := instanceName("Ana María Pérez")

	if a == b {
		t.Fatalf("dos altas del mismo nombre dieron %q: la segunda fallaría", a)
	}
	for _, name := range []string{a, b} {
		if strings.ContainsAny(name, " áéíóúñÁÉÍÓÚÑ/?#") {
			t.Fatalf("el nombre %q lleva caracteres que rompen la URL de Evolution", name)
		}
		if !strings.HasPrefix(name, "ana-maria-perez") {
			t.Fatalf("el nombre %q perdió la referencia al dueño", name)
		}
	}
}

func TestNombreDeInstanciaConEntradaRara(t *testing.T) {
	for _, entrada := range []string{"", "   ", "!!!", "日本語"} {
		name := instanceName(entrada)
		if name == "" || strings.HasPrefix(name, "-") {
			t.Fatalf("entrada %q produjo el nombre inválido %q", entrada, name)
		}
	}
}
