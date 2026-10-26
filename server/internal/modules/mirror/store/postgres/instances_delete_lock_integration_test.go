package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// Desvincular una línea mientras se importa su historial terminaba en
// «deadlock detected»: la ingesta bloqueaba la conversación y recién después la
// línea (por la FK del mensaje), y el borrado lo hacía al revés por el cascade.
// La ingesta tiene que tomar primero la línea; mientras el borrado la tenga, no
// puede haber agarrado ninguna conversación.
func TestSaveWaitsForTheLineBeforeLockingItsConversations(t *testing.T) {
	ctx, p := testDB(t)
	line := seedLine(t, ctx, p, "borrando", "Borrando")
	contact := seedContact(t, ctx, p, "5491111111111@s.whatsapp.net", "Sofía", "+5491111111111")
	seedConversation(t, ctx, p, line, contact, time.Now().UTC(), 0)

	deleting, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = deleting.Rollback(ctx) }()
	if _, err := deleting.Exec(ctx, `SELECT 1 FROM instances WHERE id=$1 FOR UPDATE`, line); err != nil {
		t.Fatal(err)
	}

	saved := make(chan error, 1)
	go func() {
		_, err := NewMessages(p).Save(ctx, &domain.Message{
			InstanceName: "borrando", RemoteJID: "5491111111111@s.whatsapp.net", WAMessageID: "LOCK-1",
			Direction: domain.Inbound, Origin: domain.OriginContact, Type: domain.TypeText,
			Status: domain.StatusSent, Body: "hola", Timestamp: time.Now().UTC(),
		}, "")
		saved <- err
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		if err := p.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			                WHERE datname = current_database() AND wait_event_type = 'Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Save nunca quedó esperando a la línea")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := deleting.Exec(ctx,
		`SELECT 1 FROM conversations WHERE instance_id=$1 FOR UPDATE NOWAIT`, line); err != nil {
		t.Fatalf("Save bloqueó la conversación antes que la línea: %v", err)
	}
	if _, err := deleting.Exec(ctx, `DELETE FROM instances WHERE id=$1`, line); err != nil {
		t.Fatal(err)
	}
	if err := deleting.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Save sobre una línea borrada = %v, want ErrNotFound", err)
	}
}
