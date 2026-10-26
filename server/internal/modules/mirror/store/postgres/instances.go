package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

type Instances struct{ db *pgxpool.Pool }

func NewInstances(db *pgxpool.Pool) *Instances { return &Instances{db: db} }

func (i *Instances) IDByName(ctx context.Context, evolutionName string) (string, error) {
	var id string
	err := i.db.QueryRow(ctx,
		`SELECT id FROM instances WHERE evolution_name = $1`, evolutionName).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", store.ErrNotFound
	}
	return id, err
}

// UpdateConnection refleja los cambios de vinculación de una línea.
//
// connected_at solo se fija en la transición hacia 'open': así queda constancia
// de cuándo se vinculó por última vez, sin pisarla con cada evento repetido.
func (i *Instances) UpdateConnection(ctx context.Context, evolutionName, state, phoneJID string) error {
	const q = `
		UPDATE instances
		   SET status       = $2::instance_status,
		       phone_jid    = COALESCE(NULLIF($3,''), phone_jid),
		       connected_at = CASE WHEN $2 = 'open' AND status <> 'open'
		                          THEN now() ELSE connected_at END,
		       last_seen_at = now()
		 WHERE evolution_name = $1`
	_, err := i.db.Exec(ctx, q, evolutionName, normalizeState(state), phoneJID)
	return err
}

func (i *Instances) Touch(ctx context.Context, evolutionName string, at time.Time) error {
	_, err := i.db.Exec(ctx,
		`UPDATE instances SET last_seen_at = $2 WHERE evolution_name = $1`, evolutionName, at)
	return err
}

// Create registra una línea antes de que exista en Evolution.
//
// El estado inicial es 'created': todavía no hay teléfono vinculado, y así el
// panel la muestra como pendiente de escanear.
func (i *Instances) Create(ctx context.Context, evolutionName, ownerLabel, displayName string) (string, error) {
	var id string
	err := i.db.QueryRow(ctx,
		`INSERT INTO instances (evolution_name, owner_label, display_name, status)
		 VALUES ($1, $2, $3, 'created') RETURNING id`,
		evolutionName, ownerLabel, nullIfEmpty(displayName)).Scan(&id)
	if isUniqueViolation(err) {
		return "", store.ErrDuplicate
	}
	return id, err
}

// PendingByLabel usa el mismo criterio que wasConnected en la lectura de líneas:
// sin connected_at ni phone_jid, la línea nunca se vinculó.
func (i *Instances) PendingByLabel(ctx context.Context, ownerLabel string) (store.PendingLine, bool, error) {
	var line store.PendingLine
	err := i.db.QueryRow(ctx, `
		SELECT id::text, evolution_name
		  FROM instances
		 WHERE owner_label = $1
		   AND status <> 'open'
		   AND connected_at IS NULL
		   AND coalesce(phone_jid, '') = ''
		 ORDER BY created_at DESC
		 LIMIT 1`, ownerLabel).Scan(&line.ID, &line.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.PendingLine{}, false, nil
	}
	if err != nil {
		return store.PendingLine{}, false, err
	}
	return line, true, nil
}

func (i *Instances) NameByID(ctx context.Context, id string) (string, error) {
	if !isUUID(id) {
		return "", store.ErrNotFound
	}
	var name string
	err := i.db.QueryRow(ctx,
		`SELECT evolution_name FROM instances WHERE id = $1`, id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", store.ErrNotFound
	}
	return name, err
}

// Delete borra la línea y todos sus datos dependientes dentro de una sola
// transacción. Los contactos son globales: solo se eliminan si ninguna otra
// línea conserva una conversación o una entrada de agenda con ellos.
func (i *Instances) Delete(ctx context.Context, id string) error {
	if !isUUID(id) {
		return store.ErrNotFound
	}

	tx, err := i.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var name string
	if err := tx.QueryRow(ctx,
		`SELECT evolution_name FROM instances WHERE id = $1`, id).Scan(&name); errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	} else if err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT DISTINCT candidate.contact_id::text
		  FROM (
		        SELECT contact_id
		          FROM conversations
		         WHERE instance_id = $1
		        UNION
		        SELECT contact.id
		          FROM contact_directory directory
		          JOIN contacts contact
		            ON contact.wa_jid = directory.wa_jid
		            OR contact.wa_jid = directory.lid_jid
		            OR contact.lid_jid = directory.wa_jid
		            OR contact.lid_jid = directory.lid_jid
		         WHERE directory.instance_id = $1
		       ) candidate`, id)
	if err != nil {
		return err
	}
	var contactIDs []string
	for rows.Next() {
		var contactID string
		if err := rows.Scan(&contactID); err != nil {
			rows.Close()
			return err
		}
		contactIDs = append(contactIDs, contactID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if _, err := tx.Exec(ctx, `DELETE FROM instances WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM webhook_events WHERE instance_name = $1`, name); err != nil {
		return err
	}
	if len(contactIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM contacts c
			 WHERE c.id = ANY($1::uuid[])
			   AND NOT EXISTS (
			       SELECT 1 FROM conversations remaining WHERE remaining.contact_id = c.id
			   )
			   AND NOT EXISTS (
			       SELECT 1
			         FROM contact_directory remaining
			        WHERE remaining.wa_jid = c.wa_jid
			           OR remaining.lid_jid = c.wa_jid
			           OR remaining.wa_jid = c.lid_jid
			           OR remaining.lid_jid = c.lid_jid
			   )`, contactIDs); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// normalizeState mapea los estados de Baileys al enum instance_status. Cualquier
// valor desconocido cae en 'connecting' en vez de romper el INSERT: un estado
// nuevo de WhatsApp no debe tumbar la ingesta.
func normalizeState(state string) string {
	switch state {
	case "open", "close", "connecting":
		return state
	default:
		return "connecting"
	}
}
