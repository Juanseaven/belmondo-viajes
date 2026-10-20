package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/db"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/db/dbtest"
	kernelmigrations "github.com/juanseaven/viajes-crm/server/internal/kernel/migrations"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// Integración de las consultas de lectura contra Postgres real.
//
// Estas consultas no se pueden probar con dobles: lo que se verifica es
// justamente que el SQL se apoye en los índices correctos, que el keyset no
// duplique ni se salte filas, y que el esquema real aguante lo que las consultas
// asumen.
//
//	createdb viajes_crm_test
//	TEST_DATABASE_URL='postgres://postgres:clave@127.0.0.1:5432/viajes_crm_test?sslmode=disable' \
//	    go test ./internal/modules/mirror/store/postgres/
//
// Sin la variable se saltan: `go test ./...` no puede exigir una base levantada.

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestMain le da a este paquete su propia base: `go test ./...` corre los
// paquetes en paralelo y compartir base con otro hace que ambos se pisen al
// limpiar el esquema.
func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m, "mirror_postgres"))
}

// testDB deja la base con el esquema REAL aplicado: las mismas migraciones que
// corren en producción, no un esquema de mentira escrito para el test. Si una
// migración y una consulta se desincronizan, se ve acá.
func testDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	ctx, pool := dbtest.Pool(t)

	if err := db.Migrate(ctx, pool, "kernel", kernelmigrations.FS(), quietLogger()); err != nil {
		t.Fatalf("migraciones del kernel: %v", err)
	}
	// Las migraciones del espejo se leen del directorio y no vía mirror.Module:
	// este paquete es el que ese módulo importa, así que depender de él acá sería
	// un ciclo. Los archivos son los mismos que embebe el módulo.
	if err := db.Migrate(ctx, pool, domain.ModuleName,
		os.DirFS("../../migrations"), quietLogger()); err != nil {
		t.Fatalf("migraciones del espejo: %v", err)
	}
	return ctx, pool
}

// --- Semillas ---------------------------------------------------------------

func seedLine(t *testing.T, ctx context.Context, p *pgxpool.Pool, name, label string) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO instances (evolution_name, owner_label, status)
		 VALUES ($1, $2, 'open') RETURNING id`, name, label).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando línea %s: %v", name, err)
	}
	return id
}

func seedContact(t *testing.T, ctx context.Context, p *pgxpool.Pool, jid, name, phone string) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO contacts (wa_jid, push_name, phone_number)
		 VALUES ($1, $2, $3) RETURNING id`, jid, name, phone).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando contacto %s: %v", jid, err)
	}
	return id
}

func seedConversation(t *testing.T, ctx context.Context, p *pgxpool.Pool,
	instanceID, contactID string, at time.Time, unread int) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO conversations
		     (instance_id, contact_id, remote_jid, last_message_at,
		      last_message_preview, last_message_type, unread_count)
		 VALUES ($1, $2, 'jid', $3, 'preview', 'text', $4) RETURNING id`,
		instanceID, contactID, at, unread).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando conversación: %v", err)
	}
	return id
}

func seedMessage(t *testing.T, ctx context.Context, p *pgxpool.Pool,
	convID, instanceID, contactID, waID, body string, at time.Time) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO messages
		     (conversation_id, instance_id, contact_id, wa_message_id,
		      direction, origin, type, body, wa_timestamp)
		 VALUES ($1, $2, $3, $4, 'inbound', 'contact', 'text', $5, $6) RETURNING id`,
		convID, instanceID, contactID, waID, body, at).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando mensaje %s: %v", waID, err)
	}
	return id
}

// La combinación conversación + UUID interno es la frontera de seguridad de
// las respuestas citadas. Un UUID válido de otro chat no debe alcanzar nunca a
// Evolution.
func TestQuoteTargetResuelveSoloMensajesDeLaMismaConversacion(t *testing.T) {
	ctx, p := testDB(t)
	line := seedLine(t, ctx, p, "ana", "Ana")
	if _, err := p.Exec(ctx,
		`UPDATE instances SET phone_jid = '5491100000000@s.whatsapp.net' WHERE id = $1`, line); err != nil {
		t.Fatalf("preparando jid de la línea: %v", err)
	}
	contactA := seedContact(t, ctx, p, "5491111111111@s.whatsapp.net", "A", "+5491111111111")
	contactB := seedContact(t, ctx, p, "5491222222222@s.whatsapp.net", "B", "+5491222222222")
	convA := seedConversation(t, ctx, p, line, contactA, time.Now().UTC(), 0)
	convB := seedConversation(t, ctx, p, line, contactB, time.Now().UTC(), 0)
	messageA := seedMessage(t, ctx, p, convA, line, contactA, "wa-a", "mensaje A", time.Now().UTC())
	messageB := seedMessage(t, ctx, p, convB, line, contactB, "wa-b", "mensaje B", time.Now().UTC())
	if _, err := p.Exec(ctx,
		`UPDATE messages SET sender_jid = '5491111111111@s.whatsapp.net' WHERE id = $1`, messageA); err != nil {
		t.Fatalf("preparando remitente: %v", err)
	}

	got, err := NewOutbound(p).QuoteTarget(ctx, convA, messageA)
	if err != nil {
		t.Fatalf("QuoteTarget: %v", err)
	}
	if got.WAMessageID != "wa-a" || got.Participant != "5491111111111@s.whatsapp.net" {
		t.Fatalf("cita = %+v", got)
	}

	if _, err := p.Exec(ctx,
		`UPDATE messages SET direction = 'outbound', origin = 'device', sender_jid = NULL WHERE id = $1`, messageA); err != nil {
		t.Fatalf("preparando mensaje saliente: %v", err)
	}
	got, err = NewOutbound(p).QuoteTarget(ctx, convA, messageA)
	if err != nil {
		t.Fatalf("QuoteTarget saliente: %v", err)
	}
	if got.WAMessageID != "wa-a" || got.Participant != "5491100000000@s.whatsapp.net" {
		t.Fatalf("cita saliente = %+v", got)
	}

	if _, err := NewOutbound(p).QuoteTarget(ctx, convA, messageB); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("mensaje de otro chat: error = %v, se esperaba ErrNotFound", err)
	}
}

func TestThreadResuelveLaVistaDelMensajeCitado(t *testing.T) {
	ctx, p := testDB(t)
	line := seedLine(t, ctx, p, "ana", "Ana")
	contact := seedContact(t, ctx, p, "5491111111111@s.whatsapp.net", "Cliente", "+5491111111111")
	conv := seedConversation(t, ctx, p, line, contact, time.Now().UTC(), 0)
	base := time.Now().UTC().Add(-time.Minute)
	originalID := seedMessage(t, ctx, p, conv, line, contact, "wa-original", "mensaje original", base)
	replyID := seedMessage(t, ctx, p, conv, line, contact, "wa-respuesta", "respuesta", base.Add(time.Second))
	if _, err := p.Exec(ctx,
		`UPDATE messages SET sender_name = 'Ana cliente' WHERE id = $1`, originalID); err != nil {
		t.Fatalf("preparando autor: %v", err)
	}
	if _, err := p.Exec(ctx,
		`UPDATE messages SET quoted_wa_message_id = 'wa-original' WHERE id = $1`, replyID); err != nil {
		t.Fatalf("preparando cita: %v", err)
	}

	messages, err := NewRead(p).Thread(ctx, store.ThreadQuery{ConversationID: conv, Limit: 10})
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	var reply store.ThreadMessage
	for _, message := range messages {
		if message.ID == replyID {
			reply = message
		}
	}
	if reply.Quote == nil {
		t.Fatal("la respuesta no incluyó la vista de la cita")
	}
	if reply.Quote.ID != originalID || reply.Quote.Body != "mensaje original" ||
		reply.Quote.Type != "text" || reply.Quote.Direction != "inbound" ||
		reply.Quote.SenderName != "Ana cliente" {
		t.Fatalf("vista de cita = %+v", reply.Quote)
	}
}

func TestThreadConCitaNoDisponibleConservaElMensaje(t *testing.T) {
	ctx, p := testDB(t)
	line := seedLine(t, ctx, p, "ana", "Ana")
	contact := seedContact(t, ctx, p, "5491111111111@s.whatsapp.net", "Cliente", "+5491111111111")
	conv := seedConversation(t, ctx, p, line, contact, time.Now().UTC(), 0)
	replyID := seedMessage(t, ctx, p, conv, line, contact, "wa-respuesta", "respuesta", time.Now().UTC())
	if _, err := p.Exec(ctx,
		`UPDATE messages SET quoted_wa_message_id = 'wa-que-no-se-sincronizo' WHERE id = $1`, replyID); err != nil {
		t.Fatalf("preparando cita ausente: %v", err)
	}

	messages, err := NewRead(p).Thread(ctx, store.ThreadQuery{ConversationID: conv, Limit: 10})
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != replyID {
		t.Fatalf("mensajes = %+v", messages)
	}
	if messages[0].QuotedID != "wa-que-no-se-sincronizo" || messages[0].Quote != nil {
		t.Fatalf("cita ausente = quotedId %q, quote %+v", messages[0].QuotedID, messages[0].Quote)
	}
}

// --- Bandeja ----------------------------------------------------------------

// EL test que justifica el keyset.
//
// La bandeja se reordena sola: cada mensaje que entra sube una conversación al
// principio. Con OFFSET, pedir la página 2 después de eso devuelve filas que ya
// venían en la página 1. Con keyset la posición está anclada a una fila concreta,
// así que lo que se mueve por arriba del cursor no reaparece.
func TestBandejaKeysetNoDuplicaCuandoEntraUnMensajeAMitadDelScroll(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "ana", "Ana")
	base := time.Now().UTC().Truncate(time.Second)

	// 9 conversaciones, de la más reciente (i=0) a la más antigua.
	ids := make([]string, 9)
	for i := range ids {
		contact := seedContact(t, ctx, p,
			fmt.Sprintf("c%d@s.whatsapp.net", i), fmt.Sprintf("Contacto %d", i), "")
		ids[i] = seedConversation(t, ctx, p, line, contact,
			base.Add(-time.Duration(i)*time.Minute), 0)
	}

	const limit = 3
	page1, err := read.Inbox(ctx, store.InboxQuery{Limit: limit})
	if err != nil {
		t.Fatalf("página 1: %v", err)
	}
	if len(page1) != limit+1 { // limit+1 = la fila sonda
		t.Fatalf("página 1 devolvió %d filas, se esperaban %d", len(page1), limit+1)
	}
	page1 = page1[:limit]
	cursor := page1[limit-1].Cursor()

	// Entra un mensaje en la conversación MÁS ANTIGUA: salta al principio de la
	// bandeja, cruzando por encima del cursor. Es el caso que rompe OFFSET.
	if _, err := p.Exec(ctx,
		`UPDATE conversations SET last_message_at = $2 WHERE id = $1`,
		ids[8], base.Add(time.Minute)); err != nil {
		t.Fatalf("moviendo la conversación: %v", err)
	}

	page2, err := read.Inbox(ctx, store.InboxQuery{Cursor: cursor, Limit: limit})
	if err != nil {
		t.Fatalf("página 2: %v", err)
	}
	if len(page2) > limit {
		page2 = page2[:limit]
	}

	seen := map[string]int{}
	for _, it := range append(append([]store.InboxItem{}, page1...), page2...) {
		seen[it.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("la conversación %s apareció %d veces entre las dos páginas", id, n)
		}
	}

	// Y lo que no se movió tiene que seguir apareciendo en orden, sin huecos: la
	// página 2 son las conversaciones 3, 4 y 5.
	for i, it := range page2 {
		if want := ids[3+i]; it.ID != want {
			t.Fatalf("página 2, posición %d = %s, se esperaba %s (hay un hueco)", i, it.ID, want)
		}
	}
}

func TestBandejaFiltraPorLinea(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	ana := seedLine(t, ctx, p, "ana", "Ana")
	pedro := seedLine(t, ctx, p, "pedro", "Pedro")
	now := time.Now().UTC()

	c1 := seedContact(t, ctx, p, "c1@s.whatsapp.net", "Uno", "")
	c2 := seedContact(t, ctx, p, "c2@s.whatsapp.net", "Dos", "")
	deAna := seedConversation(t, ctx, p, ana, c1, now, 0)
	seedConversation(t, ctx, p, pedro, c2, now, 0)

	got, err := read.Inbox(ctx, store.InboxQuery{InstanceID: ana, Limit: 10})
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(got) != 1 || got[0].ID != deAna {
		t.Fatalf("filtrando por Ana se obtuvieron %d filas, se esperaba solo la suya", len(got))
	}
	if got[0].InstanceLbl != "Ana" {
		t.Fatalf("etiqueta de línea = %q, se esperaba Ana", got[0].InstanceLbl)
	}
}

func TestBandejaFiltraConversacionesConMensajesNoLeidos(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "ana", "Ana")
	now := time.Now().UTC()
	unreadContact := seedContact(t, ctx, p, "unread@s.whatsapp.net", "No leído", "")
	readContact := seedContact(t, ctx, p, "read@s.whatsapp.net", "Leído", "")
	unread := seedConversation(t, ctx, p, line, unreadContact, now, 3)
	readConversation := seedConversation(t, ctx, p, line, readContact, now.Add(-time.Minute), 0)

	if _, err := p.Exec(ctx,
		`UPDATE conversations
		    SET last_message_direction = CASE id WHEN $1 THEN 'inbound'::message_direction ELSE 'outbound'::message_direction END
		  WHERE id IN ($1, $2)`, readConversation, unread); err != nil {
		t.Fatalf("preparando direcciones: %v", err)
	}

	got, err := read.Inbox(ctx, store.InboxQuery{UnreadOnly: true, Limit: 10})
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(got) != 1 || got[0].ID != unread {
		t.Fatalf("no leídos devolvió %#v, se esperaba solo %s", got, unread)
	}
}

func TestMensajeViejoNoCambiaLaDireccionDelUltimoMensaje(t *testing.T) {
	ctx, p := testDB(t)
	messages := NewMessages(p)

	seedLine(t, ctx, p, "ana", "Ana")
	latest := time.Now().UTC().Truncate(time.Second)
	base := domain.Message{
		InstanceName: "ana",
		RemoteJID:    "5491100000000@s.whatsapp.net",
		PushName:     "Cliente",
		Type:         domain.TypeText,
		Status:       domain.StatusSent,
	}

	outbound := base
	outbound.WAMessageID = "latest-outbound"
	outbound.Direction = domain.Outbound
	outbound.Origin = domain.OriginDevice
	outbound.Timestamp = latest
	if _, err := messages.Save(ctx, &outbound, ""); err != nil {
		t.Fatalf("guardando saliente reciente: %v", err)
	}

	inbound := base
	inbound.WAMessageID = "older-inbound"
	inbound.Direction = domain.Inbound
	inbound.Origin = domain.OriginContact
	inbound.Timestamp = latest.Add(-time.Hour)
	if _, err := messages.Save(ctx, &inbound, ""); err != nil {
		t.Fatalf("guardando entrante viejo: %v", err)
	}

	rows, err := NewRead(p).Inbox(ctx, store.InboxQuery{Limit: 10})
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(rows) != 1 || rows[0].NeedsReply {
		t.Fatalf("el mensaje viejo cambió la dirección del último mensaje: %#v", rows)
	}
}

// Un id de línea que no es un uuid no puede reventar la consulta con un error de
// sintaxis de Postgres.
func TestBandejaNoMuestraConversacionesDeLineasInactivas(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "linea-inactiva", "Línea inactiva")
	contact := seedContact(t, ctx, p, "inactivo@s.whatsapp.net", "Contacto viejo", "")
	seedConversation(t, ctx, p, line, contact, time.Now().UTC(), 0)

	if _, err := p.Exec(ctx, `UPDATE instances SET is_active = false WHERE id = $1`, line); err != nil {
		t.Fatalf("desactivando línea: %v", err)
	}

	got, err := read.Inbox(ctx, store.InboxQuery{Limit: 10})
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("se esperaban 0 conversaciones de líneas activas, se obtuvieron %d", len(got))
	}
}

func TestBandejaConLineaInvalidaDevuelveVacio(t *testing.T) {
	ctx, p := testDB(t)

	got, err := NewRead(p).Inbox(ctx, store.InboxQuery{InstanceID: "no-es-uuid", Limit: 10})
	if err != nil {
		t.Fatalf("Inbox con id inválido devolvió error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("se esperaban 0 filas, se obtuvieron %d", len(got))
	}
}

func TestBandejaBuscaContactoPorNombreYTelefono(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "ana", "Ana")
	now := time.Now().UTC()

	ana := seedContact(t, ctx, p, "c1@s.whatsapp.net", "Ana María", "573001112233")
	bob := seedContact(t, ctx, p, "c2@s.whatsapp.net", "Roberto", "573009998877")
	convAna := seedConversation(t, ctx, p, line, ana, now, 0)
	seedConversation(t, ctx, p, line, bob, now.Add(-time.Minute), 0)

	for _, term := range []string{"María", "maría", "300111"} {
		got, err := read.Inbox(ctx, store.InboxQuery{Search: term, Limit: 10})
		if err != nil {
			t.Fatalf("buscando %q: %v", term, err)
		}
		if len(got) != 1 || got[0].ID != convAna {
			t.Fatalf("buscando %q se obtuvieron %d filas, se esperaba solo la de Ana María", term, len(got))
		}
	}
}

func TestBandejaYGrupoPrefierenElNombreAgendadoPorLaLinea(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "juanse", "Juanse")
	now := time.Now().UTC()
	person := seedContact(t, ctx, p, "5491126437432@s.whatsapp.net", "BETOAVEN", "+5491126437432")
	private := seedConversation(t, ctx, p, line, person, now, 0)

	if _, err := p.Exec(ctx, `
		INSERT INTO contact_directory
		       (instance_id, wa_jid, full_name, push_name)
		VALUES ($1, '5491126437432@s.whatsapp.net', 'Beto', 'BETOAVEN')`, line); err != nil {
		t.Fatalf("sembrando agenda por línea: %v", err)
	}

	items, err := read.Inbox(ctx, store.InboxQuery{Limit: 10})
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	var got string
	for _, item := range items {
		if item.ID == private {
			got = item.ContactName
		}
	}
	if got != "Beto" {
		t.Fatalf("private contact name = %q, want saved address-book name Beto", got)
	}
}

func TestHiloGrupalResuelveElAutorConLaAgendaDeLaLinea(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "juanse", "Juanse")
	group := seedContact(t, ctx, p, "120363321554617552@g.us", "Administrativas aven", "")
	now := time.Now().UTC()
	conv := seedConversation(t, ctx, p, line, group, now, 0)
	if _, err := p.Exec(ctx, `UPDATE conversations SET remote_jid=$2, is_group=true WHERE id=$1`, conv, "120363321554617552@g.us"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `
		INSERT INTO contact_directory
		       (instance_id, wa_jid, lid_jid, full_name, push_name)
		VALUES ($1, '5491126437432@s.whatsapp.net', '94132898906287@lid', 'Beto', 'BETOAVEN')`, line); err != nil {
		t.Fatalf("sembrando agenda por línea: %v", err)
	}
	message := seedMessage(t, ctx, p, conv, line, group, "wa-group", "Hola", now)
	if _, err := p.Exec(ctx, `UPDATE messages SET sender_jid='94132898906287@lid', sender_name='BETOAVEN' WHERE id=$1`, message); err != nil {
		t.Fatal(err)
	}

	messages, err := read.Thread(ctx, store.ThreadQuery{ConversationID: conv, Limit: 10})
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	if len(messages) != 1 || messages[0].SenderName != "Beto" || messages[0].SenderPhone != "+5491126437432" {
		t.Fatalf("group message author = %+v, want Beto / +5491126437432", messages)
	}
}

func TestGuardarMensajeGrupalDevuelveElAutorResueltoParaTiempoReal(t *testing.T) {
	ctx, p := testDB(t)
	line := seedLine(t, ctx, p, "juanse", "Juanse")
	if _, err := p.Exec(ctx, `
		INSERT INTO contact_directory
		       (instance_id, wa_jid, full_name, push_name)
		VALUES ($1, '5491126437432@s.whatsapp.net', 'Beto', 'BETOAVEN')`, line); err != nil {
		t.Fatal(err)
	}

	saved, err := NewMessages(p).Save(ctx, &domain.Message{
		InstanceName: "juanse", RemoteJID: "120363321554617552@g.us",
		WAMessageID: "GROUP-REALTIME", SenderJID: "5491126437432@s.whatsapp.net",
		SenderName: "BETOAVEN", Direction: domain.Inbound, Origin: domain.OriginContact,
		Type: domain.TypeText, Status: domain.StatusSent, Body: "Hola",
		Timestamp: time.Now().UTC(), IsGroup: true,
	}, "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.SenderName != "Beto" || saved.SenderPhone != "+5491126437432" {
		t.Fatalf("saved realtime author = %q/%q, want Beto/+5491126437432", saved.SenderName, saved.SenderPhone)
	}
}

// --- Líneas -----------------------------------------------------------------

func TestGuardarMensajeIndividualPorLIDReutilizaLaConversacionTelefonica(t *testing.T) {
	ctx, p := testDB(t)
	seedLine(t, ctx, p, "juanse", "Juanse")

	phoneJID := "5491128709473@s.whatsapp.net"
	lidJID := "37035637244007@lid"
	contactID := seedContact(t, ctx, p, phoneJID, "Vero", "+5491128709473")
	if _, err := p.Exec(ctx, `UPDATE contacts SET lid_jid=$2 WHERE id=$1`, contactID, lidJID); err != nil {
		t.Fatal(err)
	}

	messages := NewMessages(p)
	inbound, err := messages.Save(ctx, &domain.Message{
		InstanceName: "juanse", RemoteJID: phoneJID, WAMessageID: "VERO-IN",
		PushName: "Vero", Direction: domain.Inbound, Origin: domain.OriginContact,
		Type: domain.TypeText, Status: domain.StatusSent, Body: "¿Venís?",
		Timestamp: time.Now().UTC(),
	}, "")
	if err != nil {
		t.Fatalf("guardando entrante: %v", err)
	}
	outbound, err := messages.Save(ctx, &domain.Message{
		InstanceName: "juanse", RemoteJID: lidJID, WAMessageID: "VERO-OUT",
		Direction: domain.Outbound, Origin: domain.OriginDevice,
		Type: domain.TypeText, Status: domain.StatusSent, Body: "Dale",
		Timestamp: time.Now().UTC().Add(time.Second),
	}, "")
	if err != nil {
		t.Fatalf("guardando saliente por LID: %v", err)
	}

	if outbound.ContactID != contactID || outbound.ConversationID != inbound.ConversationID {
		t.Fatalf("saliente quedó en %s/%s; se esperaba %s/%s",
			outbound.ContactID, outbound.ConversationID, contactID, inbound.ConversationID)
	}
	var remoteJID string
	var conversations, storedMessages int
	if err := p.QueryRow(ctx, `SELECT remote_jid FROM conversations WHERE id=$1`, inbound.ConversationID).Scan(&remoteJID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM conversations`).Scan(&conversations); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id=$1`, inbound.ConversationID).Scan(&storedMessages); err != nil {
		t.Fatal(err)
	}
	if remoteJID != phoneJID || conversations != 1 || storedMessages != 2 {
		t.Fatalf("remote=%q conversations=%d messages=%d; se esperaba %q/1/2",
			remoteJID, conversations, storedMessages, phoneJID)
	}
}

func TestAgendaFusionaUnaConversacionLIDQueYaExistia(t *testing.T) {
	ctx, p := testDB(t)
	seedLine(t, ctx, p, "juanse", "Juanse")

	phoneJID := "5491128709473@s.whatsapp.net"
	lidJID := "37035637244007@lid"
	before, err := NewMessages(p).Save(ctx, &domain.Message{
		InstanceName: "juanse", RemoteJID: lidJID, WAMessageID: "VERO-OLD-OUT",
		PushName: "Vero", Direction: domain.Outbound, Origin: domain.OriginDevice,
		Type: domain.TypeText, Status: domain.StatusSent, Body: "Que pasa",
		Timestamp: time.Now().UTC(),
	}, "")
	if err != nil {
		t.Fatalf("guardando saliente previo al mapeo: %v", err)
	}

	if err := NewContacts(p).UpsertDirectory(ctx, "juanse", []domain.ContactIdentity{{
		PhoneJID: phoneJID, LIDJID: lidJID, FullName: "Vero Traversari", PushName: "Vero",
	}}); err != nil {
		t.Fatalf("guardando mapeo de agenda: %v", err)
	}
	after, err := NewMessages(p).Save(ctx, &domain.Message{
		InstanceName: "juanse", RemoteJID: phoneJID, WAMessageID: "VERO-NEW-IN",
		PushName: "Vero", Direction: domain.Inbound, Origin: domain.OriginContact,
		Type: domain.TypeText, Status: domain.StatusSent, Body: "Prende el horno",
		Timestamp: time.Now().UTC().Add(time.Second),
	}, "")
	if err != nil {
		t.Fatalf("guardando entrante posterior al mapeo: %v", err)
	}

	var conversations, storedMessages int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM conversations`).Scan(&conversations); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id=$1`, after.ConversationID).Scan(&storedMessages); err != nil {
		t.Fatal(err)
	}
	if conversations != 1 || storedMessages != 2 || before.ConversationID != after.ConversationID {
		t.Fatalf("antes=%s después=%s conversations=%d messages=%d; se esperaba un único hilo conservando su id",
			before.ConversationID, after.ConversationID, conversations, storedMessages)
	}
}

func TestLineasCuentanChatsConMensajesNoLeidos(t *testing.T) {
	ctx, p := testDB(t)

	ana := seedLine(t, ctx, p, "ana", "Ana")
	seedLine(t, ctx, p, "pedro", "Pedro") // sin conversaciones
	now := time.Now().UTC()

	c1 := seedContact(t, ctx, p, "c1@s.whatsapp.net", "Uno", "")
	c2 := seedContact(t, ctx, p, "c2@s.whatsapp.net", "Dos", "")
	seedConversation(t, ctx, p, ana, c1, now, 3)
	seedConversation(t, ctx, p, ana, c2, now, 4)

	lines, err := NewRead(p).Lines(ctx)
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("se obtuvieron %d líneas, se esperaban 2", len(lines))
	}

	byLabel := map[string]int{}
	for _, l := range lines {
		byLabel[l.Label] = l.Unread
	}
	if byLabel["Ana"] != 2 {
		t.Fatalf("chats no leídos de Ana = %d, se esperaban 2", byLabel["Ana"])
	}
	// La clave del LEFT JOIN: una línea sin pendientes tiene que seguir apareciendo
	// con cero, no desaparecer del filtro.
	if _, ok := byLabel["Pedro"]; !ok {
		t.Fatal("la línea sin conversaciones desapareció del listado")
	}
	if byLabel["Pedro"] != 0 {
		t.Fatalf("sin-revisar de Pedro = %d, se esperaban 0", byLabel["Pedro"])
	}
}

// --- Hilo -------------------------------------------------------------------

func TestHiloPaginaHaciaAtrasSinHuecos(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "ana", "Ana")
	contact := seedContact(t, ctx, p, "c1@s.whatsapp.net", "Uno", "")
	base := time.Now().UTC().Truncate(time.Second)
	conv := seedConversation(t, ctx, p, line, contact, base, 0)

	// 10 mensajes, del más antiguo al más reciente.
	for i := 0; i < 10; i++ {
		seedMessage(t, ctx, p, conv, line, contact,
			fmt.Sprintf("wa-%d", i), fmt.Sprintf("mensaje %d", i),
			base.Add(time.Duration(i)*time.Minute))
	}

	var recogidos []string
	q := store.ThreadQuery{ConversationID: conv, Limit: 4}
	for pagina := 0; pagina < 5; pagina++ {
		msgs, err := read.Thread(ctx, q)
		if err != nil {
			t.Fatalf("hilo, página %d: %v", pagina, err)
		}
		if len(msgs) == 0 {
			break
		}
		hayMas := len(msgs) > q.Limit
		if hayMas {
			msgs = msgs[:q.Limit]
		}
		for _, m := range msgs {
			recogidos = append(recogidos, m.Body)
		}
		if !hayMas {
			break
		}
		q.Before = msgs[len(msgs)-1].Cursor()
	}

	if len(recogidos) != 10 {
		t.Fatalf("se recogieron %d mensajes en total, se esperaban 10: %v", len(recogidos), recogidos)
	}
	// Del más reciente al más antiguo, sin repetir ni saltarse ninguno.
	for i, body := range recogidos {
		if want := fmt.Sprintf("mensaje %d", 9-i); body != want {
			t.Fatalf("posición %d = %q, se esperaba %q", i, body, want)
		}
	}
}

// El modo `around` es lo que permite que un resultado de búsqueda abra el hilo EN
// el mensaje encontrado y no al final del chat.
func TestHiloAroundCentraLaVentanaEnElMensaje(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "ana", "Ana")
	contact := seedContact(t, ctx, p, "c1@s.whatsapp.net", "Uno", "")
	base := time.Now().UTC().Truncate(time.Second)
	conv := seedConversation(t, ctx, p, line, contact, base, 0)

	ids := make([]string, 40)
	for i := range ids {
		ids[i] = seedMessage(t, ctx, p, conv, line, contact,
			fmt.Sprintf("wa-%d", i), fmt.Sprintf("mensaje %d", i),
			base.Add(time.Duration(i)*time.Minute))
	}
	centro := ids[20]

	msgs, err := read.Thread(ctx, store.ThreadQuery{
		ConversationID: conv, AroundID: centro, Limit: 10,
	})
	if err != nil {
		t.Fatalf("Thread around: %v", err)
	}

	var encontrado bool
	for _, m := range msgs {
		if m.ID == centro {
			encontrado = true
		}
	}
	if !encontrado {
		t.Fatal("la ventana no contiene el mensaje pedido")
	}
	// Con límite 10 se piden 5 a cada lado: la ventana no puede ser el hilo entero.
	if len(msgs) > 12 || len(msgs) < 5 {
		t.Fatalf("la ventana tiene %d mensajes, se esperaba una centrada de ~10", len(msgs))
	}
}

func TestHiloDevuelveElAdjuntoEnLaMismaConsulta(t *testing.T) {
	ctx, p := testDB(t)

	line := seedLine(t, ctx, p, "ana", "Ana")
	contact := seedContact(t, ctx, p, "c1@s.whatsapp.net", "Uno", "")
	now := time.Now().UTC()
	conv := seedConversation(t, ctx, p, line, contact, now, 0)

	conAdjunto := seedMessage(t, ctx, p, conv, line, contact, "wa-1", "mirá", now)
	seedMessage(t, ctx, p, conv, line, contact, "wa-2", "sin adjunto", now.Add(time.Minute))

	if _, err := p.Exec(ctx,
		`INSERT INTO media_assets (message_id, mime_type, file_name, storage_status)
		 VALUES ($1, 'image/jpeg', 'pasaporte.jpg', 'stored')`, conAdjunto); err != nil {
		t.Fatalf("sembrando el adjunto: %v", err)
	}

	msgs, err := NewRead(p).Thread(ctx, store.ThreadQuery{ConversationID: conv, Limit: 10})
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}

	byID := map[string]store.ThreadMessage{}
	for _, m := range msgs {
		byID[m.ID] = m
	}
	got := byID[conAdjunto]
	if got.Media == nil {
		t.Fatal("el mensaje con adjunto llegó sin media")
	}
	if got.Media.MimeType != "image/jpeg" || got.Media.Status != "stored" {
		t.Fatalf("media = %+v, se esperaba image/jpeg / stored", got.Media)
	}
	for _, m := range msgs {
		if m.ID != conAdjunto && m.Media != nil {
			t.Fatal("un mensaje sin adjunto llegó con media distinta de nil")
		}
	}
}

// --- Marcar revisado --------------------------------------------------------

func TestMarkReadPoneACeroYEsIdempotente(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "ana", "Ana")
	contact := seedContact(t, ctx, p, "c1@s.whatsapp.net", "Uno", "")
	conv := seedConversation(t, ctx, p, line, contact, time.Now().UTC(), 5)

	for i := 0; i < 3; i++ {
		if err := read.MarkRead(ctx, conv); err != nil {
			t.Fatalf("MarkRead, llamada %d: %v", i+1, err)
		}
	}

	got, err := read.Conversation(ctx, conv)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if got.Unread != 0 {
		t.Fatalf("sin-revisar = %d, se esperaba 0", got.Unread)
	}
}

func TestLecturaDesdeWhatsAppEncuentraLaConversacionPorSuAliasLID(t *testing.T) {
	ctx, p := testDB(t)

	line := seedLine(t, ctx, p, "ana", "Ana")
	phoneJID := "5491112345678@s.whatsapp.net"
	lidJID := "12345678901234@lid"
	contact := seedContact(t, ctx, p, phoneJID, "Uno", "+5491112345678")
	if _, err := p.Exec(ctx, `UPDATE contacts SET lid_jid=$2 WHERE id=$1`, contact, lidJID); err != nil {
		t.Fatal(err)
	}
	conv := seedConversation(t, ctx, p, line, contact, time.Now().UTC(), 5)

	changed, err := NewMessages(p).MarkReadFromDevice(ctx, "ana", lidJID)
	if err != nil {
		t.Fatalf("MarkReadFromDevice: %v", err)
	}
	if len(changed) != 1 || changed[0] != conv {
		t.Fatalf("changed conversations = %v, want [%s]", changed, conv)
	}

	got, err := NewRead(p).Conversation(ctx, conv)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if got.Unread != 0 {
		t.Fatalf("unread after the linked-device receipt = %d, want 0", got.Unread)
	}
}

func TestMigracionDeLecturasConservaSoloMensajesPosterioresALaUltimaLectura(t *testing.T) {
	ctx, p := testDB(t)

	line := seedLine(t, ctx, p, "ana", "Ana")
	chat := "5491112345678@s.whatsapp.net"
	contact := seedContact(t, ctx, p, chat, "Uno", "+5491112345678")
	readAt := time.Date(2026, 8, 24, 15, 0, 0, 0, time.UTC)
	conv := seedConversation(t, ctx, p, line, contact, readAt.Add(time.Hour), 3)
	seedMessage(t, ctx, p, conv, line, contact, "before-read", "ya leido", readAt.Add(-time.Minute))
	seedMessage(t, ctx, p, conv, line, contact, "after-read", "pendiente", readAt.Add(time.Minute))

	payload := fmt.Sprintf(`{
		"event":"Receipt","instanceName":"ana","data":{
			"Chat":%q,"IsFromMe":true,"MessageIDs":["before-read"],
			"Timestamp":%q,"Type":"read"
		}}`, chat, readAt.Format(time.RFC3339))
	if _, err := p.Exec(ctx, `
		INSERT INTO webhook_events (instance_name, event, dedupe_key, payload, status)
		VALUES ('ana', 'Receipt', 'read-receipt', $1::jsonb, 'processed')`, payload); err != nil {
		t.Fatalf("seeding read receipt: %v", err)
	}

	migration, err := os.ReadFile("../../migrations/0008_reconcile_whatsapp_reads.sql")
	if err != nil {
		t.Fatalf("reading reconciliation migration: %v", err)
	}
	if _, err := p.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("running reconciliation migration: %v", err)
	}

	got, err := NewRead(p).Conversation(ctx, conv)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if got.Unread != 1 {
		t.Fatalf("unread after reconciliation = %d, want only the message after the receipt", got.Unread)
	}
}

func TestConversacionInexistenteDaErrNotFound(t *testing.T) {
	ctx, p := testDB(t)

	_, err := NewRead(p).Conversation(ctx, "8f14e45f-ceea-467a-9575-1e1e6b46f14e")
	if err != store.ErrNotFound {
		t.Fatalf("error = %v, se esperaba ErrNotFound", err)
	}
}

// --- Búsqueda ---------------------------------------------------------------

func TestBusquedaEncuentraYRecorta(t *testing.T) {
	ctx, p := testDB(t)
	read := NewRead(p)

	line := seedLine(t, ctx, p, "ana", "Ana")
	contact := seedContact(t, ctx, p, "c1@s.whatsapp.net", "PERFIL PUBLICO", "")
	if _, err := p.Exec(ctx, `
		INSERT INTO contact_directory (instance_id, wa_jid, full_name, push_name)
		VALUES ($1, 'c1@s.whatsapp.net', 'Ana María', 'PERFIL PUBLICO')`, line); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	conv := seedConversation(t, ctx, p, line, contact, now, 0)

	seedMessage(t, ctx, p, conv, line, contact, "wa-1",
		"Tu código de reserva es XYZ789, guardalo", now)
	seedMessage(t, ctx, p, conv, line, contact, "wa-2", "hola qué tal", now.Add(time.Minute))

	hits, err := read.SearchMessages(ctx, store.SearchQuery{Text: "XYZ789", Limit: 10})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("se obtuvieron %d resultados, se esperaba 1", len(hits))
	}
	if hits[0].ContactName != "Ana María" || hits[0].InstanceLbl != "Ana" {
		t.Fatalf("resultado sin contexto de contacto/línea: %+v", hits[0])
	}
	if hits[0].Snippet == "" {
		t.Fatal("el resultado llegó sin fragmento")
	}
}

func TestBusquedaRechazaTerminosCortos(t *testing.T) {
	ctx, p := testDB(t)

	_, err := NewRead(p).SearchMessages(ctx, store.SearchQuery{Text: "ab", Limit: 10})
	if err != store.ErrSearchTooShort {
		t.Fatalf("error = %v, se esperaba ErrSearchTooShort", err)
	}
}

// Un mensaje borrado sigue en la base para que el borrado quede registrado, pero
// no puede aparecer como resultado de búsqueda.
func TestBusquedaIgnoraLosBorrados(t *testing.T) {
	ctx, p := testDB(t)

	line := seedLine(t, ctx, p, "ana", "Ana")
	contact := seedContact(t, ctx, p, "c1@s.whatsapp.net", "Uno", "")
	now := time.Now().UTC()
	conv := seedConversation(t, ctx, p, line, contact, now, 0)

	id := seedMessage(t, ctx, p, conv, line, contact, "wa-1", "reserva XYZ789", now)
	if _, err := p.Exec(ctx,
		`UPDATE messages SET deleted_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("marcando el borrado: %v", err)
	}

	hits, err := NewRead(p).SearchMessages(ctx, store.SearchQuery{Text: "XYZ789", Limit: 10})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("se obtuvieron %d resultados, se esperaba 0: el borrado no debe aparecer", len(hits))
	}
}

// --- Purga ------------------------------------------------------------------

func TestPurgaBorraSoloLoResueltoYViejo(t *testing.T) {
	ctx, p := testDB(t)

	insert := func(dedupe, status string, age time.Duration) {
		t.Helper()
		_, err := p.Exec(ctx,
			`INSERT INTO webhook_events (instance_name, event, dedupe_key, payload, status, received_at)
			 VALUES ('ana', 'messages.upsert', $1, '{}'::jsonb, $2::event_status, now() - $3::interval)`,
			dedupe, status, fmt.Sprintf("%d hours", int(age.Hours())))
		if err != nil {
			t.Fatalf("sembrando evento %s: %v", dedupe, err)
		}
	}

	insert("viejo-procesado", "processed", 48*time.Hour) // se borra
	insert("viejo-saltado", "skipped", 48*time.Hour)     // se borra
	insert("viejo-pendiente", "pending", 48*time.Hour)   // se conserva: hay que reprocesarlo
	insert("viejo-fallido", "failed", 48*time.Hour)      // se conserva: hay que reprocesarlo
	insert("nuevo-procesado", "processed", 1*time.Hour)  // se conserva: dentro de la retención

	deleted, err := NewMaintenance(p).PurgeEvents(ctx, time.Now().Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatalf("PurgeEvents: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("se borraron %d filas, se esperaban 2", deleted)
	}

	var quedan []string
	rows, err := p.Query(ctx, `SELECT dedupe_key FROM webhook_events ORDER BY dedupe_key`)
	if err != nil {
		t.Fatalf("releyendo: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		quedan = append(quedan, k)
	}

	want := []string{"nuevo-procesado", "viejo-fallido", "viejo-pendiente"}
	if len(quedan) != len(want) {
		t.Fatalf("quedaron %v, se esperaba %v", quedan, want)
	}
	for i := range want {
		if quedan[i] != want[i] {
			t.Fatalf("quedaron %v, se esperaba %v", quedan, want)
		}
	}
}

// El borrado va por lotes para no mantener una transacción abierta sobre millones
// de filas. El bucle tiene que seguir hasta agotar, no parar en el primer lote.
func TestPurgaAgotaVariosLotes(t *testing.T) {
	ctx, p := testDB(t)

	for i := 0; i < 25; i++ {
		_, err := p.Exec(ctx,
			`INSERT INTO webhook_events (instance_name, event, dedupe_key, payload, status, received_at)
			 VALUES ('ana', 'messages.upsert', $1, '{}'::jsonb, 'processed', now() - interval '48 hours')`,
			fmt.Sprintf("evento-%d", i))
		if err != nil {
			t.Fatalf("sembrando: %v", err)
		}
	}

	deleted, err := NewMaintenance(p).PurgeEvents(ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("PurgeEvents: %v", err)
	}
	if deleted != 25 {
		t.Fatalf("se borraron %d filas en lotes de 10, se esperaban 25", deleted)
	}
}
