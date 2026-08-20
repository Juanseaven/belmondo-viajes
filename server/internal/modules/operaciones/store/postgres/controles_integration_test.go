package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Un control por acción se cierra con la prueba, no con la palabra. La planilla
// de papel decía que alguien avisó de la visa; acá queda el mensaje que lo dice.
func TestElCierrePorAccionGuardaElMensajeQueLoPrueba(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	mensaje := seedMessage(t, ctx, pool, f.conversation, f.instance, f.contact, f.user)

	if err := s.CloseControl(ctx, store.CloseControl{
		ControlID: control(t, ctx, pool, f.trip, "control-visa"),
		UserID:    f.user,
		MessageID: mensaje,
	}); err != nil {
		t.Fatalf("cerrando el control de visa: %v", err)
	}

	if estado(t, ctx, pool, f.trip, "control-visa") != "cerrado" {
		t.Error("el control quedó abierto")
	}

	var guardado string
	err := pool.QueryRow(ctx,
		`SELECT e.message_id FROM control_evidence e
		   JOIN controls c ON c.id = e.control_id
		  WHERE c.trip_id = $1 AND c.item_id = 'control-visa'`, f.trip).Scan(&guardado)
	if err != nil {
		t.Fatalf("el cierre no dejó evidencia: %v", err)
	}
	if guardado != mensaje {
		t.Errorf("la evidencia apunta a %s, se esperaba %s", guardado, mensaje)
	}
}

// Sin mensaje, un control por acción no se cierra. Permitirlo lo convertiría en
// un tilde: la única diferencia entre este diseño y la planilla de papel es que
// la prueba es obligatoria.
func TestUnControlPorAccionNoSeCierraSinMensaje(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	err := s.CloseControl(ctx, store.CloseControl{
		ControlID: control(t, ctx, pool, f.trip, "control-visa"),
		UserID:    f.user,
	})
	if !errors.Is(err, store.ErrEvidenciaRequerida) {
		t.Fatalf("error %v, se esperaba ErrEvidenciaRequerida", err)
	}
	if estado(t, ctx, pool, f.trip, "control-visa") != "abierto" {
		t.Error("el control se cerró igual, sin evidencia")
	}
}

// Un mensaje de otra conversación no prueba nada sobre este viaje. Sin esta
// validación, la evidencia sería un uuid cualquiera y el enlace mentiría.
func TestUnMensajeAjenoNoSirveComoEvidencia(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	otro := seedContact(t, ctx, pool, "5491199999999@s.whatsapp.net", "Otra persona")
	otraConv := seedConversation(t, ctx, pool, f.instance, otro)
	ajeno := seedMessage(t, ctx, pool, otraConv, f.instance, otro, f.user)

	err := s.CloseControl(ctx, store.CloseControl{
		ControlID: control(t, ctx, pool, f.trip, "control-visa"),
		UserID:    f.user,
		MessageID: ajeno,
	})
	if !errors.Is(err, store.ErrEvidenciaAjena) {
		t.Fatalf("error %v, se esperaba ErrEvidenciaAjena", err)
	}
	if estado(t, ctx, pool, f.trip, "control-visa") != "abierto" {
		t.Error("el control se cerró con un mensaje de otra conversación")
	}
}

// El manual no lleva mensaje, pero sí firma: quién lo confirmó y cuándo. Eso es
// lo que la hoja de papel buscaba y no podía dar.
func TestElCierreManualGuardaQuienYCuando(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	if err := s.CloseControl(ctx, store.CloseControl{
		ControlID: control(t, ctx, pool, f.trip, "control-comidas"),
		UserID:    f.user,
	}); err != nil {
		t.Fatalf("cerrando el control de comidas: %v", err)
	}

	var quien string
	var cuando time.Time
	err := pool.QueryRow(ctx,
		`SELECT e.closed_by, e.closed_at FROM control_evidence e
		   JOIN controls c ON c.id = e.control_id
		  WHERE c.trip_id = $1 AND c.item_id = 'control-comidas'`, f.trip).Scan(&quien, &cuando)
	if err != nil {
		t.Fatalf("el cierre manual no dejó firma: %v", err)
	}
	if quien != f.user {
		t.Errorf("firmó %s, se esperaba %s", quien, f.user)
	}
	if cuando.IsZero() {
		t.Error("la firma quedó sin fecha")
	}
}

// Un derivado no se cierra a mano: si se pudiera, volvería a ser una
// declaración y todo el diseño se cae.
func TestUnDerivadoNoSeCierraAMano(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	err := s.CloseControl(ctx, store.CloseControl{
		ControlID: control(t, ctx, pool, f.trip, "carga-nombres"),
		UserID:    f.user,
	})
	if !errors.Is(err, store.ErrDerivadoNoSeCierraAMano) {
		t.Fatalf("error %v, se esperaba ErrDerivadoNoSeCierraAMano", err)
	}
}

// Reabrir deja el control pendiente otra vez pero NO borra la evidencia: que
// alguna vez se cerró, y con qué, es parte de la historia del viaje.
func TestReabrirConservaLaHistoria(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	id := control(t, ctx, pool, f.trip, "control-comidas")
	if err := s.CloseControl(ctx, store.CloseControl{ControlID: id, UserID: f.user}); err != nil {
		t.Fatalf("cerrando: %v", err)
	}
	if err := s.ReopenControl(ctx, id); err != nil {
		t.Fatalf("reabriendo: %v", err)
	}

	if estado(t, ctx, pool, f.trip, "control-comidas") != "abierto" {
		t.Error("el control siguió cerrado")
	}
	var evidencias int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM control_evidence WHERE control_id = $1`, id).Scan(&evidencias); err != nil {
		t.Fatalf("contando evidencias: %v", err)
	}
	if evidencias != 1 {
		t.Errorf("quedaron %d evidencias, reabrir no debe borrar la historia", evidencias)
	}
}

// --- Escenario --------------------------------------------------------------

type fixture struct{ user, contact, instance, conversation, trip string }

func escenario(t *testing.T, ctx context.Context, pool *pgxpool.Pool, s *Store) fixture {
	t.Helper()
	f := fixture{}
	f.user = seedUser(t, ctx, pool, "vendedora@agencia.test")
	f.contact = seedContact(t, ctx, pool, "5491133212522@s.whatsapp.net", "Marina Ferreyra")
	f.instance = seedInstance(t, ctx, pool)
	f.conversation = seedConversation(t, ctx, pool, f.instance, f.contact)

	viaje, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: f.contact, Destination: "Cancún",
		DepartAt: fecha("2026-12-10"), ReturnAt: fecha("2026-12-24"), CreatedBy: f.user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}
	f.trip = viaje.ID
	return f
}

func control(t *testing.T, ctx context.Context, p *pgxpool.Pool, tripID, itemID string) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`SELECT id FROM controls WHERE trip_id = $1 AND item_id = $2`, tripID, itemID).Scan(&id)
	if err != nil {
		t.Fatalf("buscando el control %s: %v", itemID, err)
	}
	return id
}

func seedInstance(t *testing.T, ctx context.Context, p *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO instances (evolution_name, owner_label, status)
		 VALUES ('linea-test', 'Juanse', 'open') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando línea: %v", err)
	}
	return id
}

func seedConversation(t *testing.T, ctx context.Context, p *pgxpool.Pool, instanceID, contactID string) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO conversations (instance_id, contact_id, remote_jid, last_message_at,
		                            last_message_preview, last_message_type)
		 VALUES ($1, $2, 'jid', now(), 'hola', 'text') RETURNING id`,
		instanceID, contactID).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando conversación: %v", err)
	}
	return id
}

func seedMessage(t *testing.T, ctx context.Context, p *pgxpool.Pool, convID, instanceID, contactID, userID string) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO messages (conversation_id, instance_id, contact_id, wa_message_id,
		                       direction, origin, type, body, wa_timestamp, sent_by_user_id)
		 VALUES ($1, $2, $3, $4, 'outbound', 'panel', 'text', 'aviso', now(), $5)
		 RETURNING id`,
		convID, instanceID, contactID, "wa-"+convID[:8]+"-"+contactID[:8], userID).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando mensaje: %v", err)
	}
	return id
}
