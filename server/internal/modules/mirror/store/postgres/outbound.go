package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

type Outbound struct{ db *pgxpool.Pool }

func NewOutbound(db *pgxpool.Pool) *Outbound { return &Outbound{db: db} }

var _ store.OutboundStore = (*Outbound)(nil)

// Target trae en una consulta todo lo necesario para enviar: el nombre de la
// instancia en Evolution y el JID del destinatario.
//
// `remote_jid` está desnormalizado en conversations justamente para esto: enviar
// no debería pagar un JOIN contra contacts.
func (o *Outbound) Target(ctx context.Context, conversationID string) (store.SendTarget, error) {
	if !isUUID(conversationID) {
		return store.SendTarget{}, store.ErrNotFound
	}

	const q = `
		SELECT c.id, c.instance_id, i.evolution_name, c.remote_jid,
		       i.status::text, i.is_active
		  FROM conversations c
		  JOIN instances i ON i.id = c.instance_id
		 WHERE c.id = $1`

	var t store.SendTarget
	err := o.db.QueryRow(ctx, q, conversationID).Scan(
		&t.ConversationID, &t.InstanceID, &t.InstanceName,
		&t.RemoteJID, &t.LineStatus, &t.LineActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.SendTarget{}, store.ErrNotFound
	}
	return t, err
}

func (o *Outbound) QuoteTarget(ctx context.Context, conversationID, messageID string) (store.QuoteTarget, error) {
	if !isUUID(conversationID) || !isUUID(messageID) {
		return store.QuoteTarget{}, store.ErrNotFound
	}

	const q = `
		SELECT m.wa_message_id,
		       CASE WHEN m.direction = 'outbound'
		            THEN coalesce(i.phone_jid, '')
		            ELSE coalesce(m.sender_jid, c.remote_jid, '')
		       END
		  FROM messages m
		  JOIN conversations c ON c.id = m.conversation_id
		  JOIN instances i ON i.id = m.instance_id
		 WHERE m.id = $2 AND c.id = $1`

	var target store.QuoteTarget
	err := o.db.QueryRow(ctx, q, conversationID, messageID).Scan(
		&target.WAMessageID, &target.Participant)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.QuoteTarget{}, store.ErrNotFound
	}
	return target, err
}

const outboundColumns = `
	id, conversation_id, instance_id, user_id, idempotency_key,
	type::text, coalesce(body, ''), coalesce(media_storage_key, ''),
	coalesce(wa_message_id, ''), status::text, coalesce(error, ''),
	sent_at, created_at`

func scanOutbound(row pgx.Row) (*store.Outbound, error) {
	var o store.Outbound
	err := row.Scan(&o.ID, &o.ConversationID, &o.InstanceID, &o.UserID, &o.IdempotencyKey,
		&o.Type, &o.Body, &o.StorageKey,
		&o.WAMessageID, &o.Status, &o.Error, &o.SentAt, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// Create inserta la intención de envío.
//
// ON CONFLICT DO NOTHING más un SELECT de rescate: si dos peticiones con la misma
// clave llegan a la vez, una inserta y la otra no ve fila devuelta, así que relee.
// Delegar el desempate en el índice único evita la carrera que tendría un
// "comprobar y después insertar".
func (o *Outbound) Create(ctx context.Context, in *store.Outbound) (*store.Outbound, bool, error) {
	const insert = `
		INSERT INTO outbound_messages
		    (conversation_id, instance_id, user_id, idempotency_key,
		     type, body, media_storage_key, status)
		VALUES ($1, $2, $3, $4, $5::message_type, $6, $7, 'pending')
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING ` + outboundColumns

	created, err := scanOutbound(o.db.QueryRow(ctx, insert,
		in.ConversationID, in.InstanceID, in.UserID, in.IdempotencyKey,
		in.Type, nullIfEmpty(in.Body), nullIfEmpty(in.StorageKey)))
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("registrando el envío: %w", err)
	}

	// La clave ya existía: es un reintento o un doble clic. Se devuelve el envío
	// original y quien llama NO vuelve a mandar a Evolution.
	existing, err := scanOutbound(o.db.QueryRow(ctx,
		`SELECT `+outboundColumns+` FROM outbound_messages WHERE idempotency_key = $1`,
		in.IdempotencyKey))
	if err != nil {
		return nil, false, fmt.Errorf("releyendo el envío duplicado: %w", err)
	}
	return existing, false, nil
}

func (o *Outbound) MarkSent(ctx context.Context, id, waMessageID string) error {
	_, err := o.db.Exec(ctx,
		`UPDATE outbound_messages
		    SET status = 'sent', wa_message_id = $2, sent_at = now(), error = NULL
		  WHERE id = $1`, id, waMessageID)
	return err
}

func (o *Outbound) MarkFailed(ctx context.Context, id, cause string) error {
	_, err := o.db.Exec(ctx,
		`UPDATE outbound_messages SET status = 'failed', error = $2 WHERE id = $1`,
		id, truncate(cause, 1000))
	return err
}

// UserByWAMessageID recupera el autor de un envío ya confirmado.
//
// Se filtra también por instancia porque el id de WhatsApp solo es único dentro
// de una línea, igual que en la constraint messages_wa_id_uniq.
func (o *Outbound) UserByWAMessageID(ctx context.Context, instanceID, waMessageID string) (string, bool) {
	var userID string
	err := o.db.QueryRow(ctx,
		`SELECT user_id::text FROM outbound_messages
		  WHERE instance_id = $1 AND wa_message_id = $2`,
		instanceID, waMessageID).Scan(&userID)
	if err != nil || userID == "" {
		return "", false
	}
	return userID, true
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
