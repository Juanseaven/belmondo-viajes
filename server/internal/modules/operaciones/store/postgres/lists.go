package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Las listas de la pantalla de Viajes. La clasificación de pendientes vive en
// domain.ClassifyPending; acá solo se junta lo que la regla necesita.

// tripRowSelect lee un viaje como se ve en una lista. La conversación es la más
// reciente de cualquiera de los contactos del cliente: conversations es tabla
// pública del espejo.
const tripRowSelect = `
	SELECT t.id, t.number, t.status, coalesce(t.destination, ''), t.depart_at, t.return_at,
	       cu.name, (SELECT count(*)::int FROM passengers p WHERE p.trip_id = t.id),
	       coalesce(u.full_name, ''), pa.total_usd, pa.deposit_usd, pa.deposit_due_at,
	       pa.deposit_paid_at, coalesce(conv.id::text, '')
	  FROM trips t
	  JOIN customers cu ON cu.id = t.customer_id
	  LEFT JOIN users u ON u.id = t.seller_user_id
	  LEFT JOIN payments pa ON pa.trip_id = t.id
	  LEFT JOIN LATERAL (
	    SELECT cv.id FROM customer_contacts cc
	      JOIN conversations cv ON cv.contact_id = cc.contact_id
	     WHERE cc.customer_id = t.customer_id
	     ORDER BY cv.last_message_at DESC NULLS LAST, cv.created_at DESC
	     LIMIT 1
	  ) conv ON true`

func scanTripRow(row pgx.CollectableRow) (store.TripRow, error) {
	var r store.TripRow
	err := row.Scan(&r.ID, &r.Number, &r.Status, &r.Destination, &r.DepartAt, &r.ReturnAt,
		&r.CustomerName, &r.PassengerCount, &r.SellerName, &r.TotalUSD, &r.DepositUSD,
		&r.DepositDueAt, &r.DepositPaidAt, &r.ConversationID)
	return r, err
}

// pendingItems junta, por viaje, lo que tiene plazo: los controles abiertos con
// vencimiento y la seña sin cobrar. La seña sale de la fila ya leída.
func (s *Store) pendingItems(ctx context.Context, trips []store.TripRow) (map[string][]domain.PendingItem, error) {
	items := make(map[string][]domain.PendingItem, len(trips))
	if len(trips) == 0 {
		return items, nil
	}
	ids := make([]string, len(trips))
	for i, trip := range trips {
		ids[i] = trip.ID
		if trip.DepositDueAt != nil && trip.DepositPaidAt == nil {
			items[trip.ID] = append(items[trip.ID], domain.DepositItem(*trip.DepositDueAt, trip.DepositUSD))
		}
	}

	rows, err := s.pool.Query(ctx,
		`SELECT trip_id::text, id::text, item_id, critical, due_at
		   FROM controls
		  WHERE trip_id = ANY($1::uuid[]) AND status = 'abierto' AND due_at IS NOT NULL`, ids)
	if err != nil {
		return nil, fmt.Errorf("leyendo pendientes: %w", err)
	}
	defer rows.Close()

	tasks := make(map[string]string)
	for _, item := range domain.Checklist() {
		tasks[item.ID] = item.Task
	}
	for rows.Next() {
		var tripID string
		item := domain.PendingItem{Kind: domain.PendingControl}
		if err := rows.Scan(&tripID, &item.ID, &item.ItemID, &item.Critical, &item.DueAt); err != nil {
			return nil, fmt.Errorf("armando pendientes: %w", err)
		}
		item.Task = tasks[item.ItemID]
		items[tripID] = append(items[tripID], item)
	}
	return items, rows.Err()
}

var groupOrder = map[domain.PendingGroup]int{domain.GroupAtrasado: 0, domain.GroupSemana: 1, domain.GroupSalen: 2}

// Pending arma la lista de pendientes para el día dado.
//
// La consulta trae solo candidatos —viajes abiertos con algo que vence dentro
// de la semana o que salen en las próximas dos— y ClassifyPending decide. El
// filtro de la consulta es más ancho que la regla a propósito: si alguna vez
// difieren, sobra un candidato, nunca falta un pendiente.
func (s *Store) Pending(ctx context.Context, today time.Time) ([]store.PendingRow, error) {
	weekEnd := today.AddDate(0, 0, 7)
	departEnd := today.AddDate(0, 0, 14)
	rows, err := s.pool.Query(ctx, tripRowSelect+`
		 WHERE t.status <> 'cerrado'
		   AND (EXISTS (SELECT 1 FROM controls c
		                 WHERE c.trip_id = t.id AND c.status = 'abierto' AND c.due_at <= $1::date)
		        OR (pa.deposit_paid_at IS NULL AND pa.deposit_due_at <= $1::date)
		        OR t.depart_at BETWEEN $2::date AND $3::date)`, day(weekEnd), day(today), day(departEnd))
	if err != nil {
		return nil, fmt.Errorf("listando pendientes: %w", err)
	}
	trips, err := pgx.CollectRows(rows, scanTripRow)
	if err != nil {
		return nil, fmt.Errorf("armando pendientes: %w", err)
	}
	items, err := s.pendingItems(ctx, trips)
	if err != nil {
		return nil, err
	}

	out := make([]store.PendingRow, 0, len(trips))
	for _, trip := range trips {
		classified, ok := domain.ClassifyPending(today, trip.DepartAt, items[trip.ID])
		if !ok {
			continue
		}
		trip.Next = domain.NextPending(items[trip.ID])
		out = append(out, store.PendingRow{TripRow: trip, Group: classified.Group, Task: classified.Main, More: classified.More})
	}

	// Dentro de cada grupo: lo crítico primero y después el plazo más cercano. En
	// los que salen pronto, por fecha de salida.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Group != b.Group {
			return groupOrder[a.Group] < groupOrder[b.Group]
		}
		if a.Group == domain.GroupSalen {
			return a.DepartAt.Before(*b.DepartAt)
		}
		if a.Task.Critical != b.Task.Critical {
			return a.Task.Critical
		}
		if !a.Task.DueAt.Equal(b.Task.DueAt) {
			return a.Task.DueAt.Before(b.Task.DueAt)
		}
		return a.Number < b.Number
	})
	return out, nil
}

// ListTrips es la consulta de «Todos los viajes»: búsqueda, filtros, orden y
// cursor. El cursor es keyset sobre la clave de orden más el número, que es
// único: paginar no repite ni saltea aunque se carguen viajes mientras tanto.
func (s *Store) ListTrips(ctx context.Context, f store.TripFilter) (store.TripPage, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	q := &query{}
	where := filterSQL(q, f)

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM trips t
		   JOIN customers cu ON cu.id = t.customer_id
		   LEFT JOIN payments pa ON pa.trip_id = t.id
		  WHERE `+where, q.args...).Scan(&total); err != nil {
		return store.TripPage{}, fmt.Errorf("contando viajes: %w", err)
	}

	key, direction := sortKey(f.Sort)
	pageWhere := where
	if f.Cursor != "" {
		k, n, err := decodeCursor(f.Cursor)
		if err != nil {
			return store.TripPage{}, store.ErrBadCursor
		}
		cmp := ">"
		if direction == "DESC" {
			cmp = "<"
		}
		if key == "" {
			pageWhere += fmt.Sprintf(" AND t.number < %s", q.arg(n))
		} else {
			cast := map[store.TripSort]string{store.SortSalida: "date", store.SortTotal: "numeric"}[sortOrDefault(f.Sort)]
			kArg, nArg := q.arg(k), q.arg(n)
			pageWhere += fmt.Sprintf(" AND (%[1]s %[2]s %[3]s::%[4]s OR (%[1]s = %[3]s::%[4]s AND t.number < %[5]s))",
				key, cmp, kArg, cast, nArg)
		}
	}
	order := "t.number DESC"
	if key != "" {
		order = key + " " + direction + ", t.number DESC"
	}
	rows, err := s.pool.Query(ctx, tripRowSelect+` WHERE `+pageWhere+
		` ORDER BY `+order+fmt.Sprintf(" LIMIT %d", f.Limit+1), q.args...)
	if err != nil {
		return store.TripPage{}, fmt.Errorf("listando viajes: %w", err)
	}
	trips, err := pgx.CollectRows(rows, scanTripRow)
	if err != nil {
		return store.TripPage{}, fmt.Errorf("armando viajes: %w", err)
	}

	page := store.TripPage{Items: trips, Total: total}
	if len(trips) > f.Limit {
		page.Items = trips[:f.Limit]
		page.NextCursor = encodeCursor(f.Sort, page.Items[f.Limit-1])
	}
	items, err := s.pendingItems(ctx, page.Items)
	if err != nil {
		return store.TripPage{}, err
	}
	for i := range page.Items {
		page.Items[i].Next = domain.NextPending(items[page.Items[i].ID])
	}
	return page, nil
}

// day escribe un día calendario para compararlo contra una columna date. Pasar
// un time.Time haría la comparación en timestamptz, que depende del huso
// horario de la sesión.
func day(t time.Time) string { return t.Format("2006-01-02") }

type query struct{ args []any }

func (q *query) arg(value any) string {
	q.args = append(q.args, value)
	return "$" + strconv.Itoa(len(q.args))
}

// fold pasa una columna a minúsculas y sin tildes, para que «cancun» encuentre
// «Cancún». Con translate alcanza para el castellano y no hace falta instalar
// la extensión unaccent.
func fold(column string) string {
	return fmt.Sprintf("translate(lower(%s), 'áéíóúüñ', 'aeiouun')", column)
}

var foldReplacer = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

func filterSQL(q *query, f store.TripFilter) string {
	conds := []string{"true"}

	if text := strings.TrimSpace(f.Query); text != "" {
		if number, err := strconv.ParseInt(strings.TrimPrefix(text, "#"), 10, 64); err == nil {
			conds = append(conds, "t.number = "+q.arg(number))
		} else {
			like := q.arg("%" + foldReplacer.Replace(strings.ToLower(text)) + "%")
			conds = append(conds, fmt.Sprintf(`(%[1]s LIKE %[4]s OR %[2]s LIKE %[4]s
			  OR EXISTS (SELECT 1 FROM passengers p WHERE p.trip_id = t.id AND %[3]s LIKE %[4]s))`,
				fold("cu.name"), fold("coalesce(t.destination, '')"),
				fold("p.first_name || ' ' || p.last_name"), like))
		}
	}
	switch {
	case len(f.Statuses) > 0:
		statuses := make([]string, len(f.Statuses))
		for i, status := range f.Statuses {
			statuses[i] = string(status)
		}
		conds = append(conds, "t.status = ANY("+q.arg(statuses)+"::text[])")
	case f.OpenOnly:
		conds = append(conds, "t.status <> 'cerrado'")
	}
	if f.DepartFrom != nil {
		conds = append(conds, "t.depart_at >= "+q.arg(day(*f.DepartFrom))+"::date")
	}
	if f.DepartTo != nil {
		conds = append(conds, "t.depart_at <= "+q.arg(day(*f.DepartTo))+"::date")
	}
	switch {
	case f.SellerNone:
		conds = append(conds, "t.seller_user_id IS NULL")
	case f.SellerID != "":
		conds = append(conds, "t.seller_user_id = "+q.arg(f.SellerID)+"::uuid")
	}
	if f.LateOnly {
		today := q.arg(day(f.Today)) + "::date"
		conds = append(conds, fmt.Sprintf(`(EXISTS (SELECT 1 FROM controls c
		      WHERE c.trip_id = t.id AND c.status = 'abierto' AND c.due_at < %[1]s)
		   OR (pa.deposit_paid_at IS NULL AND pa.deposit_due_at < %[1]s))`, today))
	}
	return strings.Join(conds, " AND ")
}

func sortOrDefault(s store.TripSort) store.TripSort {
	if s == "" {
		return store.SortSalida
	}
	return s
}

// sortKey es la expresión de orden, nunca nula: los viajes sin salida van al
// final y los sin total, último en el orden por total. Vacía para el número,
// que se ordena solo.
func sortKey(s store.TripSort) (string, string) {
	switch sortOrDefault(s) {
	case store.SortNumero:
		return "", "DESC"
	case store.SortTotal:
		return "coalesce(pa.total_usd, -1)", "DESC"
	default:
		return "coalesce(t.depart_at, 'infinity'::date)", "ASC"
	}
}

func encodeCursor(s store.TripSort, last store.TripRow) string {
	var key string
	switch sortOrDefault(s) {
	case store.SortTotal:
		key = "-1"
		if last.TotalUSD != nil {
			key = strconv.FormatFloat(*last.TotalUSD, 'f', -1, 64)
		}
	case store.SortSalida:
		key = "infinity"
		if last.DepartAt != nil {
			key = last.DepartAt.Format("2006-01-02")
		}
	}
	return base64.RawURLEncoding.EncodeToString([]byte(key + "|" + strconv.FormatInt(last.Number, 10)))
}

func decodeCursor(cursor string) (string, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", 0, err
	}
	key, number, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", 0, fmt.Errorf("cursor sin número")
	}
	n, err := strconv.ParseInt(number, 10, 64)
	return key, n, err
}
