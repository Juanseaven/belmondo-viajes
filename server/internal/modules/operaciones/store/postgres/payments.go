package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// PutPayment guarda la cotización del viaje: total, cambio, seña y
// condiciones. El total en pesos se calcula (total por cambio) y la seña cobrada
// sale de los pagos registrados: ninguno de los dos se escribe a mano.
func (s *Store) PutPayment(ctx context.Context, p store.Payment) (store.Payment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Payment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`INSERT INTO payments
		 (trip_id, total_usd, rate, total_ars, method, deposit_usd, deposit_due_at,
		  non_refundable_usd, updated_by)
		 VALUES ($1, $2, $3, CASE WHEN $2::numeric IS NOT NULL AND $3::numeric IS NOT NULL THEN round($2::numeric * $3::numeric) END,
		         nullif($4, ''), $5, $6, $7, $8)
		 ON CONFLICT (trip_id) DO UPDATE SET
		   total_usd = excluded.total_usd, rate = excluded.rate,
		   total_ars = excluded.total_ars, method = excluded.method,
		   deposit_usd = excluded.deposit_usd, deposit_due_at = excluded.deposit_due_at,
		   non_refundable_usd = excluded.non_refundable_usd, updated_by = excluded.updated_by,
		   -- La seña vieja cobrada sin monto deja de serlo solo si alguien la
		   -- vuelve a cargar desde cero: sin monto y sin fecha.
		   deposit_paid_before = payments.deposit_paid_before
		                         AND NOT (excluded.deposit_usd IS NULL AND excluded.deposit_due_at IS NULL)`,
		p.TripID, p.TotalUSD, p.Rate, p.Method, p.DepositUSD,
		p.DepositDueAt, p.NonRefundableUSD, p.UpdatedBy,
	)
	if err != nil {
		return store.Payment{}, writeError("guardando el pago", err)
	}
	if err := settleDeposit(ctx, tx, p.TripID); err != nil {
		return store.Payment{}, err
	}
	if err := refreshDerived(ctx, tx, p.TripID); err != nil {
		return store.Payment{}, err
	}
	saved, err := readPayment(ctx, tx, p.TripID)
	if err != nil {
		return store.Payment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Payment{}, err
	}
	saved.UpdatedBy = p.UpdatedBy
	return *saved, nil
}

func readPayment(ctx context.Context, q querier, tripID string) (*store.Payment, error) {
	var p store.Payment
	err := q.QueryRow(ctx,
		`SELECT trip_id, total_usd, rate, total_ars, coalesce(method, ''), deposit_usd,
		        deposit_due_at, deposit_paid_at, non_refundable_usd, deposit_paid_before
		   FROM payments WHERE trip_id = $1`, tripID,
	).Scan(&p.TripID, &p.TotalUSD, &p.Rate, &p.TotalARS, &p.Method, &p.DepositUSD,
		&p.DepositDueAt, &p.DepositPaidAt, &p.NonRefundableUSD, &p.DepositPaidBefore)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leyendo el pago: %w", err)
	}
	return &p, nil
}

// depositTolerance es lo que puede faltar para dar la seña o el total por
// alcanzados: un pago en pesos pasado a dólares con un cambio con decimales
// queda unos centavos corto.
const depositTolerance = 1

// settleDeposit da la seña por cobrada con el pago que la alcanza, en orden de
// fecha, y con la fecha de ese pago. Sin monto de seña no hay nada que
// alcanzar; una seña cobrada antes de los pagos registrados conserva su fecha.
func settleDeposit(ctx context.Context, tx pgx.Tx, tripID string) error {
	_, err := tx.Exec(ctx,
		`UPDATE payments p SET deposit_paid_at = CASE
		   WHEN p.deposit_paid_before THEN p.deposit_paid_at
		   WHEN p.deposit_usd IS NULL THEN NULL
		   ELSE (SELECT x.paid_at::timestamp AT TIME ZONE 'America/Argentina/Buenos_Aires'
		           FROM (SELECT paid_at, sum(amount_usd) OVER (ORDER BY paid_at, created_at, id) AS acc
		                   FROM trip_payments WHERE trip_id = p.trip_id) x
		          WHERE x.acc > p.deposit_usd - $2
		          ORDER BY x.acc LIMIT 1)
		 END
		 WHERE p.trip_id = $1`, tripID, depositTolerance)
	if err != nil {
		return fmt.Errorf("calculando la seña: %w", err)
	}
	return nil
}

// AddPaymentRecord registra una plata que entró. En pesos, el equivalente en
// dólares sale del cambio de ese pago.
func (s *Store) AddPaymentRecord(ctx context.Context, n store.NewPaymentRecord) (store.PaymentRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.PaymentRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO trip_payments (trip_id, amount, currency, rate, amount_usd, paid_at, method, note, created_by)
		 VALUES ($1, $2, $3, $4,
		         CASE WHEN $3 = 'USD' THEN $2::numeric ELSE round($2::numeric / $4::numeric, 2) END,
		         $5, nullif(btrim($6), ''), nullif(btrim($7), ''), $8)
		 RETURNING id`,
		n.TripID, n.Amount, n.Currency, n.Rate, day(n.PaidAt), n.Method, n.Note, n.CreatedBy,
	).Scan(&id)
	if err != nil {
		return store.PaymentRecord{}, writeError("registrando el pago", err)
	}
	if err := settleDeposit(ctx, tx, n.TripID); err != nil {
		return store.PaymentRecord{}, err
	}
	records, err := readPaymentRecords(ctx, tx, n.TripID)
	if err != nil {
		return store.PaymentRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.PaymentRecord{}, err
	}
	for _, r := range records {
		if r.ID == id {
			return r, nil
		}
	}
	return store.PaymentRecord{}, store.ErrNotFound
}

// DeletePaymentRecord borra un pago mal cargado y devuelve su viaje.
func (s *Store) DeletePaymentRecord(ctx context.Context, id string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tripID string
	if err := tx.QueryRow(ctx, `DELETE FROM trip_payments WHERE id = $1 RETURNING trip_id`, id).Scan(&tripID); err != nil {
		return "", writeError("borrando el pago", err)
	}
	if err := settleDeposit(ctx, tx, tripID); err != nil {
		return "", err
	}
	return tripID, tx.Commit(ctx)
}

// readPaymentRecords lee los pagos de un viaje, del más nuevo al más viejo.
func readPaymentRecords(ctx context.Context, q querier, tripID string) ([]store.PaymentRecord, error) {
	rows, err := q.Query(ctx,
		`SELECT tp.id, tp.trip_id, tp.amount, tp.currency, tp.rate, tp.amount_usd, tp.paid_at,
		        coalesce(tp.method, ''), coalesce(tp.note, ''), coalesce(u.full_name, ''), tp.created_at
		   FROM trip_payments tp LEFT JOIN users u ON u.id = tp.created_by
		  WHERE tp.trip_id = $1
		  ORDER BY tp.paid_at DESC, tp.created_at DESC, tp.id DESC`, tripID)
	if err != nil {
		return nil, fmt.Errorf("leyendo los pagos: %w", err)
	}
	records, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.PaymentRecord, error) {
		var r store.PaymentRecord
		err := row.Scan(&r.ID, &r.TripID, &r.Amount, &r.Currency, &r.Rate, &r.AmountUSD, &r.PaidAt,
			&r.Method, &r.Note, &r.CreatedByName, &r.CreatedAt)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("armando los pagos: %w", err)
	}
	return records, nil
}
