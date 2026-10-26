package postgres

import (
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
)

// El historial se importa por tandas en una transacción. La tanda tiene que
// dejar la bandeja igual que si cada mensaje se hubiera guardado por separado, y
// reimportarla —un lote reintentado— no puede duplicar nada.
func TestSaveHistoryPersistsTheChunkAndIsIdempotent(t *testing.T) {
	ctx, p := testDB(t)
	line := seedLine(t, ctx, p, "historial", "Historial")
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	history := func(jid, id, body string, at time.Time, direction domain.Direction) *domain.Message {
		origin := domain.OriginContact
		if direction == domain.Outbound {
			origin = domain.OriginDevice
		}
		return &domain.Message{
			InstanceName: "historial", RemoteJID: jid, WAMessageID: id, PushName: "Sofía",
			Direction: direction, Origin: origin, Type: domain.TypeText, Status: domain.StatusSent,
			Body: body, Timestamp: at, Historical: true,
		}
	}
	chunk := []*domain.Message{
		history("5491111111111@s.whatsapp.net", "H-1", "hola", base, domain.Inbound),
		history("5491111111111@s.whatsapp.net", "H-2", "último de Sofía", base.Add(time.Minute), domain.Outbound),
		history("5491222222222@s.whatsapp.net", "H-3", "otro chat", base.Add(2*time.Minute), domain.Inbound),
	}

	messages := NewMessages(p)
	if err := messages.SaveHistory(ctx, chunk); err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}
	if err := messages.SaveHistory(ctx, chunk); err != nil {
		t.Fatalf("SaveHistory reintentado: %v", err)
	}

	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM messages WHERE instance_id=$1`, line).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("mensajes = %d, want 3", count)
	}
	var preview string
	var unread int
	err := p.QueryRow(ctx, `
		SELECT c.last_message_preview, c.unread_count
		  FROM conversations c JOIN contacts ct ON ct.id = c.contact_id
		 WHERE c.instance_id=$1 AND ct.wa_jid='5491111111111@s.whatsapp.net'`, line).Scan(&preview, &unread)
	if err != nil {
		t.Fatal(err)
	}
	if preview != "último de Sofía" || unread != 0 {
		t.Fatalf("conversación = preview %q unread %d", preview, unread)
	}
}
