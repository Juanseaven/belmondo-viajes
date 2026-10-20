package postgres

import (
	"context"
	"fmt"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

func (s *Store) PutPayment(ctx context.Context, p store.Payment) (store.Payment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Payment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var saved store.Payment
	err = tx.QueryRow(ctx,
		`INSERT INTO payments
		 (trip_id, total_usd, rate, total_ars, method, deposit_usd, deposit_due_at,
		  deposit_paid_at, non_refundable_usd, updated_by)
		 VALUES ($1, $2, $3, $4, nullif($5, ''), $6, $7, $8, $9, $10)
		 ON CONFLICT (trip_id) DO UPDATE SET
		   total_usd = excluded.total_usd, rate = excluded.rate,
		   total_ars = excluded.total_ars, method = excluded.method,
		   deposit_usd = excluded.deposit_usd, deposit_due_at = excluded.deposit_due_at,
		   deposit_paid_at = excluded.deposit_paid_at,
		   non_refundable_usd = excluded.non_refundable_usd, updated_by = excluded.updated_by
		 RETURNING trip_id, total_usd, rate, total_ars, coalesce(method, ''),
		           deposit_usd, deposit_due_at, deposit_paid_at, non_refundable_usd`,
		p.TripID, p.TotalUSD, p.Rate, p.TotalARS, p.Method, p.DepositUSD,
		p.DepositDueAt, p.DepositPaidAt, p.NonRefundableUSD, p.UpdatedBy,
	).Scan(&saved.TripID, &saved.TotalUSD, &saved.Rate, &saved.TotalARS, &saved.Method,
		&saved.DepositUSD, &saved.DepositDueAt, &saved.DepositPaidAt, &saved.NonRefundableUSD)
	if err != nil {
		return store.Payment{}, fmt.Errorf("guardando el pago: %w", err)
	}
	if err := refreshDerived(ctx, tx, p.TripID); err != nil {
		return store.Payment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Payment{}, err
	}
	saved.UpdatedBy = p.UpdatedBy
	return saved, nil
}
