package postgres

import (
	"context"
	"fmt"
	"strings"

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

	var instanceID string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM instances WHERE evolution_name = $1`, instanceName,
	).Scan(&instanceID); err != nil {
		return fmt.Errorf("resolviendo línea %q: %w", instanceName, err)
	}

	const upsertGlobal = `
		INSERT INTO contacts (wa_jid, phone_number, push_name, lid_jid)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''))
		ON CONFLICT (wa_jid) DO UPDATE SET
			phone_number = coalesce(contacts.phone_number, excluded.phone_number),
			push_name = coalesce(nullif(excluded.push_name, ''), contacts.push_name),
			lid_jid = coalesce(nullif(excluded.lid_jid, ''), contacts.lid_jid)`
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
		if _, err := tx.Exec(ctx, upsertGlobal,
			contact.PhoneJID, phone, pushName, contact.LIDJID); err != nil {
			return fmt.Errorf("guardando contacto %q: %w", contact.PhoneJID, err)
		}
		if _, err := tx.Exec(ctx, upsertLine, instanceID, contact.PhoneJID, contact.LIDJID,
			contact.FirstName, contact.FullName, pushName, contact.BusinessName); err != nil {
			return fmt.Errorf("guardando contacto %q en la agenda de %q: %w", contact.PhoneJID, instanceName, err)
		}
	}

	return tx.Commit(ctx)
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
