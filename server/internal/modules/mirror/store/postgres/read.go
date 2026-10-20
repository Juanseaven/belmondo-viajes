package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// Read implementa las consultas que alimentan el panel.
type Read struct{ db *pgxpool.Pool }

func NewRead(db *pgxpool.Pool) *Read { return &Read{db: db} }

var _ store.ReadStore = (*Read)(nil)

// Lines lista las líneas con su contador de chats sin leer.
//
// El LEFT JOIN filtra por unread_count > 0 dentro de la condición, no en un
// WHERE de fuera. Dos razones: un WHERE convertiría el LEFT JOIN en INNER y
// haría desaparecer las líneas sin pendientes; y así la unión se apoya en
// conversations_unread_idx, que solo indexa las filas con algo pendiente. Sin ese
// índice esto escanea la tabla entera de conversaciones en cada carga del panel.
func (r *Read) Lines(ctx context.Context) ([]store.Line, error) {
	const q = `
		SELECT i.id, i.evolution_name, i.owner_label,
		       coalesce(i.display_name, ''), coalesce(i.phone_number, ''),
		       i.status, i.is_active,
		       (i.connected_at IS NOT NULL OR coalesce(i.phone_jid, '') <> ''),
		       i.last_seen_at,
		       count(c.id)::int
		  FROM instances i
		  LEFT JOIN conversations c
		         ON c.instance_id = i.id
		        AND c.unread_count > 0
		        AND NOT c.is_archived
		 WHERE i.is_active
		 GROUP BY i.id
		 ORDER BY i.owner_label`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listando líneas: %w", err)
	}
	defer rows.Close()

	lines := []store.Line{}
	for rows.Next() {
		var l store.Line
		if err := rows.Scan(&l.ID, &l.Name, &l.Label, &l.DisplayName, &l.PhoneNumber,
			&l.Status, &l.IsActive, &l.WasConnected, &l.LastSeenAt, &l.Unread); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

// inboxColumns es el proyectado de la bandeja. El nombre a mostrar se resuelve en
// SQL con coalesce y no en Go: así el ORDER BY y el filtro de búsqueda operan
// sobre el mismo valor que ve el usuario.
const inboxColumns = `
	c.id, c.instance_id, i.owner_label, c.contact_id,
	coalesce(
		nullif(ct.display_name, ''),
		CASE WHEN NOT c.is_group THEN coalesce(
			nullif(cd.full_name, ''), nullif(cd.first_name, ''),
			nullif(cd.business_name, ''), nullif(cd.push_name, '')
		) END,
		nullif(ct.push_name, ''), ct.phone_number, ct.wa_jid
	),
	coalesce(ct.phone_number, ''), coalesce(ct.profile_pic_url, ''),
	c.is_group, c.last_message_at, coalesce(c.last_message_preview, ''),
	coalesce(c.last_message_type::text, ''), c.unread_count,
	coalesce(c.last_message_direction = 'inbound', false)`

const inboxFrom = `
	FROM conversations c
	JOIN instances i ON i.id = c.instance_id
	JOIN contacts  ct ON ct.id = c.contact_id
	LEFT JOIN contact_directory cd
	       ON cd.instance_id = c.instance_id AND cd.wa_jid = ct.wa_jid`

func scanInbox(row pgx.Row) (store.InboxItem, error) {
	var it store.InboxItem
	err := row.Scan(&it.ID, &it.InstanceID, &it.InstanceLbl, &it.ContactID,
		&it.ContactName, &it.PhoneNumber, &it.AvatarURL,
		&it.IsGroup, &it.LastMessageAt, &it.Preview, &it.LastType, &it.Unread,
		&it.NeedsReply)
	return it, err
}

// Inbox devuelve una página de la bandeja, de la conversación con actividad más
// reciente hacia atrás.
//
// El keyset se compara con la tupla (last_message_at, id) porque es exactamente
// la clave de conversations_inbox_idx. Comparar tuplas y no columnas sueltas
// permite a Postgres resolverlo con un solo descenso por el índice.
func (r *Read) Inbox(ctx context.Context, q store.InboxQuery) ([]store.InboxItem, error) {
	where := []string{"NOT c.is_archived", "i.is_active"}
	var args []any

	if q.InstanceID != "" {
		if !isUUID(q.InstanceID) {
			return []store.InboxItem{}, nil // filtro por una línea que no existe
		}
		args = append(args, q.InstanceID)
		where = append(where, fmt.Sprintf("c.instance_id = $%d", len(args)))
	}

	if q.Search != "" {
		// ILIKE sobre el índice de trigramas de contacts. Es filtro de navegación:
		// encontrar un chat entre cientos, no buscar dentro de lo conversado.
		args = append(args, "%"+q.Search+"%")
		where = append(where, fmt.Sprintf(
			`(coalesce(ct.display_name,'') ILIKE $%d
			  OR coalesce(cd.full_name,'') ILIKE $%d
			  OR coalesce(cd.first_name,'') ILIKE $%d
			  OR coalesce(cd.business_name,'') ILIKE $%d
			  OR coalesce(cd.push_name,'') ILIKE $%d
			  OR coalesce(ct.push_name,'') ILIKE $%d
			  OR coalesce(ct.phone_number,'') ILIKE $%d)`,
			len(args), len(args), len(args), len(args), len(args), len(args), len(args)))
	}

	if q.UnreadOnly {
		where = append(where, "c.unread_count > 0")
	}

	if !q.Cursor.IsZero() {
		args = append(args, q.Cursor.At, q.Cursor.ID)
		where = append(where, fmt.Sprintf(
			"(c.last_message_at, c.id) < ($%d, $%d)", len(args)-1, len(args)))
	}

	// limit+1: la fila extra revela si hay página siguiente sin pagar un COUNT.
	args = append(args, q.Limit+1)

	sql := `SELECT ` + inboxColumns + inboxFrom +
		` WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY c.last_message_at DESC NULLS LAST, c.id DESC` +
		fmt.Sprintf(` LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("listando la bandeja: %w", err)
	}
	defer rows.Close()

	items := []store.InboxItem{}
	for rows.Next() {
		it, err := scanInbox(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *Read) Conversation(ctx context.Context, id string) (store.InboxItem, error) {
	if !isUUID(id) {
		return store.InboxItem{}, store.ErrNotFound
	}
	it, err := scanInbox(r.db.QueryRow(ctx,
		`SELECT `+inboxColumns+inboxFrom+` WHERE c.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.InboxItem{}, store.ErrNotFound
	}
	return it, err
}

// MarkRead pone el contador a cero.
//
// La guarda `unread_count > 0` evita escribir cuando no hay nada que cambiar: el
// frontend llama a esto cada vez que se abre un hilo, y sin la guarda cada
// apertura genera una escritura y dispara el trigger de updated_at, moviendo la
// fila en disco sin motivo.
func (r *Read) MarkRead(ctx context.Context, conversationID string) error {
	if !isUUID(conversationID) {
		return store.ErrNotFound
	}
	_, err := r.db.Exec(ctx,
		`UPDATE conversations SET unread_count = 0
		  WHERE id = $1 AND unread_count > 0`, conversationID)
	return err
}
