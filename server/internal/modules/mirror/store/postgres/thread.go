package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// threadColumns proyecta el mensaje y su adjunto de una vez.
//
// El LEFT JOIN a media_assets evita el N+1 clásico del hilo: sin él, pintar 50
// mensajes con 10 imágenes son 10 consultas extra solo para saber si el archivo
// ya está subido.
const threadColumns = `
	m.id, m.direction::text, m.origin::text, m.type::text, m.status::text,
	coalesce(m.body, ''), m.wa_timestamp,
	coalesce(m.sent_by_user_id::text, ''), coalesce(u.full_name, ''),
	coalesce(m.quoted_wa_message_id, ''), m.edited_at, m.deleted_at,
	ma.id, coalesce(ma.mime_type, ''), coalesce(ma.file_name, ''),
	coalesce(ma.size_bytes, 0), coalesce(ma.duration_seconds, 0),
	coalesce(ma.width, 0), coalesce(ma.height, 0), ma.storage_status::text`

const threadFrom = `
	FROM messages m
	LEFT JOIN users        u  ON u.id = m.sent_by_user_id
	LEFT JOIN media_assets ma ON ma.message_id = m.id`

func scanThreadMessage(rows pgx.Rows) (store.ThreadMessage, error) {
	var (
		m        store.ThreadMessage
		mediaID  *string
		md       store.ThreadMedia
		mdStatus *string
	)
	err := rows.Scan(&m.ID, &m.Direction, &m.Origin, &m.Type, &m.Status,
		&m.Body, &m.Timestamp,
		&m.SentByID, &m.SentByName,
		&m.QuotedID, &m.EditedAt, &m.DeletedAt,
		&mediaID, &md.MimeType, &md.FileName,
		&md.Size, &md.Duration, &md.Width, &md.Height, &mdStatus)
	if err != nil {
		return store.ThreadMessage{}, err
	}
	if mediaID != nil {
		md.ID = *mediaID
		if mdStatus != nil {
			md.Status = *mdStatus
		}
		m.Media = &md
	}
	return m, nil
}

// Thread devuelve una ventana de mensajes de una conversación.
func (r *Read) Thread(ctx context.Context, q store.ThreadQuery) ([]store.ThreadMessage, error) {
	if !isUUID(q.ConversationID) {
		return []store.ThreadMessage{}, nil
	}
	if q.AroundID != "" {
		return r.threadAround(ctx, q)
	}
	return r.threadBefore(ctx, q)
}

// threadBefore es el scroll normal: del cursor hacia atrás en el tiempo.
//
// El keyset compara (wa_timestamp, id), que es la clave de messages_thread_idx.
// Se ordena por wa_timestamp —el instante que puso WhatsApp— y no por created_at:
// un webhook reintentado 40 s después no debe reordenar la conversación.
func (r *Read) threadBefore(ctx context.Context, q store.ThreadQuery) ([]store.ThreadMessage, error) {
	where := []string{"m.conversation_id = $1"}
	args := []any{q.ConversationID}

	if !q.Before.IsZero() {
		args = append(args, q.Before.At, q.Before.ID)
		where = append(where, fmt.Sprintf(
			"(m.wa_timestamp, m.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, q.Limit+1)

	sql := `SELECT ` + threadColumns + threadFrom +
		` WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY m.wa_timestamp DESC, m.id DESC` +
		fmt.Sprintf(` LIMIT $%d`, len(args))

	return r.queryThread(ctx, sql, args...)
}

// threadAround devuelve una ventana centrada en un mensaje.
//
// Existe para los resultados de búsqueda: hacer clic en un resultado tiene que
// abrir el hilo EN ese mensaje. Sin esto, el clic te deja al final del chat y
// encontrar lo que buscabas exige scrollear a mano.
//
// Se resuelve con dos consultas simétricas unidas: la mitad anterior en orden
// descendente y la posterior en ascendente, cada una apoyada en el mismo índice.
// Intentarlo con un OFFSET calculado sería frágil y mucho más caro.
func (r *Read) threadAround(ctx context.Context, q store.ThreadQuery) ([]store.ThreadMessage, error) {
	if !isUUID(q.AroundID) {
		return []store.ThreadMessage{}, nil
	}
	half := q.Limit / 2
	if half < 1 {
		half = 1
	}

	const sql = `
		WITH anchor AS (
		    SELECT wa_timestamp, id FROM messages
		     WHERE id = $2 AND conversation_id = $1
		),
		anteriores AS (
		    SELECT m.id FROM messages m, anchor a
		     WHERE m.conversation_id = $1
		       AND (m.wa_timestamp, m.id) < (a.wa_timestamp, a.id)
		     ORDER BY m.wa_timestamp DESC, m.id DESC
		     LIMIT $3
		),
		posteriores AS (
		    SELECT m.id FROM messages m, anchor a
		     WHERE m.conversation_id = $1
		       AND (m.wa_timestamp, m.id) >= (a.wa_timestamp, a.id)
		     ORDER BY m.wa_timestamp ASC, m.id ASC
		     LIMIT $3
		),
		ventana AS (
		    SELECT id FROM anteriores
		    UNION ALL
		    SELECT id FROM posteriores
		)
		SELECT ` + threadColumns + threadFrom + `
		 WHERE m.id IN (SELECT id FROM ventana)
		 ORDER BY m.wa_timestamp DESC, m.id DESC`

	return r.queryThread(ctx, sql, q.ConversationID, q.AroundID, half)
}

func (r *Read) queryThread(ctx context.Context, sql string, args ...any) ([]store.ThreadMessage, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("leyendo el hilo: %w", err)
	}
	defer rows.Close()

	msgs := []store.ThreadMessage{}
	for rows.Next() {
		m, err := scanThreadMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
