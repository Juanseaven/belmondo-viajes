package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// CloseControl cierra un renglón y guarda con qué se cerró.
//
// La evidencia va en su propia fila y no en una columna del control: reabrir no
// puede borrar que alguna vez se cerró, ni con qué.
func (s *Store) CloseControl(ctx context.Context, c store.CloseControl) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var closing domain.Closing
	var customerID, tripID string
	var derives *string
	err = tx.QueryRow(ctx,
		`SELECT c.closing, t.customer_id, t.id, c.derives
		   FROM controls c JOIN trips t ON t.id = c.trip_id
		  WHERE c.id = $1 FOR UPDATE OF c`, c.ControlID).Scan(&closing, &customerID, &tripID, &derives)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrControlInexistente
	}
	if err != nil {
		return fmt.Errorf("leyendo el control: %w", err)
	}

	if closing == domain.ClosingDerivado {
		return store.ErrDerivadoNoSeCierraAMano
	}
	// Un cliente sin chat no puede recibir el mensaje que prueba el control: se
	// marca a mano y queda quién y cuándo, como un manual. Con chat, el mensaje
	// sigue siendo la prueba.
	if closing == domain.ClosingAccion && c.MessageID == "" {
		hasChat, err := customerHasChat(ctx, tx, customerID)
		if err != nil {
			return err
		}
		if hasChat {
			return store.ErrEvidenciaRequerida
		}
	}

	if c.MessageID != "" {
		if err := checkEvidence(ctx, tx, c.MessageID, customerID); err != nil {
			return err
		}
	}

	// El aviso de requisitos migratorios guarda qué avisó: con eso, un
	// requisito nuevo reabre el control. Y no cierra con algo por verificar.
	var notified []byte
	if derives != nil && *derives == string(domain.DerivaRequisitos) {
		if notified, err = notifiedRequirements(ctx, tx, tripID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE controls SET status = 'cerrado' WHERE id = $1`, c.ControlID); err != nil {
		return fmt.Errorf("cerrando el control: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO control_evidence (control_id, closed_by, message_id, notified)
		 VALUES ($1, $2, nullif($3, '')::uuid, $4)`,
		c.ControlID, c.UserID, c.MessageID, notified); err != nil {
		return fmt.Errorf("guardando la evidencia: %w", err)
	}

	return tx.Commit(ctx)
}

// hasChatSQL dice si el cliente tiene WhatsApp: algún contacto vinculado con
// una conversación individual. Un contacto vinculado sin chat (el alias que deja
// la fusión de LID, uno que llegó solo por el directorio) no alcanza.
const hasChatSQL = `EXISTS (
	SELECT 1 FROM customer_contacts hc
	  JOIN conversations hv ON hv.contact_id = hc.contact_id
	 WHERE hc.customer_id = %s AND NOT hv.is_group)`

func customerHasChat(ctx context.Context, q querier, customerID string) (bool, error) {
	var has bool
	if err := q.QueryRow(ctx, `SELECT `+fmt.Sprintf(hasChatSQL, "$1"), customerID).Scan(&has); err != nil {
		return false, fmt.Errorf("buscando el chat del cliente: %w", err)
	}
	return has, nil
}

// checkEvidence verifica que el mensaje le hable a este cliente.
//
// Sin esta comprobación la evidencia sería un uuid cualquiera: el control diría
// que se avisó de la visa enlazando una conversación con otra persona. El camino
// —mensaje, conversación, contacto, cliente— es el mismo puente que permite que
// un cliente tenga varios números.
func checkEvidence(ctx context.Context, tx pgx.Tx, messageID, customerID string) error {
	var propio bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1
		       FROM messages m
		       JOIN conversations cv ON cv.id = m.conversation_id
		       JOIN customer_contacts cc ON cc.contact_id = cv.contact_id
		      WHERE m.id = $1 AND cc.customer_id = $2)`,
		messageID, customerID).Scan(&propio)
	if err != nil {
		return fmt.Errorf("verificando la evidencia: %w", err)
	}
	if !propio {
		return store.ErrEvidenciaAjena
	}
	return nil
}

// ReopenControl vuelve a dejar pendiente un renglón, conservando su historia.
//
// Un derivado no se reabre por acá: lo gobierna el dato, y forzarlo dejaría el
// control mintiendo hasta el próximo recálculo.
func (s *Store) ReopenControl(ctx context.Context, controlID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE controls SET status = 'abierto'
		  WHERE id = $1 AND closing <> 'derivado'`, controlID)
	if err != nil {
		return fmt.Errorf("reabriendo el control: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrControlInexistente
	}
	return nil
}
