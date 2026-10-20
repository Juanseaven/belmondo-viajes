package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

func (s *Store) Customer(ctx context.Context, customerID string) (store.Customer, error) {
	var c store.Customer
	err := s.pool.QueryRow(ctx,
		`SELECT cu.id, cu.name, coalesce(cu.email, ''), coalesce(cu.phone_number, ''),
		        coalesce(cu.document_type, ''), coalesce(cu.document_number, ''),
		        cu.customer_since,
		        coalesce(array_agg(cc.contact_id::text ORDER BY cc.linked_at)
		                 FILTER (WHERE cc.contact_id IS NOT NULL), '{}')
		   FROM customers cu
		   LEFT JOIN customer_contacts cc ON cc.customer_id = cu.id
		  WHERE cu.id = $1 GROUP BY cu.id`, customerID,
	).Scan(&c.ID, &c.Name, &c.Email, &c.PhoneNumber, &c.DocumentType,
		&c.DocumentNumber, &c.CustomerSince, &c.ContactIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Customer{}, store.ErrNotFound
	}
	if err != nil {
		return store.Customer{}, fmt.Errorf("leyendo el cliente: %w", err)
	}
	return c, nil
}

func (s *Store) UpdateCustomer(ctx context.Context, customerID string, patch store.CustomerPatch) (store.Customer, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Customer{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var c store.Customer
	err = tx.QueryRow(ctx,
		`UPDATE customers SET
		   name = coalesce($2, name),
		   email = CASE WHEN $3::text IS NULL THEN email ELSE nullif($3, '') END,
		   phone_number = CASE WHEN $4::text IS NULL THEN phone_number ELSE nullif($4, '') END,
		   document_type = CASE WHEN $5::text IS NULL THEN document_type ELSE nullif($5, '') END,
		   document_number = CASE WHEN $6::text IS NULL THEN document_number ELSE nullif($6, '') END,
		   customer_since = CASE WHEN $7::text IS NULL THEN customer_since ELSE $7::date END
		 WHERE id = $1
		 RETURNING id, name, coalesce(email, ''), coalesce(phone_number, ''),
		           coalesce(document_type, ''), coalesce(document_number, ''), customer_since`,
		customerID, patch.Name, patch.Email, patch.PhoneNumber, patch.DocumentType,
		patch.DocumentNumber, patch.CustomerSince,
	).Scan(&c.ID, &c.Name, &c.Email, &c.PhoneNumber, &c.DocumentType,
		&c.DocumentNumber, &c.CustomerSince)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Customer{}, store.ErrNotFound
	}
	if err != nil {
		return store.Customer{}, fmt.Errorf("actualizando el cliente: %w", err)
	}

	rows, err := tx.Query(ctx, `SELECT id::text FROM trips WHERE customer_id = $1`, customerID)
	if err != nil {
		return store.Customer{}, fmt.Errorf("listando viajes del cliente: %w", err)
	}
	tripIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return store.Customer{}, fmt.Errorf("armando viajes del cliente: %w", err)
	}
	for _, tripID := range tripIDs {
		if err := refreshDerived(ctx, tx, tripID); err != nil {
			return store.Customer{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Customer{}, err
	}
	return s.Customer(ctx, customerID)
}

func (s *Store) LinkContact(ctx context.Context, customerID, contactID, userID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO customer_contacts (customer_id, contact_id, linked_by) VALUES ($1, $2, $3)`,
		customerID, contactID, userID)
	if err != nil {
		return writeError("vinculando el contacto", err)
	}
	return nil
}
