package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juanseaven/whatsapp-mirror/server/internal/domain"
	"github.com/juanseaven/whatsapp-mirror/server/internal/store"
)

type Messages struct{ db *pgxpool.Pool }

func NewMessages(db *pgxpool.Pool) *Messages { return &Messages{db: db} }

// Save persiste un mensaje entrante o saliente.
//
// Todo ocurre en UNA transacción: contacto, conversación, mensaje, contadores y
// fila de media. Si se hiciera por partes, un fallo a mitad dejaría la bandeja
// mostrando un `last_message_at` que no corresponde a ningún mensaje real, y en
// una herramienta de auditoría eso es peor que no mostrar nada.
func (m *Messages) Save(ctx context.Context, msg *domain.Message, sentByUserID string) (*store.PersistedMessage, error) {
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op si ya se hizo Commit

	var instanceID string
	err = tx.QueryRow(ctx,
		`SELECT id FROM instances WHERE evolution_name = $1`, msg.InstanceName,
	).Scan(&instanceID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Una línea que Evolution conoce pero nosotros no. Es un error de
		// aprovisionamiento y debe verse, no tragarse creando filas fantasma.
		return nil, fmt.Errorf("%w: instancia %q", store.ErrNotFound, msg.InstanceName)
	}
	if err != nil {
		return nil, err
	}

	contactID, err := upsertContact(ctx, tx, msg)
	if err != nil {
		return nil, fmt.Errorf("upsert de contacto: %w", err)
	}

	conversationID, err := upsertConversation(ctx, tx, instanceID, contactID, msg)
	if err != nil {
		return nil, fmt.Errorf("upsert de conversación: %w", err)
	}

	var sentBy *string
	if sentByUserID != "" {
		sentBy = &sentByUserID
	}

	// ON CONFLICT DO NOTHING + RETURNING: si la fila ya existía no devuelve nada,
	// y así distinguimos un mensaje nuevo de un reintento sin una consulta extra.
	const insertMsg = `
		INSERT INTO messages (
			conversation_id, instance_id, contact_id, wa_message_id,
			direction, origin, type, status, sent_by_user_id,
			body, quoted_wa_message_id, wa_timestamp
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12)
		ON CONFLICT (instance_id, wa_message_id) DO NOTHING
		RETURNING id`

	var messageID string
	err = tx.QueryRow(ctx, insertMsg,
		conversationID, instanceID, contactID, msg.WAMessageID,
		msg.Direction, msg.Origin, msg.Type, msg.Status, sentBy,
		nullIfEmpty(msg.Body), msg.QuotedID, msg.Timestamp,
	).Scan(&messageID)

	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &store.PersistedMessage{
			ConversationID: conversationID,
			InstanceID:     instanceID,
			ContactID:      contactID,
			IsNew:          false,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("insertando mensaje: %w", err)
	}

	// El preview y el contador se actualizan solo si este mensaje es MÁS RECIENTE
	// que el último registrado. Un webhook reintentado 40 s tarde no debe
	// retroceder la bandeja.
	const bumpConv = `
		UPDATE conversations
		   SET last_message_at      = GREATEST(COALESCE(last_message_at, $2), $2),
		       last_message_preview = CASE WHEN last_message_at IS NULL OR $2 >= last_message_at
		                                   THEN $3 ELSE last_message_preview END,
		       last_message_type    = CASE WHEN last_message_at IS NULL OR $2 >= last_message_at
		                                   THEN $4::message_type ELSE last_message_type END,
		       unread_count         = CASE WHEN $5 = 'inbound'
		                                   THEN unread_count + 1 ELSE unread_count END
		 WHERE id = $1`
	if _, err := tx.Exec(ctx, bumpConv,
		conversationID, msg.Timestamp, preview(msg), msg.Type, msg.Direction,
	); err != nil {
		return nil, fmt.Errorf("actualizando conversación: %w", err)
	}

	var mediaID string
	if msg.Type.HasMedia() && msg.Media != nil {
		const insertMedia = `
			INSERT INTO media_assets (
				message_id, mime_type, file_name, size_bytes, duration_seconds, width, height
			) VALUES ($1, NULLIF($2,''), NULLIF($3,''), NULLIF($4,0), NULLIF($5,0), NULLIF($6,0), NULLIF($7,0))
			RETURNING id`
		if err := tx.QueryRow(ctx, insertMedia,
			messageID, msg.Media.MimeType, msg.Media.FileName, msg.Media.SizeBytes,
			msg.Media.Duration, msg.Media.Width, msg.Media.Height,
		).Scan(&mediaID); err != nil {
			return nil, fmt.Errorf("insertando media_asset: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &store.PersistedMessage{
		MessageID:      messageID,
		ConversationID: conversationID,
		InstanceID:     instanceID,
		ContactID:      contactID,
		MediaAssetID:   mediaID,
		IsNew:          true,
	}, nil
}

func upsertContact(ctx context.Context, tx pgx.Tx, msg *domain.Message) (string, error) {
	// El push_name solo se sobrescribe si viene con valor: un evento sin pushName
	// no debe borrar el nombre que ya conocíamos del cliente.
	const q = `
		INSERT INTO contacts (wa_jid, phone_number, push_name)
		VALUES ($1, $2, NULLIF($3,''))
		ON CONFLICT (wa_jid) DO UPDATE
		   SET push_name = COALESCE(NULLIF(EXCLUDED.push_name,''), contacts.push_name)
		RETURNING id`

	var id string
	err := tx.QueryRow(ctx, q, msg.RemoteJID, phoneFromJID(msg.RemoteJID), msg.PushName).Scan(&id)
	return id, err
}

func upsertConversation(ctx context.Context, tx pgx.Tx, instanceID, contactID string, msg *domain.Message) (string, error) {
	const q = `
		INSERT INTO conversations (instance_id, contact_id, remote_jid, is_group)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (instance_id, contact_id) DO UPDATE
		   SET remote_jid = EXCLUDED.remote_jid
		RETURNING id`

	var id string
	err := tx.QueryRow(ctx, q, instanceID, contactID, msg.RemoteJID, msg.IsGroup).Scan(&id)
	return id, err
}

func (m *Messages) UpdateStatus(ctx context.Context, instanceName, waMessageID string, status domain.MessageStatus) error {
	// El estado solo avanza: sent -> delivered -> read. Los ACK de WhatsApp
	// pueden llegar desordenados y un 'delivered' tardío no debe deshacer un
	// 'read' ya registrado.
	const q = `
		UPDATE messages m
		   SET status = $3
		  FROM instances i
		 WHERE m.instance_id = i.id
		   AND i.evolution_name = $1
		   AND m.wa_message_id = $2
		   AND array_position(ARRAY['pending','sent','delivered','read']::message_status[], $3)
		     > array_position(ARRAY['pending','sent','delivered','read']::message_status[], m.status)`
	_, err := m.db.Exec(ctx, q, instanceName, waMessageID, status)
	return err
}

// MarkDeleted es un soft delete deliberado. Que un vendedor borre un mensaje para
// el cliente no puede borrarlo del registro de auditoría: ese borrado es, en sí
// mismo, información relevante para la dirección.
func (m *Messages) MarkDeleted(ctx context.Context, instanceName, waMessageID string) error {
	const q = `
		UPDATE messages m
		   SET deleted_at = now()
		  FROM instances i
		 WHERE m.instance_id = i.id
		   AND i.evolution_name = $1
		   AND m.wa_message_id = $2
		   AND m.deleted_at IS NULL`
	_, err := m.db.Exec(ctx, q, instanceName, waMessageID)
	return err
}

func phoneFromJID(jid string) string {
	if i := strings.IndexByte(jid, '@'); i > 0 {
		return "+" + jid[:i]
	}
	return ""
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// preview es el texto que ve el admin en la lista de la bandeja.
func preview(msg *domain.Message) string {
	if msg.Body != "" {
		// Por runas, no por bytes: cortar a mitad de un carácter acentuado o de un
		// emoji produce UTF-8 inválido que Postgres rechaza.
		const max = 120
		r := []rune(msg.Body)
		if len(r) > max {
			return string(r[:max]) + "…"
		}
		return msg.Body
	}
	switch msg.Type {
	case domain.TypeImage:
		return "📷 Imagen"
	case domain.TypeAudio:
		return "🎤 Audio"
	case domain.TypeVideo:
		return "🎬 Vídeo"
	case domain.TypeDocument:
		if msg.Media != nil && msg.Media.FileName != "" {
			return "📎 " + msg.Media.FileName
		}
		return "📎 Documento"
	case domain.TypeSticker:
		return "Sticker"
	case domain.TypeLocation:
		return "📍 Ubicación"
	case domain.TypeContact:
		return "👤 Contacto"
	default:
		return ""
	}
}
