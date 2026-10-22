package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

var fileSeq int

// seedFile siembra un mensaje del cliente con su archivo. Sin tipo de archivo
// (texto), no lleva media_assets.
func seedFile(t *testing.T, ctx context.Context, p *pgxpool.Pool, convID, instanceID, contactID, msgType string, at time.Time) string {
	t.Helper()
	fileSeq++
	var id string
	if err := p.QueryRow(ctx,
		`INSERT INTO messages (conversation_id, instance_id, contact_id, wa_message_id, direction, origin, type, body, wa_timestamp)
		 VALUES ($1, $2, $3, $4, 'inbound', 'contact', $5, 'adjunto', $6) RETURNING id`,
		convID, instanceID, contactID, fmt.Sprintf("wa-file-%d", fileSeq), msgType, at).Scan(&id); err != nil {
		t.Fatalf("sembrando mensaje %s: %v", msgType, err)
	}
	if msgType != "text" {
		if _, err := p.Exec(ctx,
			`INSERT INTO media_assets (message_id, mime_type, file_name, storage_status) VALUES ($1, 'image/jpeg', 'foto.jpg', 'stored')`,
			id); err != nil {
			t.Fatalf("sembrando archivo: %v", err)
		}
	}
	return id
}

func TestLosArchivosJuntanLosChatsIndividualesDelCliente(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "archivos@agencia.test")
	cliente := newCustomer(t, s, user, store.NewCustomer{Name: "Marta"})
	lineA := seedNamedInstance(t, ctx, pool, "linea-a")
	lineB := seedNamedInstance(t, ctx, pool, "linea-b")

	propio := seedContact(t, ctx, pool, "marta@s.whatsapp.net", "Marta")
	oficina := seedContact(t, ctx, pool, "marta-oficina@s.whatsapp.net", "Marta oficina")
	grupo := seedContact(t, ctx, pool, "familia@g.us", "Familia")
	for _, c := range []string{propio, oficina, grupo} {
		if err := s.LinkContact(ctx, cliente.ID, c, user); err != nil {
			t.Fatal(err)
		}
	}
	convA := seedConversation(t, ctx, pool, lineA, propio)
	convB := seedConversation(t, ctx, pool, lineB, oficina)
	var convG string
	if err := pool.QueryRow(ctx, `INSERT INTO conversations (instance_id, contact_id, remote_jid, is_group)
		VALUES ($1, $2, 'familia@g.us', true) RETURNING id`, lineA, grupo).Scan(&convG); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	foto1 := seedFile(t, ctx, pool, convA, lineA, propio, "image", base)
	video := seedFile(t, ctx, pool, convB, lineB, oficina, "video", base.Add(time.Hour))
	foto2 := seedFile(t, ctx, pool, convA, lineA, propio, "image", base.Add(2*time.Hour))
	doc := seedFile(t, ctx, pool, convB, lineB, oficina, "document", base.Add(3*time.Hour))
	seedFile(t, ctx, pool, convA, lineA, propio, "audio", base.Add(4*time.Hour))
	seedFile(t, ctx, pool, convA, lineA, propio, "sticker", base.Add(5*time.Hour))
	seedFile(t, ctx, pool, convG, lineA, grupo, "image", base.Add(6*time.Hour))
	borrada := seedFile(t, ctx, pool, convA, lineA, propio, "image", base.Add(7*time.Hour))
	if _, err := pool.Exec(ctx, `UPDATE messages SET deleted_at = now() WHERE id = $1`, borrada); err != nil {
		t.Fatal(err)
	}
	texto := seedFile(t, ctx, pool, convA, lineA, propio, "text", base.Add(8*time.Hour))

	page, err := s.CustomerFiles(ctx, cliente.ID, store.FileFilter{Kind: store.FilesMedia, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Counts["todos"] != 4 || page.Counts["documentos"] != 1 || page.Counts["favoritos"] != 0 {
		t.Fatalf("chips = %v", page.Counts)
	}
	if len(page.Items) != 2 || page.Items[0].MessageID != foto2 || page.Items[1].MessageID != video || page.NextCursor == "" {
		t.Fatalf("primera página = %+v", page.Items)
	}
	if page.Items[1].InstanceLabel == "" || page.Items[1].ConversationID != convB || page.Items[1].Direction != "inbound" {
		t.Fatalf("archivo = %+v", page.Items[1])
	}
	next, err := s.CustomerFiles(ctx, cliente.ID, store.FileFilter{Kind: store.FilesMedia, Limit: 2, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].MessageID != foto1 || next.NextCursor != "" {
		t.Fatalf("segunda página = %+v, %v", next.Items, err)
	}
	docs, err := s.CustomerFiles(ctx, cliente.ID, store.FileFilter{Kind: store.FilesDocuments})
	if err != nil || len(docs.Items) != 1 || docs.Items[0].MessageID != doc {
		t.Fatalf("documentos = %+v, %v", docs.Items, err)
	}

	// La estrella es de la agencia: la marca uno y la ve cualquiera.
	if err := s.SetFavorite(ctx, foto1, user, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFavorite(ctx, foto1, user, true); err != nil {
		t.Fatalf("marcar dos veces: %v", err)
	}
	favs, err := s.CustomerFiles(ctx, cliente.ID, store.FileFilter{Kind: store.FilesMedia, Favorites: true})
	if err != nil || len(favs.Items) != 1 || !favs.Items[0].Favorite || favs.Counts["favoritos"] != 1 {
		t.Fatalf("favoritos = %+v, %v", favs, err)
	}
	if err := s.SetFavorite(ctx, foto1, user, false); err != nil {
		t.Fatal(err)
	}
	if favs, _ := s.CustomerFiles(ctx, cliente.ID, store.FileFilter{Favorites: true}); len(favs.Items) != 0 {
		t.Fatalf("desmarcar no la sacó: %+v", favs.Items)
	}

	if err := s.SetFavorite(ctx, texto, user, true); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("un texto no se marca: %v", err)
	}
	if err := s.SetFavorite(ctx, "00000000-0000-0000-0000-000000000000", user, true); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("un mensaje que no existe: %v", err)
	}
}
