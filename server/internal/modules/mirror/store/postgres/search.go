package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// MinSearchLen es el mínimo de caracteres para buscar dentro de los mensajes.
//
// No es una regla de usabilidad: el índice es de trigramas, y con menos de tres
// caracteres no hay ni un trigrama completo que consultar. Postgres ignoraría el
// índice y escanearía la tabla entera de mensajes.
const MinSearchLen = 3

// searchTimeout acota lo que puede durar una búsqueda.
//
// La búsqueda por subcadena es el único endpoint del módulo que no escala solo.
// Va bien con términos raros —un código de reserva, un número— y mal con términos
// comunes, donde el índice devuelve un candidato enorme que hay que ordenar por
// fecha. Antes que arriesgar que una búsqueda desafortunada monopolice una
// conexión del pool, se corta y se le pide al usuario que afine.
const searchTimeout = "3s"

// SearchMessages busca dentro del texto conversado.
//
// Devuelve ErrSearchTooShort si el término no llega al mínimo; el handler lo
// traduce a 400.
func (r *Read) SearchMessages(ctx context.Context, q store.SearchQuery) ([]store.SearchHit, error) {
	text := strings.TrimSpace(q.Text)
	if len([]rune(text)) < MinSearchLen {
		return nil, store.ErrSearchTooShort
	}

	// Los mensajes borrados siguen en la base para que el borrado quede
	// registrado, pero no deben aparecer como resultados de búsqueda.
	where := []string{
		"m.body IS NOT NULL",
		"m.deleted_at IS NULL",
		"m.body ILIKE $1",
	}
	args := []any{"%" + text + "%"}

	if q.InstanceID != "" {
		if !isUUID(q.InstanceID) {
			return []store.SearchHit{}, nil
		}
		args = append(args, q.InstanceID)
		where = append(where, fmt.Sprintf("m.instance_id = $%d", len(args)))
	}

	if !q.Cursor.IsZero() {
		args = append(args, q.Cursor.At, q.Cursor.ID)
		where = append(where, fmt.Sprintf(
			"(m.wa_timestamp, m.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, q.Limit+1)

	sql := `
		SELECT m.id, m.conversation_id, m.instance_id, i.owner_label,
		       coalesce(nullif(ct.display_name, ''), nullif(ct.push_name, ''),
		                ct.phone_number, ct.wa_jid),
		       m.body, m.wa_timestamp
		  FROM messages m
		  JOIN instances i  ON i.id = m.instance_id
		  JOIN contacts  ct ON ct.id = m.contact_id
		 WHERE ` + strings.Join(where, " AND ") + `
		 ORDER BY m.wa_timestamp DESC, m.id DESC` +
		fmt.Sprintf(` LIMIT $%d`, len(args))

	// El timeout se fija en la transacción, no en la sesión: el pool reutiliza
	// conexiones y un SET a secas se lo llevaría puesto el siguiente que la use.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("abriendo transacción de búsqueda: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '`+searchTimeout+`'`); err != nil {
		return nil, fmt.Errorf("fijando el timeout de búsqueda: %w", err)
	}

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		if isTimeout(err) {
			return nil, store.ErrSearchTooBroad
		}
		return nil, fmt.Errorf("buscando mensajes: %w", err)
	}
	defer rows.Close()

	hits := []store.SearchHit{}
	for rows.Next() {
		var h store.SearchHit
		var body string
		if err := rows.Scan(&h.MessageID, &h.ConversationID, &h.InstanceID,
			&h.InstanceLbl, &h.ContactName, &body, &h.Timestamp); err != nil {
			return nil, err
		}
		h.Snippet = store.Snippet(body, text)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		if isTimeout(err) {
			return nil, store.ErrSearchTooBroad
		}
		return nil, err
	}
	return hits, nil
}
