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
