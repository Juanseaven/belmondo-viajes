package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

func (s *Store) TripDetail(ctx context.Context, tripID string) (store.TripDetailData, error) {
	var d store.TripDetailData
	err := s.pool.QueryRow(ctx,
		`SELECT t.id, t.number, t.customer_id, t.status, coalesce(t.destination, ''), t.depart_at,
		        t.return_at, coalesce(t.seller_user_id::text, ''), coalesce(su.full_name, ''),
		        coalesce(cu2.full_name, ''), t.created_at,
		        cu.id, cu.name, coalesce(cu.email, ''), coalesce(cu.phone_number, ''),
		        coalesce(cu.document_type, ''), coalesce(cu.document_number, ''), cu.customer_since,
		        coalesce(array_agg(cc.contact_id::text) FILTER (WHERE cc.contact_id IS NOT NULL), '{}')
		   FROM trips t
		   JOIN customers cu ON cu.id = t.customer_id
		   LEFT JOIN customer_contacts cc ON cc.customer_id = cu.id
		   LEFT JOIN users su ON su.id = t.seller_user_id
		   LEFT JOIN users cu2 ON cu2.id = t.created_by
		  WHERE t.id = $1
		  GROUP BY t.id, cu.id, su.full_name, cu2.full_name`, tripID,
	).Scan(&d.ID, &d.Number, &d.CustomerID, &d.Status, &d.Destination, &d.DepartAt,
		&d.ReturnAt, &d.SellerUserID, &d.SellerName, &d.CreatedByName, &d.CreatedAt,
		&d.Customer.ID, &d.Customer.Name, &d.Customer.Email, &d.Customer.PhoneNumber,
		&d.Customer.DocumentType, &d.Customer.DocumentNumber, &d.Customer.CustomerSince,
		&d.Customer.ContactIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.TripDetailData{}, store.ErrNotFound
	}
	if err != nil {
		return store.TripDetailData{}, fmt.Errorf("leyendo el viaje: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, trip_id, first_name, last_name, kind, is_holder,
		        coalesce(document_type, ''), coalesce(document_number, ''), document_expires_at
		   FROM passengers WHERE trip_id = $1 ORDER BY is_holder DESC, created_at, id`, tripID)
	if err != nil {
		return store.TripDetailData{}, fmt.Errorf("leyendo pasajeros: %w", err)
	}
	d.Passengers, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.Passenger, error) {
		var p store.Passenger
		err := row.Scan(&p.ID, &p.TripID, &p.FirstName, &p.LastName, &p.Kind, &p.IsHolder,
			&p.DocumentType, &p.DocumentNumber, &p.DocumentExpiresAt)
		return p, err
	})
	if err != nil {
		return store.TripDetailData{}, fmt.Errorf("armando pasajeros: %w", err)
	}

	var payment store.Payment
	err = s.pool.QueryRow(ctx,
		`SELECT trip_id, total_usd, rate, total_ars, coalesce(method, ''), deposit_usd,
		        deposit_due_at, deposit_paid_at, non_refundable_usd
		   FROM payments WHERE trip_id = $1`, tripID,
	).Scan(&payment.TripID, &payment.TotalUSD, &payment.Rate, &payment.TotalARS, &payment.Method,
		&payment.DepositUSD, &payment.DepositDueAt, &payment.DepositPaidAt, &payment.NonRefundableUSD)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return store.TripDetailData{}, fmt.Errorf("leyendo el pago: %w", err)
	}
	if err == nil {
		d.Payment = &payment
	}

	if err := s.readControls(ctx, tripID, &d); err != nil {
		return store.TripDetailData{}, err
	}
	return d, nil
}

func (s *Store) readControls(ctx context.Context, tripID string, d *store.TripDetailData) error {
	rows, err := s.pool.Query(ctx,
		`SELECT c.id, c.item_id, c.stage, c.closing, c.critical, c.status, c.due_at,
		        e.id, e.closed_by, coalesce(u.full_name, ''), e.closed_at, e.message_id
		   FROM controls c
		   LEFT JOIN control_evidence e ON e.control_id = c.id
		   LEFT JOIN users u ON u.id = e.closed_by
		  WHERE c.trip_id = $1
		  ORDER BY c.created_at, e.closed_at`, tripID)
	if err != nil {
		return fmt.Errorf("leyendo controles: %w", err)
	}
	defer rows.Close()

	byID := make(map[string]int)
	for rows.Next() {
		var c store.Control
		var evidenceID, closedBy, messageID *string
		var closedByName string
		var closedAt *time.Time
		if err := rows.Scan(&c.ID, &c.ItemID, &c.Stage, &c.Closing, &c.Critical, &c.Status,
			&c.DueAt, &evidenceID, &closedBy, &closedByName, &closedAt, &messageID); err != nil {
			return fmt.Errorf("armando controles: %w", err)
		}
		idx, ok := byID[c.ID]
		if !ok {
			applyTemplate(&c)
			c.Evidence = []store.ControlEvidence{}
			d.Controls = append(d.Controls, c)
			idx = len(d.Controls) - 1
			byID[c.ID] = idx
		}
		if evidenceID != nil {
			e := store.ControlEvidence{ID: *evidenceID, ClosedByName: closedByName, ClosedAt: *closedAt}
			if closedBy != nil {
				e.ClosedBy = *closedBy
			}
			if messageID != nil {
				e.MessageID = *messageID
			}
			d.Controls[idx].Evidence = append(d.Controls[idx].Evidence, e)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("recorriendo controles: %w", err)
	}
	order := checklistOrder()
	sort.SliceStable(d.Controls, func(i, j int) bool { return order[d.Controls[i].ItemID] < order[d.Controls[j].ItemID] })
	return nil
}

func applyTemplate(c *store.Control) {
	for _, item := range domain.Checklist() {
		if item.ID == c.ItemID {
			c.Title, c.Prompt, c.ActionLabel, c.Message = item.Title, item.Prompt, item.ActionLabel, item.Message
			return
		}
	}
}

func checklistOrder() map[string]int {
	order := make(map[string]int, len(domain.Checklist()))
	for i, item := range domain.Checklist() {
		order[item.ID] = i
	}
	return order
}

func (s *Store) TripsByContact(ctx context.Context, contactID string) ([]store.TripSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.number, t.customer_id, t.status, coalesce(t.destination, ''), t.depart_at,
		        t.return_at, coalesce(t.seller_user_id::text, ''), coalesce(u.full_name, ''),
		        t.created_at,
		        (SELECT count(*) FROM passengers p WHERE p.trip_id = t.id), pa.total_usd
		   FROM customer_contacts cc
		   JOIN trips t ON t.customer_id = cc.customer_id
		   LEFT JOIN users u ON u.id = t.seller_user_id
		   LEFT JOIN payments pa ON pa.trip_id = t.id
		  WHERE cc.contact_id = $1
		  ORDER BY (t.status = 'cerrado'), t.created_at DESC, t.number DESC`, contactID)
	if err != nil {
		return nil, fmt.Errorf("listando viajes: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.TripSummary, error) {
		var t store.TripSummary
		err := row.Scan(&t.ID, &t.Number, &t.CustomerID, &t.Status, &t.Destination,
			&t.DepartAt, &t.ReturnAt, &t.SellerUserID, &t.SellerName, &t.CreatedAt,
			&t.PassengerCount, &t.TotalUSD)
		return t, err
	})
}

func (s *Store) Inbox(ctx context.Context, contactIDs []string) ([]store.InboxItem, error) {
	if len(contactIDs) == 0 {
		return []store.InboxItem{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`WITH requested AS (
		   SELECT contact_id::uuid, ord FROM unnest($1::text[]) WITH ORDINALITY r(contact_id, ord)
		 )
		 SELECT r.contact_id::text,
		        t.id, t.number, t.customer_id, t.status, t.destination, t.depart_at, t.return_at,
		        coalesce(t.seller_user_id::text, ''), coalesce(u.full_name, ''), t.created_at,
		        pc.count, pa.total_usd,
		        nc.id, nc.item_id, nc.closing, nc.critical, nc.due_at
		   FROM requested r
		   LEFT JOIN customer_contacts cc ON cc.contact_id = r.contact_id
		   LEFT JOIN LATERAL (
		     SELECT t.* FROM trips t WHERE t.customer_id = cc.customer_id
		     ORDER BY (t.status = 'cerrado'), t.created_at DESC, t.number DESC LIMIT 1
		   ) t ON true
		   LEFT JOIN users u ON u.id = t.seller_user_id
		   LEFT JOIN LATERAL (SELECT count(*)::int AS count FROM passengers p WHERE p.trip_id = t.id) pc ON true
		   LEFT JOIN payments pa ON pa.trip_id = t.id
		   LEFT JOIN LATERAL (
		     SELECT c.id, c.item_id, c.closing, c.critical, c.due_at
		       FROM controls c WHERE c.trip_id = t.id AND c.status = 'abierto'
		      ORDER BY c.critical DESC, c.due_at ASC NULLS LAST, c.created_at LIMIT 1
		   ) nc ON true
		  ORDER BY r.ord`, contactIDs)
	if err != nil {
		return nil, fmt.Errorf("enriqueciendo bandeja: %w", err)
	}
	defer rows.Close()

	items := make([]store.InboxItem, 0, len(contactIDs))
	for rows.Next() {
		var item store.InboxItem
		var tripID, customerID, status, destination, sellerID, sellerName *string
		var number *int64
		var departAt, returnAt, createdAt *time.Time
		var passengerCount *int
		var totalUSD *float64
		var controlID, itemID, closing *string
		var critical *bool
		var dueAt *time.Time
		if err := rows.Scan(&item.ContactID, &tripID, &number, &customerID, &status, &destination,
			&departAt, &returnAt, &sellerID, &sellerName, &createdAt, &passengerCount, &totalUSD,
			&controlID, &itemID, &closing, &critical, &dueAt); err != nil {
			return nil, fmt.Errorf("armando bandeja: %w", err)
		}
		if tripID != nil {
			t := &store.TripSummary{Trip: store.Trip{
				ID: *tripID, Number: *number, CustomerID: *customerID,
				Status: domain.TripStatus(*status), Destination: *destination,
				DepartAt: departAt, ReturnAt: returnAt, SellerUserID: *sellerID,
				SellerName: *sellerName, CreatedAt: *createdAt,
			}, PassengerCount: *passengerCount, TotalUSD: totalUSD}
			if controlID != nil {
				t.NextControl = &store.ControlSummary{
					ID: *controlID, ItemID: *itemID, Closing: domain.Closing(*closing),
					Critical: *critical, DueAt: dueAt,
				}
				for _, template := range domain.Checklist() {
					if template.ID == *itemID {
						t.NextControl.Title = template.Title
						break
					}
				}
			}
			item.Trip = t
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recorriendo bandeja: %w", err)
	}
	return items, nil
}
