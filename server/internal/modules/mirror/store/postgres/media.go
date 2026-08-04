package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

type Media struct{ db *pgxpool.Pool }

func NewMedia(db *pgxpool.Pool) *Media { return &Media{db: db} }

// NextPending recupera los adjuntos que quedaron a medias: los que nunca se
// procesaron y los que fallaron sin agotar reintentos.
//
// FOR UPDATE SKIP LOCKED hace que varias réplicas del backend puedan barrer la
// cola a la vez sin repartirse el mismo trabajo ni bloquearse entre ellas.
func (m *Media) NextPending(ctx context.Context, limit int) ([]store.PendingMedia, error) {
	const q = `
		SELECT ma.id, ma.message_id, i.evolution_name, msg.wa_message_id,
		       COALESCE(ma.mime_type,''), COALESCE(ma.file_name,''), ma.attempts
		  FROM media_assets ma
		  JOIN messages  msg ON msg.id = ma.message_id
		  JOIN instances i   ON i.id   = msg.instance_id
		 WHERE ma.storage_status IN ('pending','failed')
		   AND ma.attempts < 5
		 ORDER BY ma.created_at
		 LIMIT $1
		   FOR UPDATE OF ma SKIP LOCKED`

	rows, err := m.db.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.PendingMedia
	for rows.Next() {
		var p store.PendingMedia
		if err := rows.Scan(&p.ID, &p.MessageID, &p.InstanceName,
			&p.WAMessageID, &p.MimeType, &p.FileName, &p.Attempts); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (m *Media) MarkProcessing(ctx context.Context, id string) error {
	const q = `
		UPDATE media_assets
		   SET storage_status = 'processing', attempts = attempts + 1
		 WHERE id = $1`
	_, err := m.db.Exec(ctx, q, id)
	return err
}

func (m *Media) MarkStored(ctx context.Context, id, storageKey, sha256 string, size int64) error {
	const q = `
		UPDATE media_assets
		   SET storage_status = 'stored', storage_key = $2, sha256 = $3,
		       size_bytes = $4, stored_at = now(), last_error = NULL
		 WHERE id = $1`
	_, err := m.db.Exec(ctx, q, id, storageKey, sha256, size)
	return err
}

func (m *Media) MarkFailed(ctx context.Context, id, cause string) error {
	const q = `
		UPDATE media_assets
		   SET storage_status = 'failed', last_error = $2
		 WHERE id = $1`
	_, err := m.db.Exec(ctx, q, id, cause)
	return err
}

// StorageKey devuelve la clave en R2 y el mime. Lo usa el endpoint que emite la
// URL firmada.
func (m *Media) StorageKey(ctx context.Context, id string) (string, string, error) {
	var key, mimeType string
	err := m.db.QueryRow(ctx,
		`SELECT COALESCE(storage_key,''), COALESCE(mime_type,'')
		   FROM media_assets
		  WHERE id = $1 AND storage_status = 'stored'`, id).Scan(&key, &mimeType)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", store.ErrNotFound
	}
	return key, mimeType, err
}
