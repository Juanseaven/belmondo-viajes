package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Los archivos del cliente: las fotos, videos y documentos de los chats
// individuales de sus contactos. Los guarda el espejo (messages y
// media_assets, tablas públicas que acá solo se leen); operaciones guarda
// únicamente qué archivos son favoritos.

// fileTypes son los tipos de mensaje que entran en Archivos. Los audios se
// escuchan en el chat y los stickers no son archivos de nadie.
const fileTypes = `('image', 'video', 'document')`

// customerFilesSQL son los archivos de un cliente. Un grupo no entra: ahí manda
// fotos gente que no es el cliente.
const customerFilesSQL = `
	SELECT ma.id::text AS asset_id, m.id::text AS message_id, cv.id::text AS conversation_id,
	       m.type::text AS type, coalesce(ma.mime_type, '') AS mime_type,
	       coalesce(ma.file_name, '') AS file_name, ma.size_bytes, ma.width, ma.height,
	       ma.duration_seconds, m.direction::text AS direction, m.wa_timestamp,
	       ma.storage_status::text AS status, coalesce(m.body, '') AS caption,
	       coalesce(i.owner_label, '') AS instance_label, ff.message_id IS NOT NULL AS favorite
	  FROM customer_contacts cc
	  JOIN conversations cv ON cv.contact_id = cc.contact_id AND NOT cv.is_group
	  JOIN instances i ON i.id = cv.instance_id
	  JOIN messages m ON m.conversation_id = cv.id AND m.deleted_at IS NULL AND m.type IN ` + fileTypes + `
	  JOIN media_assets ma ON ma.message_id = m.id
	  LEFT JOIN file_favorites ff ON ff.message_id = m.id
	 WHERE cc.customer_id = $1`

// CustomerFiles lista una de las dos listas de Archivos —fotos y videos, o
// documentos—, de la más nueva a la más vieja, con los números de los chips.
func (s *Store) CustomerFiles(ctx context.Context, customerID string, f store.FileFilter) (store.FilePage, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 60
	}
	page := store.FilePage{Items: []store.CustomerFile{}, Counts: map[string]int{}}

	var todos, favoritos, documentos int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*)::int, count(*) FILTER (WHERE favorite)::int, count(*) FILTER (WHERE type = 'document')::int
		   FROM (`+customerFilesSQL+`) f`, customerID).Scan(&todos, &favoritos, &documentos); err != nil {
		return store.FilePage{}, writeError("contando los archivos", err)
	}
	page.Counts["todos"], page.Counts["favoritos"], page.Counts["documentos"] = todos, favoritos, documentos

	q := &query{args: []any{customerID}}
	conds := []string{"true"}
	if f.Kind == store.FilesDocuments {
		conds = append(conds, "type = 'document'")
	} else {
		conds = append(conds, "type <> 'document'")
	}
	if f.Favorites {
		conds = append(conds, "favorite")
	}
	if f.Cursor != "" {
		at, id, err := decodeFileCursor(f.Cursor)
		if err != nil {
			return store.FilePage{}, store.ErrBadCursor
		}
		atArg, idArg := q.arg(at), q.arg(id)
		conds = append(conds, fmt.Sprintf("(wa_timestamp < %[1]s OR (wa_timestamp = %[1]s AND message_id < %[2]s))", atArg, idArg))
	}

	rows, err := s.pool.Query(ctx,
		`SELECT * FROM (`+customerFilesSQL+`) f WHERE `+strings.Join(conds, " AND ")+
			fmt.Sprintf(` ORDER BY wa_timestamp DESC, message_id DESC LIMIT %d`, f.Limit+1), q.args...)
	if err != nil {
		return store.FilePage{}, writeError("listando los archivos", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.CustomerFile, error) {
		var c store.CustomerFile
		err := row.Scan(&c.AssetID, &c.MessageID, &c.ConversationID, &c.Type, &c.MimeType, &c.FileName,
			&c.SizeBytes, &c.Width, &c.Height, &c.DurationSeconds, &c.Direction, &c.SentAt, &c.Status,
			&c.Caption, &c.InstanceLabel, &c.Favorite)
		return c, err
	})
	if err != nil {
		return store.FilePage{}, fmt.Errorf("armando los archivos: %w", err)
	}
	if len(items) > f.Limit {
		items = items[:f.Limit]
		last := items[len(items)-1]
		page.NextCursor = encodeFileCursor(last.SentAt, last.MessageID)
	}
	page.Items = items
	return page, nil
}

func encodeFileCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeFileCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, "", fmt.Errorf("cursor sin mensaje")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	return t, id, err
}

// SetFavorite marca o desmarca un archivo con la estrella. Solo se marcan
// mensajes con archivo de los tipos que entran en Archivos.
func (s *Store) SetFavorite(ctx context.Context, messageID, userID string, on bool) error {
	if !on {
		if _, err := s.pool.Exec(ctx, `DELETE FROM file_favorites WHERE message_id = $1`, messageID); err != nil {
			return writeError("desmarcando el favorito", err)
		}
		return nil
	}
	var isFile bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM media_assets ma WHERE ma.message_id = m.id) AND m.type IN `+fileTypes+`
		   FROM messages m WHERE m.id = $1`, messageID).Scan(&isFile)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	if err != nil {
		return writeError("buscando el archivo", err)
	}
	if !isFile {
		return fmt.Errorf("%w: el mensaje no tiene un archivo", store.ErrInvalid)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO file_favorites (message_id, marked_by) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		messageID, userID); err != nil {
		return writeError("marcando el favorito", err)
	}
	return nil
}
