package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
)

// Contacts persists WhatsApp's directory independently from conversations.
// HistorySync sends names and phone-to-LID mappings in separate chunks.
type Contacts struct{ db *pgxpool.Pool }

func NewContacts(db *pgxpool.Pool) *Contacts { return &Contacts{db: db} }

func (c *Contacts) UpsertDirectory(ctx context.Context, instanceName string, contacts []domain.ContactIdentity) error {
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Primero la línea, como en saveMessage: el borrado en cascada sigue ese
	// mismo orden y así no puede cruzarse con la agenda.
	var instanceID string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM instances WHERE evolution_name = $1 FOR KEY SHARE`, instanceName,
	).Scan(&instanceID); err != nil {
		return fmt.Errorf("resolviendo línea %q: %w", instanceName, err)
	}

	const upsertGlobal = `
		INSERT INTO contacts (wa_jid, phone_number, push_name, lid_jid)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''))
		ON CONFLICT (wa_jid) DO UPDATE SET
			phone_number = coalesce(contacts.phone_number, excluded.phone_number),
			push_name = coalesce(nullif(excluded.push_name, ''), contacts.push_name),
			lid_jid = coalesce(nullif(excluded.lid_jid, ''), contacts.lid_jid)
		RETURNING id`
	const upsertLine = `
		INSERT INTO contact_directory (
			instance_id, wa_jid, lid_jid, first_name, full_name, push_name, business_name
		) VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''))
		ON CONFLICT (instance_id, wa_jid) DO UPDATE SET
			lid_jid = coalesce(excluded.lid_jid, contact_directory.lid_jid),
			first_name = coalesce(excluded.first_name, contact_directory.first_name),
			full_name = coalesce(excluded.full_name, contact_directory.full_name),
			push_name = coalesce(excluded.push_name, contact_directory.push_name),
			business_name = coalesce(excluded.business_name, contact_directory.business_name),
			updated_at = now()`

	// La agenda llega de varias fuentes a la vez (recuperación de la línea, cada
	// lote de historial), cada una en su orden. Recorrerla siempre por JID hace
	// que dos transacciones bloqueen los contactos en el mismo orden y no se
	// crucen en un deadlock.
	contacts = slices.Clone(contacts)
	slices.SortFunc(contacts, func(a, b domain.ContactIdentity) int {
		return strings.Compare(a.PhoneJID, b.PhoneJID)
	})
	for _, contact := range contacts {
		if contact.PhoneJID == "" {
			continue
		}
		phone := ""
		if strings.HasSuffix(contact.PhoneJID, "@s.whatsapp.net") {
			phone = strings.TrimSuffix(contact.PhoneJID, "@s.whatsapp.net")
		}
		pushName := contact.PushName
		if pushName == "" {
			pushName = contact.Name
		}
		var canonicalID string
		if err := tx.QueryRow(ctx, upsertGlobal,
			contact.PhoneJID, phone, pushName, contact.LIDJID).Scan(&canonicalID); err != nil {
			return fmt.Errorf("guardando contacto %q: %w", contact.PhoneJID, err)
		}
		if err := mergeLIDContact(ctx, tx, canonicalID, contact.PhoneJID, contact.LIDJID); err != nil {
			return fmt.Errorf("fusionando identidad LID %q: %w", contact.LIDJID, err)
		}
		if _, err := tx.Exec(ctx, upsertLine, instanceID, contact.PhoneJID, contact.LIDJID,
			contact.FirstName, contact.FullName, pushName, contact.BusinessName); err != nil {
			return fmt.Errorf("guardando contacto %q en la agenda de %q: %w", contact.PhoneJID, instanceName, err)
		}
	}

	return tx.Commit(ctx)
}

type conversationAlias struct {
	id                   string
	instanceID           string
	lastMessageAt        *time.Time
	lastMessagePreview   *string
	lastMessageType      *string
	lastMessageDirection *string
	unreadCount          int
	isArchived           bool
	createdAt            time.Time
}

// mergeLIDContact moves every conversation attached to the opaque LID onto the
// phone contact. The alias contact itself is deliberately retained: other
// modules may reference it, while no inbox row remains attached to it.
func mergeLIDContact(ctx context.Context, tx pgx.Tx, canonicalID, phoneJID, lidJID string) error {
	if canonicalID == "" || phoneJID == "" || !strings.HasSuffix(lidJID, "@lid") {
		return nil
	}

	var aliasID string
	err := tx.QueryRow(ctx,
		`SELECT id FROM contacts WHERE wa_jid=$1 AND id<>$2`, lidJID, canonicalID,
	).Scan(&aliasID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE contacts canonical
		   SET display_name = coalesce(canonical.display_name, alias.display_name),
		       push_name = coalesce(canonical.push_name, alias.push_name),
		       profile_pic_url = coalesce(canonical.profile_pic_url, alias.profile_pic_url),
		       notes = coalesce(canonical.notes, alias.notes),
		       is_blocked = canonical.is_blocked OR alias.is_blocked
		  FROM contacts alias
		 WHERE canonical.id=$1 AND alias.id=$2`, canonicalID, aliasID); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, instance_id, last_message_at, last_message_preview,
		       last_message_type::text, last_message_direction::text,
		       unread_count, is_archived, created_at
		  FROM conversations
		 WHERE contact_id=$1
		 FOR UPDATE`, aliasID)
	if err != nil {
		return err
	}
	var aliases []conversationAlias
	for rows.Next() {
		var item conversationAlias
		if err := rows.Scan(&item.id, &item.instanceID, &item.lastMessageAt,
			&item.lastMessagePreview, &item.lastMessageType, &item.lastMessageDirection,
			&item.unreadCount, &item.isArchived, &item.createdAt); err != nil {
			rows.Close()
			return err
		}
		aliases = append(aliases, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, alias := range aliases {
		var targetID string
		err := tx.QueryRow(ctx, `
			SELECT id FROM conversations
			 WHERE instance_id=$1 AND contact_id=$2
			 FOR UPDATE`, alias.instanceID, canonicalID).Scan(&targetID)
		if errors.Is(err, pgx.ErrNoRows) {
			if _, err := tx.Exec(ctx,
				`UPDATE messages SET contact_id=$2 WHERE conversation_id=$1`, alias.id, canonicalID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE conversations
				   SET contact_id=$2, remote_jid=$3
				 WHERE id=$1`, alias.id, canonicalID, phoneJID); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			UPDATE messages
			   SET conversation_id=$2, contact_id=$3
			 WHERE conversation_id=$1`, alias.id, targetID, canonicalID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE outbound_messages SET conversation_id=$2 WHERE conversation_id=$1`, alias.id, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE conversations
			   SET remote_jid=$2,
			       last_message_preview = CASE
			         WHEN last_message_at IS NULL OR ($3::timestamptz IS NOT NULL AND $3 >= last_message_at) THEN $4
			         ELSE last_message_preview END,
			       last_message_type = CASE
			         WHEN last_message_at IS NULL OR ($3::timestamptz IS NOT NULL AND $3 >= last_message_at) THEN $5::message_type
			         ELSE last_message_type END,
			       last_message_direction = CASE
			         WHEN last_message_at IS NULL OR ($3::timestamptz IS NOT NULL AND $3 >= last_message_at) THEN $6::message_direction
			         ELSE last_message_direction END,
			       last_message_at = CASE
			         WHEN last_message_at IS NULL OR ($3::timestamptz IS NOT NULL AND $3 >= last_message_at) THEN $3
			         ELSE last_message_at END,
			       unread_count = unread_count + $7,
			       is_archived = is_archived AND $8,
			       created_at = least(created_at, $9)
			 WHERE id=$1`, targetID, phoneJID, alias.lastMessageAt, alias.lastMessagePreview,
			alias.lastMessageType, alias.lastMessageDirection, alias.unreadCount,
			alias.isArchived, alias.createdAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM conversations WHERE id=$1`, alias.id); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contacts) SetGroupName(ctx context.Context, instanceName, groupJID, name string) error {
	if !strings.HasSuffix(groupJID, "@g.us") || strings.TrimSpace(name) == "" {
		return nil
	}
	var known bool
	if err := c.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM instances WHERE evolution_name=$1)`, instanceName,
	).Scan(&known); err != nil {
		return err
	}
	if !known {
		return fmt.Errorf("línea %q no encontrada", instanceName)
	}
	_, err := c.db.Exec(ctx, `
		INSERT INTO contacts (wa_jid, push_name)
		VALUES ($1, $2)
		ON CONFLICT (wa_jid) DO UPDATE SET push_name=excluded.push_name`,
		groupJID, strings.TrimSpace(name))
	return err
}
