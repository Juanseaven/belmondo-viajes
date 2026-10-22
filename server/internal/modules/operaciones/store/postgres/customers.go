package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// La pantalla de Clientes. Un cliente es quien compra el viaje; puede tener
// varios contactos de WhatsApp o ninguno. Lee contacts, conversations e
// instances del espejo, que son tablas públicas, y no escribe ninguna.

const customerSelect = `
	SELECT cu.id, cu.name, coalesce(cu.email, ''), coalesce(cu.phone_number, ''),
	       coalesce(cu.document_type, ''), coalesce(cu.document_number, ''),
	       cu.customer_since,
	       coalesce((SELECT array_agg(cc.contact_id::text ORDER BY cc.linked_at)
	                   FROM customer_contacts cc WHERE cc.customer_id = cu.id), '{}'),
	       ` + "%s" + `
	  FROM customers cu`

var customerColumns = fmt.Sprintf(customerSelect, fmt.Sprintf(hasChatSQL, "cu.id"))

func scanCustomer(row pgx.Row) (store.Customer, error) {
	var c store.Customer
	err := row.Scan(&c.ID, &c.Name, &c.Email, &c.PhoneNumber, &c.DocumentType,
		&c.DocumentNumber, &c.CustomerSince, &c.ContactIDs, &c.HasChat)
	return c, err
}

func (s *Store) Customer(ctx context.Context, customerID string) (store.Customer, error) {
	c, err := scanCustomer(s.pool.QueryRow(ctx, customerColumns+` WHERE cu.id = $1`, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Customer{}, store.ErrNotFound
	}
	if err != nil {
		return store.Customer{}, writeError("leyendo el cliente", err)
	}
	return c, nil
}

// CreateCustomer da de alta a quien compra sin haber escrito por WhatsApp.
func (s *Store) CreateCustomer(ctx context.Context, n store.NewCustomer) (store.Customer, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO customers (name, email, phone_number, document_type, document_number, created_by)
		 VALUES ($1, nullif($2, ''), nullif($3, ''), nullif($4, ''), nullif($5, ''), $6)
		 RETURNING id`,
		n.Name, n.Email, n.PhoneNumber, n.DocumentType, n.DocumentNumber, n.CreatedBy).Scan(&id)
	if err != nil {
		return store.Customer{}, writeError("creando el cliente", err)
	}
	return s.Customer(ctx, id)
}

func (s *Store) UpdateCustomer(ctx context.Context, customerID string, patch store.CustomerPatch) (store.Customer, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Customer{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE customers SET
		   name = coalesce($2, name),
		   email = CASE WHEN $3::text IS NULL THEN email ELSE nullif($3, '') END,
		   phone_number = CASE WHEN $4::text IS NULL THEN phone_number ELSE nullif($4, '') END,
		   document_type = CASE WHEN $5::text IS NULL THEN document_type ELSE nullif($5, '') END,
		   document_number = CASE WHEN $6::text IS NULL THEN document_number ELSE nullif($6, '') END,
		   customer_since = CASE WHEN $7::text IS NULL THEN customer_since ELSE $7::date END
		 WHERE id = $1`,
		customerID, patch.Name, patch.Email, patch.PhoneNumber, patch.DocumentType,
		patch.DocumentNumber, patch.CustomerSince,
	)
	if err != nil {
		return store.Customer{}, writeError("actualizando el cliente", err)
	}
	if tag.RowsAffected() == 0 {
		return store.Customer{}, store.ErrNotFound
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

// contactName es el nombre de un contacto como lo muestra el espejo cuando no
// hay agenda de la línea: el editado a mano, el que publica el contacto o su
// número.
const contactName = `coalesce(nullif(ct.display_name, ''), nullif(ct.push_name, ''), ct.phone_number, ct.wa_jid)`

// CustomerDetail arma la página del cliente: sus datos, sus contactos, sus
// viajes y con quién viaja.
func (s *Store) CustomerDetail(ctx context.Context, customerID string) (store.CustomerDetail, error) {
	customer, err := s.Customer(ctx, customerID)
	if err != nil {
		return store.CustomerDetail{}, err
	}
	d := store.CustomerDetail{Customer: customer}

	rows, err := s.pool.Query(ctx,
		`SELECT ct.id, `+contactName+`, coalesce(ct.phone_number, ''),
		        coalesce(conv.id::text, ''), coalesce(conv.owner_label, ''), conv.last_message_at
		   FROM customer_contacts cc
		   JOIN contacts ct ON ct.id = cc.contact_id
		   LEFT JOIN LATERAL (
		     SELECT cv.id, i.owner_label, cv.last_message_at
		       FROM conversations cv JOIN instances i ON i.id = cv.instance_id
		      WHERE cv.contact_id = ct.id AND NOT cv.is_group
		      ORDER BY cv.last_message_at DESC NULLS LAST, cv.created_at DESC
		      LIMIT 1
		   ) conv ON true
		  WHERE cc.customer_id = $1
		  ORDER BY conv.last_message_at DESC NULLS LAST, cc.linked_at`, customerID)
	if err != nil {
		return store.CustomerDetail{}, fmt.Errorf("leyendo los contactos del cliente: %w", err)
	}
	d.Contacts, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.CustomerContact, error) {
		var c store.CustomerContact
		err := row.Scan(&c.ContactID, &c.Name, &c.PhoneNumber, &c.ConversationID, &c.InstanceLabel, &c.LastMessageAt)
		return c, err
	})
	if err != nil {
		return store.CustomerDetail{}, fmt.Errorf("armando los contactos del cliente: %w", err)
	}

	rows, err = s.pool.Query(ctx, tripRowSelect+`
		 WHERE t.customer_id = $1
		 ORDER BY (t.status = 'cerrado'), t.created_at DESC, t.number DESC`, customerID)
	if err != nil {
		return store.CustomerDetail{}, fmt.Errorf("listando los viajes del cliente: %w", err)
	}
	if d.Trips, err = pgx.CollectRows(rows, scanTripRow); err != nil {
		return store.CustomerDetail{}, fmt.Errorf("armando los viajes del cliente: %w", err)
	}
	items, err := s.pendingItems(ctx, d.Trips)
	if err != nil {
		return store.CustomerDetail{}, err
	}
	for i := range d.Trips {
		d.Trips[i].Next = domain.NextPending(items[d.Trips[i].ID])
	}

	if d.Companions, err = companions(ctx, s.pool, customerID, ""); err != nil {
		return store.CustomerDetail{}, err
	}
	return d, nil
}

// --- La lista ----------------------------------------------------------------

// customerRowsSQL arma, por cliente, lo que muestra la tabla y lo que la
// filtra. El último movimiento es lo más reciente entre el cliente, sus viajes,
// sus pagos, sus pasajeros y sus controles cerrados: cargar una seña o cerrar
// un control también es trabajar con ese cliente.
const customerRowsSQL = `
	SELECT cu.id, cu.name, cu.customer_since, coalesce(cu.email, '') AS email,
	       coalesce(cu.phone_number, '') AS phone_number,
	       coalesce(chat.phone, '') AS contact_phone, coalesce(chat.id::text, '') AS conversation_id,
	       coalesce(chat.count, 0) AS chat_count,
	       coalesce(tr.count, 0) AS trip_count, coalesce(tr.last_destination, '') AS last_destination,
	       ot.id AS open_id, ot.number AS open_number, ot.status AS open_status,
	       coalesce(ot.destination, '') AS open_destination, ot.depart_at AS open_depart_at,
	       greatest(cu.created_at, cu.updated_at, tr.activity, act.activity) AS activity
	  FROM customers cu
	  LEFT JOIN LATERAL (
	    SELECT (array_agg(cv.id ORDER BY cv.last_message_at DESC NULLS LAST, cv.created_at DESC))[1] AS id,
	           (array_agg(ct.phone_number ORDER BY cv.last_message_at DESC NULLS LAST, cv.created_at DESC))[1] AS phone,
	           count(*)::int AS count
	      FROM customer_contacts cc
	      JOIN conversations cv ON cv.contact_id = cc.contact_id AND NOT cv.is_group
	      JOIN contacts ct ON ct.id = cc.contact_id
	     WHERE cc.customer_id = cu.id
	  ) chat ON true
	  LEFT JOIN LATERAL (
	    SELECT count(*)::int AS count,
	           (array_agg(t.destination ORDER BY t.created_at DESC, t.number DESC))[1] AS last_destination,
	           max(greatest(t.created_at, t.updated_at)) AS activity
	      FROM trips t WHERE t.customer_id = cu.id
	  ) tr ON true
	  LEFT JOIN LATERAL (
	    SELECT t.id, t.number, t.status, t.destination, t.depart_at
	      FROM trips t WHERE t.customer_id = cu.id AND t.status <> 'cerrado'
	     ORDER BY t.created_at DESC, t.number DESC LIMIT 1
	  ) ot ON true
	  LEFT JOIN LATERAL (
	    SELECT max(x.at) AS activity FROM (
	      SELECT pa.updated_at AS at FROM payments pa JOIN trips t ON t.id = pa.trip_id WHERE t.customer_id = cu.id
	      UNION ALL
	      SELECT greatest(p.created_at, p.updated_at) FROM passengers p JOIN trips t ON t.id = p.trip_id WHERE t.customer_id = cu.id
	      UNION ALL
	      SELECT e.closed_at FROM control_evidence e JOIN controls c ON c.id = e.control_id
	        JOIN trips t ON t.id = c.trip_id WHERE t.customer_id = cu.id
	    ) x
	  ) act ON true`

// ListCustomers es la consulta de la pantalla de Clientes: búsqueda, chip,
// orden y cursor.
func (s *Store) ListCustomers(ctx context.Context, f store.CustomerFilter) (store.CustomerPage, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	if f.Chip == "" {
		f.Chip = store.ChipTodos
	}

	q := &query{}
	search := customerSearchSQL(q, f.Query)
	// Los números de los chips: cuántos habría en cada uno con la búsqueda.
	counts := map[string]int{}
	var todos, conViaje, sinViajes, sinWhatsApp int
	if err := s.pool.QueryRow(ctx,
		`WITH base AS (`+customerRowsSQL+` WHERE `+search+`)
		 SELECT count(*)::int,
		        count(*) FILTER (WHERE open_id IS NOT NULL)::int,
		        count(*) FILTER (WHERE trip_count = 0)::int,
		        count(*) FILTER (WHERE chat_count = 0)::int
		   FROM base`, q.args...).Scan(&todos, &conViaje, &sinViajes, &sinWhatsApp); err != nil {
		return store.CustomerPage{}, fmt.Errorf("contando clientes: %w", err)
	}
	counts[string(store.ChipTodos)] = todos
	counts[string(store.ChipConViaje)] = conViaje
	counts[string(store.ChipSinViajes)] = sinViajes
	counts[string(store.ChipSinWhatsApp)] = sinWhatsApp

	conds := []string{"true"}
	switch f.Chip {
	case store.ChipConViaje:
		conds = append(conds, "open_id IS NOT NULL")
	case store.ChipSinViajes:
		conds = append(conds, "trip_count = 0")
	case store.ChipSinWhatsApp:
		conds = append(conds, "chat_count = 0")
	}

	byName := f.Sort == store.SortNombre
	order := "activity DESC, id DESC"
	if byName {
		order = fold("name") + " ASC, id ASC"
	}
	if f.Cursor != "" {
		key, id, err := decodeCustomerCursor(f.Cursor)
		if err != nil {
			return store.CustomerPage{}, store.ErrBadCursor
		}
		kArg, idArg := q.arg(key), q.arg(id)
		if byName {
			conds = append(conds, fmt.Sprintf("(%[1]s > %[2]s OR (%[1]s = %[2]s AND id > %[3]s::uuid))", fold("name"), kArg, idArg))
		} else {
			conds = append(conds, fmt.Sprintf("(activity < %[1]s::timestamptz OR (activity = %[1]s::timestamptz AND id < %[2]s::uuid))", kArg, idArg))
		}
	}

	rows, err := s.pool.Query(ctx,
		`WITH base AS (`+customerRowsSQL+` WHERE `+search+`)
		 SELECT id, name, customer_since, email, phone_number, contact_phone, conversation_id,
		        chat_count, trip_count, last_destination, open_id, open_number, open_status,
		        open_destination, open_depart_at, activity, `+fold("name")+`
		   FROM base WHERE `+strings.Join(conds, " AND ")+`
		  ORDER BY `+order+fmt.Sprintf(" LIMIT %d", f.Limit+1), q.args...)
	if err != nil {
		return store.CustomerPage{}, fmt.Errorf("listando clientes: %w", err)
	}
	type rowWithKey struct {
		store.CustomerRow
		folded string
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (rowWithKey, error) {
		var r rowWithKey
		var openID, openStatus *string
		var openNumber *int64
		var openDestination string
		var openDepart *time.Time
		err := row.Scan(&r.ID, &r.Name, &r.CustomerSince, &r.Email, &r.PhoneNumber, &r.ContactPhone,
			&r.ConversationID, &r.ChatCount, &r.TripCount, &r.LastDestination, &openID, &openNumber,
			&openStatus, &openDestination, &openDepart, &r.LastActivity, &r.folded)
		if openID != nil {
			r.OpenTrip = &store.CustomerOpenTrip{ID: *openID, Number: *openNumber,
				Status: domain.TripStatus(*openStatus), Destination: openDestination, DepartAt: openDepart}
		}
		return r, err
	})
	if err != nil {
		return store.CustomerPage{}, fmt.Errorf("armando clientes: %w", err)
	}

	total := counts[string(f.Chip)]
	page := store.CustomerPage{Items: make([]store.CustomerRow, 0, len(list)), Total: total, ByChip: counts}
	if len(list) > f.Limit {
		last := list[f.Limit-1]
		key := last.LastActivity.UTC().Format(time.RFC3339Nano)
		if byName {
			key = last.folded
		}
		page.NextCursor = encodeCustomerCursor(key, last.ID)
		list = list[:f.Limit]
	}
	for _, r := range list {
		page.Items = append(page.Items, r.CustomerRow)
	}
	return page, nil
}

var nonPhone = regexp.MustCompile(`[^0-9+\-().\s]`)

// customerSearchSQL busca por nombre, mail, documento y por el nombre de
// cualquier pasajero de sus viajes, sin tildes ni mayúsculas. Si lo escrito
// parece un teléfono —solo dígitos y separadores—, busca también por los
// dígitos del teléfono del cliente y de sus contactos: «4788 2210» encuentra
// «11 4788-2210» y «+5491147882210».
func customerSearchSQL(q *query, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "true"
	}
	like := q.arg("%" + foldReplacer.Replace(strings.ToLower(text)) + "%")
	conds := []string{
		fold("cu.name") + " LIKE " + like,
		"lower(coalesce(cu.email, '')) LIKE " + like,
		fmt.Sprintf(`EXISTS (SELECT 1 FROM passengers p JOIN trips t ON t.id = p.trip_id
		  WHERE t.customer_id = cu.id AND %s LIKE %s)`, fold("p.first_name || ' ' || p.last_name"), like),
	}
	if doc := alnum(text); doc != "" {
		conds = append(conds, fmt.Sprintf(`regexp_replace(lower(coalesce(cu.document_number, '')), '[^a-z0-9]', '', 'g') LIKE %s`,
			q.arg("%"+doc+"%")))
	}
	if digits := onlyDigits(text); len(digits) >= 3 && !nonPhone.MatchString(text) {
		d := q.arg("%" + digits + "%")
		conds = append(conds,
			fmt.Sprintf(`regexp_replace(coalesce(cu.phone_number, ''), '\D', '', 'g') LIKE %s`, d),
			fmt.Sprintf(`EXISTS (SELECT 1 FROM customer_contacts sc JOIN contacts sct ON sct.id = sc.contact_id
			  WHERE sc.customer_id = cu.id AND regexp_replace(coalesce(sct.phone_number, ''), '\D', '', 'g') LIKE %s)`, d))
	}
	return "(" + strings.Join(conds, " OR ") + ")"
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func alnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// lastEight son los últimos 8 dígitos de un teléfono: lo que coincide entre el
// que se escribe a mano («11 4788-2210», «011 15 4788-2210») y el que llega de
// WhatsApp en E.164. Vacío si no llega a 8.
func lastEight(phone string) string {
	digits := onlyDigits(phone)
	if len(digits) < 8 {
		return ""
	}
	return digits[len(digits)-8:]
}

const lastEightSQL = `right(regexp_replace(coalesce(%s, ''), '\D', '', 'g'), 8)`

func encodeCustomerCursor(key, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(key + "|" + id))
}

func decodeCustomerCursor(cursor string) (string, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	i := strings.LastIndex(string(raw), "|")
	if i < 0 {
		return "", "", fmt.Errorf("cursor sin cliente")
	}
	return string(raw[:i]), string(raw[i+1:]), nil
}

// --- Repetidos y vínculos ----------------------------------------------------

// matchColumns es un cliente como se muestra en un aviso de repetido o en una
// sugerencia de vínculo.
var matchColumns = `cu.id, cu.name, coalesce(cu.phone_number, ''), coalesce(cu.email, ''),
	(SELECT count(*)::int FROM trips t WHERE t.customer_id = cu.id),
	coalesce((SELECT cv.id::text FROM customer_contacts cc
	            JOIN conversations cv ON cv.contact_id = cc.contact_id AND NOT cv.is_group
	           WHERE cc.customer_id = cu.id
	           ORDER BY cv.last_message_at DESC NULLS LAST LIMIT 1), '')`

func scanMatch(row pgx.CollectableRow) (store.CustomerMatch, error) {
	var m store.CustomerMatch
	err := row.Scan(&m.ID, &m.Name, &m.PhoneNumber, &m.Email, &m.TripCount, &m.ConversationID)
	return m, err
}

// nameSimilarity es el umbral de trigramas para «el nombre se parece». Con 0.35,
// «Lucia Mendez» encuentra a «Lucía Méndez» y a «Lucía Méndez Paz», y no a
// «Lucía Gómez».
const nameSimilarity = 0.35

// SimilarCustomers devuelve hasta 3 clientes que podrían ser el que se está
// cargando: nombre parecido, mismo teléfono, mismo mail o mismo documento.
// Primero los que coinciden por un dato exacto, después por parecido del nombre.
func (s *Store) SimilarCustomers(ctx context.Context, sq store.SimilarQuery) ([]store.CustomerMatch, error) {
	q := &query{}
	exact := []string{}
	if phone := lastEight(sq.PhoneNumber); phone != "" {
		p := q.arg(phone)
		exact = append(exact,
			fmt.Sprintf(lastEightSQL, "cu.phone_number")+" = "+p,
			fmt.Sprintf(`EXISTS (SELECT 1 FROM customer_contacts sc JOIN contacts sct ON sct.id = sc.contact_id
			  WHERE sc.customer_id = cu.id AND %s = %s)`, fmt.Sprintf(lastEightSQL, "sct.phone_number"), p))
	}
	if email := strings.ToLower(strings.TrimSpace(sq.Email)); email != "" {
		exact = append(exact, "lower(cu.email) = "+q.arg(email))
	}
	if doc := alnum(sq.DocumentNumber); doc != "" {
		exact = append(exact, `regexp_replace(lower(coalesce(cu.document_number, '')), '[^a-z0-9]', '', 'g') = `+q.arg(doc))
	}
	exactSQL := "false"
	if len(exact) > 0 {
		exactSQL = "(" + strings.Join(exact, " OR ") + ")"
	}
	similarSQL := "0::real"
	if name := strings.TrimSpace(sq.Name); len([]rune(name)) >= 3 {
		similarSQL = fmt.Sprintf("similarity(%s, %s)", fold("cu.name"), q.arg(foldReplacer.Replace(strings.ToLower(name))))
	}
	if exactSQL == "false" && similarSQL == "0::real" {
		return []store.CustomerMatch{}, nil
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(
		`SELECT %[1]s FROM (
		   SELECT cu.*, %[2]s AS exact, %[3]s AS sim FROM customers cu
		 ) cu
		 WHERE cu.exact OR cu.sim >= %[4]g
		 ORDER BY cu.exact DESC, cu.sim DESC, cu.name
		 LIMIT 3`, matchColumns, exactSQL, similarSQL, nameSimilarity), q.args...)
	if err != nil {
		return nil, fmt.Errorf("buscando clientes parecidos: %w", err)
	}
	matches, err := pgx.CollectRows(rows, scanMatch)
	if err != nil {
		return nil, fmt.Errorf("armando clientes parecidos: %w", err)
	}
	return matches, nil
}

// ContactCustomer es el cliente de un contacto, o vacío si todavía no es de
// nadie.
func (s *Store) ContactCustomer(ctx context.Context, contactID string) (string, error) {
	var customerID string
	err := s.pool.QueryRow(ctx,
		`SELECT customer_id::text FROM customer_contacts WHERE contact_id = $1`, contactID).Scan(&customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", writeError("buscando el cliente del contacto", err)
	}
	return customerID, nil
}

// SuggestedCustomers son los clientes que podrían ser el dueño de un contacto
// que todavía no es de nadie: los que tienen un teléfono con los mismos últimos
// 8 dígitos. Un contacto que ya es de un cliente no sugiere nada: vincularlo
// sería unir dos clientes.
func (s *Store) SuggestedCustomers(ctx context.Context, contactID string) ([]store.CustomerMatch, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+matchColumns+`
		   FROM contacts ct
		   JOIN customers cu ON length(regexp_replace(coalesce(ct.phone_number, ''), '\D', '', 'g')) >= 8
		                    AND `+fmt.Sprintf(lastEightSQL, "cu.phone_number")+` = `+fmt.Sprintf(lastEightSQL, "ct.phone_number")+`
		  WHERE ct.id = $1
		    AND NOT EXISTS (SELECT 1 FROM customer_contacts cc WHERE cc.contact_id = ct.id)
		  ORDER BY cu.name
		  LIMIT 3`, contactID)
	if err != nil {
		return nil, fmt.Errorf("buscando clientes para el contacto: %w", err)
	}
	matches, err := pgx.CollectRows(rows, scanMatch)
	if err != nil {
		return nil, fmt.Errorf("armando clientes para el contacto: %w", err)
	}
	return matches, nil
}

// FreeContacts busca contactos que todavía no son de ningún cliente y tienen un
// chat individual. Con un cliente dado, primero los que coinciden con su
// teléfono; después, el chat más reciente.
func (s *Store) FreeContacts(ctx context.Context, text, customerID string) ([]store.FreeContact, error) {
	q := &query{}
	conds := []string{"NOT EXISTS (SELECT 1 FROM customer_contacts cc WHERE cc.contact_id = ct.id)"}
	if text = strings.TrimSpace(text); text != "" {
		like := q.arg("%" + foldReplacer.Replace(strings.ToLower(text)) + "%")
		search := []string{fold(contactName) + " LIKE " + like}
		if digits := onlyDigits(text); len(digits) >= 3 {
			search = append(search, fmt.Sprintf(`regexp_replace(coalesce(ct.phone_number, ''), '\D', '', 'g') LIKE %s`,
				q.arg("%"+digits+"%")))
		}
		conds = append(conds, "("+strings.Join(search, " OR ")+")")
	}
	order := "conv.last_message_at DESC NULLS LAST"
	if customerID != "" {
		order = fmt.Sprintf(`coalesce((SELECT %s = %s AND length(%s) = 8 FROM customers cu WHERE cu.id = %s::uuid), false) DESC, `,
			fmt.Sprintf(lastEightSQL, "cu.phone_number"), fmt.Sprintf(lastEightSQL, "ct.phone_number"),
			fmt.Sprintf(lastEightSQL, "cu.phone_number"), q.arg(customerID)) + order
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(
		`SELECT ct.id, %[1]s, coalesce(ct.phone_number, ''), conv.id, conv.owner_label, conv.last_message_at
		   FROM contacts ct
		   JOIN LATERAL (
		     SELECT cv.id, i.owner_label, cv.last_message_at
		       FROM conversations cv JOIN instances i ON i.id = cv.instance_id
		      WHERE cv.contact_id = ct.id AND NOT cv.is_group
		      ORDER BY cv.last_message_at DESC NULLS LAST, cv.created_at DESC
		      LIMIT 1
		   ) conv ON true
		  WHERE %[2]s
		  ORDER BY %[3]s
		  LIMIT 20`, contactName, strings.Join(conds, " AND "), order), q.args...)
	if err != nil {
		return nil, fmt.Errorf("buscando contactos sin cliente: %w", err)
	}
	contacts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.FreeContact, error) {
		var c store.FreeContact
		err := row.Scan(&c.ContactID, &c.Name, &c.PhoneNumber, &c.ConversationID, &c.InstanceLabel, &c.LastMessageAt)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("armando contactos sin cliente: %w", err)
	}
	return contacts, nil
}
