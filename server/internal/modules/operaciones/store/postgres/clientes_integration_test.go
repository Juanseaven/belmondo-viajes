package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/evaluacion"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// La pantalla de Clientes contra Postgres real: el cliente cargado a mano, los
// controles sin chat, la lista, los repetidos, los vínculos y traer pasajeros.

func newCustomer(t *testing.T, s *Store, user string, n store.NewCustomer) store.Customer {
	t.Helper()
	n.CreatedBy = user
	c, err := s.CreateCustomer(t.Context(), n)
	if err != nil {
		t.Fatalf("alta de %s: %v", n.Name, err)
	}
	return c
}

func tripFor(t *testing.T, s *Store, user, customerID, destination string) store.Trip {
	t.Helper()
	trip, err := s.CreateTrip(t.Context(), store.NewTrip{CustomerID: customerID, Destination: destination,
		DepartAt: fecha("2026-12-10"), ReturnAt: fecha("2026-12-24"), CreatedBy: user})
	if err != nil {
		t.Fatalf("viaje de %s: %v", customerID, err)
	}
	return trip
}

func contactWithPhone(t *testing.T, ctx context.Context, p *pgxpool.Pool, jid, name, phone string) string {
	t.Helper()
	var id string
	if err := p.QueryRow(ctx,
		`INSERT INTO contacts (wa_jid, push_name, phone_number) VALUES ($1, $2, $3) RETURNING id`,
		jid, name, phone).Scan(&id); err != nil {
		t.Fatalf("sembrando contacto: %v", err)
	}
	return id
}

func TestUnClienteSinChatTieneViajeConTodosSusControles(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")

	marta := newCustomer(t, s, user, store.NewCustomer{Name: "Marta Pérez", PhoneNumber: "11 4788-2210", DocumentType: "DNI"})
	if marta.HasChat || len(marta.ContactIDs) != 0 {
		t.Fatalf("cliente a mano = %+v", marta)
	}
	trip := tripFor(t, s, user, marta.ID, "Japón")
	if trip.CustomerID != marta.ID {
		t.Fatalf("el viaje es de %s, se esperaba %s", trip.CustomerID, marta.ID)
	}
	var controles int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM controls WHERE trip_id = $1`, trip.ID).Scan(&controles); err != nil || controles == 0 {
		t.Fatalf("controles = %d, %v", controles, err)
	}

	_, err := s.CreateTrip(ctx, store.NewTrip{CustomerID: "00000000-0000-0000-0000-000000000000", CreatedBy: user})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cliente inexistente: %v", err)
	}
}

// Sin chat no hay dónde mandar el mensaje que prueba un control por acción: se
// marca a mano. Con chat, el mensaje sigue siendo la prueba.
func TestUnControlPorAccionSeMarcaAManoSoloSinChat(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")

	sinChat := newCustomer(t, s, user, store.NewCustomer{Name: "Marta Pérez"})
	trip := tripFor(t, s, user, sinChat.ID, "Japón")
	vouchers := control(t, ctx, pool, trip.ID, "carga-vouchers")
	if err := s.CloseControl(ctx, store.CloseControl{ControlID: vouchers, UserID: user}); err != nil {
		t.Fatalf("sin chat se marca a mano: %v", err)
	}
	var cerradoPor *string
	var mensaje *string
	if err := pool.QueryRow(ctx, `SELECT closed_by::text, message_id::text FROM control_evidence WHERE control_id = $1`,
		vouchers).Scan(&cerradoPor, &mensaje); err != nil {
		t.Fatalf("evidencia: %v", err)
	}
	if cerradoPor == nil || *cerradoPor != user || mensaje != nil {
		t.Fatalf("evidencia = %v, %v; se esperaba quién y sin mensaje", cerradoPor, mensaje)
	}

	// Un contacto vinculado sin conversación, o con una de grupo, no es un chat.
	alias := seedContact(t, ctx, pool, "alias@lid", "Marta alias")
	if err := s.LinkContact(ctx, sinChat.ID, alias, user); err != nil {
		t.Fatal(err)
	}
	grupo := seedContact(t, ctx, pool, "familia@g.us", "Familia Pérez")
	if err := s.LinkContact(ctx, sinChat.ID, grupo, user); err != nil {
		t.Fatal(err)
	}
	line := seedInstance(t, ctx, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO conversations (instance_id, contact_id, remote_jid, is_group)
		VALUES ($1, $2, 'familia@g.us', true)`, line, grupo); err != nil {
		t.Fatal(err)
	}
	reembolso := control(t, ctx, pool, trip.ID, "control-no-reembolsable")
	if err := s.CloseControl(ctx, store.CloseControl{ControlID: reembolso, UserID: user}); err != nil {
		t.Fatalf("un alias o un grupo no son un chat: %v", err)
	}

	// Con una conversación individual, pasa a pedir el mensaje.
	seedConversation(t, ctx, pool, line, alias)
	flight := control(t, ctx, pool, trip.ID, "control-flight-plus")
	err := s.CloseControl(ctx, store.CloseControl{ControlID: flight, UserID: user})
	if !errors.Is(err, store.ErrEvidenciaRequerida) {
		t.Fatalf("con chat se pide el mensaje: %v", err)
	}
	if detail, err := s.TripDetail(ctx, trip.ID); err != nil || !detail.Customer.HasChat {
		t.Fatalf("el viaje tiene que decir que el cliente tiene chat: %v %v", detail.Customer.HasChat, err)
	}
}

// Marcar a mano «Requisitos migratorios» guarda qué se avisó, como al mandar el
// mensaje: un requisito nuevo lo reabre.
func TestMarcarAManoLosRequisitosGuardaLoAvisado(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")
	cliente := newCustomer(t, s, user, store.NewCustomer{Name: "Marta Pérez"})
	trip := tripFor(t, s, user, cliente.ID, "Japón")
	ana, err := s.AddPassenger(ctx, store.NewPassenger{TripID: trip.ID, FirstName: "Ana", LastName: "Pérez",
		Kind: "adulto", DocumentType: "Pasaporte", DocumentExpiresAt: ptrTime(fecha("2030-01-01")), CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRequirements(ctx, trip.ID, []evaluacion.Requirement{req(ana.ID, "JP", evaluacion.KeyVisa, evaluacion.Falta)}, nil); err != nil {
		t.Fatal(err)
	}
	visa := control(t, ctx, pool, trip.ID, "control-visa")
	if err := s.CloseControl(ctx, store.CloseControl{ControlID: visa, UserID: user}); err != nil {
		t.Fatalf("marcando requisitos: %v", err)
	}
	var notified *string
	if err := pool.QueryRow(ctx, `SELECT notified::text FROM control_evidence WHERE control_id = $1`, visa).Scan(&notified); err != nil {
		t.Fatal(err)
	}
	if notified == nil || *notified == "[]" {
		t.Fatalf("notified = %v, se esperaba el requisito avisado", notified)
	}
}

func TestLaListaBuscaPorTelefonoPasajeroYSinTildes(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")

	marta := newCustomer(t, s, user, store.NewCustomer{Name: "Marta Pérez", PhoneNumber: "11 4788-2210", DocumentNumber: "22.456.789"})
	trip := tripFor(t, s, user, marta.ID, "Japón")
	if _, err := s.AddPassenger(ctx, store.NewPassenger{TripID: trip.ID, FirstName: "Tomás", LastName: "Gómez",
		Kind: "menor", CreatedBy: user}); err != nil {
		t.Fatal(err)
	}
	lucia := newCustomer(t, s, user, store.NewCustomer{Name: "Lucía Méndez", Email: "lucia@mail.com"})
	contacto := contactWithPhone(t, ctx, pool, "5491160127788@s.whatsapp.net", "Lu", "+5491160127788")
	if err := s.LinkContact(ctx, lucia.ID, contacto, user); err != nil {
		t.Fatal(err)
	}
	seedConversation(t, ctx, pool, seedInstance(t, ctx, pool), contacto)
	newCustomer(t, s, user, store.NewCustomer{Name: "Ricardo Torres"})

	find := func(q string) []string {
		t.Helper()
		page, err := s.ListCustomers(ctx, store.CustomerFilter{Query: q})
		if err != nil {
			t.Fatalf("buscando %q: %v", q, err)
		}
		names := []string{}
		for _, row := range page.Items {
			names = append(names, row.Name)
		}
		return names
	}
	for q, want := range map[string]string{
		"4788 2210": "Marta Pérez", "+54 9 11 4788-2210": "", "1147882210": "Marta Pérez",
		"6012-7788": "Lucía Méndez", "tomas": "Marta Pérez", "MENDEZ": "Lucía Méndez",
		"22456789": "Marta Pérez", "lucia@": "Lucía Méndez",
	} {
		got := find(q)
		if want == "" {
			// El teléfono a mano no tiene el 54 9: el número internacional entero
			// no lo contiene. Lo encuentran los dígitos que sí escribió.
			if len(got) != 0 {
				t.Errorf("%q = %v, se esperaba ninguno", q, got)
			}
			continue
		}
		if len(got) != 1 || got[0] != want {
			t.Errorf("%q = %v, se esperaba %s", q, got, want)
		}
	}

	page, err := s.ListCustomers(ctx, store.CustomerFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || page.ByChip["con-viaje"] != 1 || page.ByChip["sin-viajes"] != 2 || page.ByChip["sin-whatsapp"] != 2 {
		t.Fatalf("chips = %v, total = %d", page.ByChip, page.Total)
	}
	for _, row := range page.Items {
		switch row.Name {
		case "Lucía Méndez":
			if row.ChatCount != 1 || row.ConversationID == "" || row.ContactPhone != "+5491160127788" {
				t.Errorf("Lucía = %+v", row)
			}
		case "Marta Pérez":
			if row.TripCount != 1 || row.OpenTrip == nil || row.OpenTrip.Destination != "Japón" || row.ChatCount != 0 {
				t.Errorf("Marta = %+v", row)
			}
		}
	}

	conViaje, err := s.ListCustomers(ctx, store.CustomerFilter{Chip: store.ChipSinWhatsApp, Query: "marta"})
	if err != nil {
		t.Fatal(err)
	}
	if conViaje.Total != 1 || len(conViaje.Items) != 1 || conViaje.ByChip["todos"] != 1 {
		t.Fatalf("chip con búsqueda = %+v", conViaje)
	}
}

// El último movimiento sube a un cliente cuando se trabaja en sus viajes, no
// solo cuando se edita el viaje.
func TestElUltimoMovimientoSigueAlTrabajoEnLosViajes(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")
	first := func() string {
		t.Helper()
		page, err := s.ListCustomers(ctx, store.CustomerFilter{})
		if err != nil {
			t.Fatal(err)
		}
		return page.Items[0].Name
	}

	viejo := newCustomer(t, s, user, store.NewCustomer{Name: "Viejo"})
	trip := tripFor(t, s, user, viejo.ID, "Salta")
	newCustomer(t, s, user, store.NewCustomer{Name: "Nuevo"})
	// Todo lo del viejo pasa antes de ayer, para que el nuevo quede primero.
	// Sin los disparadores, que pondrían updated_at en ahora.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE customers DISABLE TRIGGER customers_touch;
		ALTER TABLE trips DISABLE TRIGGER trips_touch;
		UPDATE customers SET created_at = now() - interval '2 days', updated_at = now() - interval '2 days' WHERE name = 'Viejo';
		UPDATE trips SET created_at = now() - interval '2 days', updated_at = now() - interval '2 days';
		ALTER TABLE customers ENABLE TRIGGER customers_touch;
		ALTER TABLE trips ENABLE TRIGGER trips_touch`); err != nil {
		t.Fatal(err)
	}
	if got := first(); got != "Nuevo" {
		t.Fatalf("primero = %s, se esperaba Nuevo", got)
	}

	if _, err := s.AddPassenger(ctx, store.NewPassenger{TripID: trip.ID, FirstName: "Ana", LastName: "Viejo",
		Kind: "adulto", CreatedBy: user}); err != nil {
		t.Fatal(err)
	}
	if got := first(); got != "Viejo" {
		t.Fatalf("un pasajero nuevo no subió al cliente: primero = %s", got)
	}

	nuevo := newCustomer(t, s, user, store.NewCustomer{Name: "Más nuevo"})
	_ = nuevo
	if got := first(); got != "Más nuevo" {
		t.Fatalf("primero = %s", got)
	}
	usd := 1000.0
	if _, err := s.PutPayment(ctx, store.Payment{TripID: trip.ID, TotalUSD: &usd, UpdatedBy: user}); err != nil {
		t.Fatal(err)
	}
	if got := first(); got != "Viejo" {
		t.Fatalf("un pago no subió al cliente: primero = %s", got)
	}

	newCustomer(t, s, user, store.NewCustomer{Name: "Último"})
	if err := s.CloseControl(ctx, store.CloseControl{ControlID: control(t, ctx, pool, trip.ID, "carga-vouchers"), UserID: user}); err != nil {
		t.Fatal(err)
	}
	if got := first(); got != "Viejo" {
		t.Fatalf("un control cerrado no subió al cliente: primero = %s", got)
	}
}

func TestLaListaPaginaSinRepetirEnLosDosOrdenes(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")
	for _, name := range []string{"Ana", "Beto", "Carla", "Dario", "Ema"} {
		newCustomer(t, s, user, store.NewCustomer{Name: name})
	}
	for _, sort := range []store.CustomerSort{store.SortMovimiento, store.SortNombre} {
		seen := map[string]bool{}
		order := []string{}
		cursor := ""
		for {
			page, err := s.ListCustomers(ctx, store.CustomerFilter{Sort: sort, Cursor: cursor, Limit: 2})
			if err != nil {
				t.Fatalf("%s: %v", sort, err)
			}
			for _, row := range page.Items {
				if seen[row.ID] {
					t.Fatalf("%s: %s repetido", sort, row.Name)
				}
				seen[row.ID] = true
				order = append(order, row.Name)
			}
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		if len(order) != 5 {
			t.Fatalf("%s: %v", sort, order)
		}
		if sort == store.SortNombre && (order[0] != "Ana" || order[4] != "Ema") {
			t.Fatalf("por nombre = %v", order)
		}
		if sort == store.SortMovimiento && order[0] != "Ema" {
			t.Fatalf("por movimiento = %v", order)
		}
	}
}

func TestElAvisoDeRepetidoEncuentraPorCadaDato(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")
	lucia := newCustomer(t, s, user, store.NewCustomer{Name: "Lucía Méndez", PhoneNumber: "011 15 6012-7788",
		Email: "Lucia@Mail.com", DocumentNumber: "30.111.222"})
	newCustomer(t, s, user, store.NewCustomer{Name: "Lucía Gómez"})

	for name, q := range map[string]store.SimilarQuery{
		"nombre":    {Name: "Lucia Mendez"},
		"teléfono":  {Name: "LM", PhoneNumber: "+54 9 11 6012-7788"},
		"mail":      {Email: "lucia@mail.com "},
		"documento": {DocumentNumber: "30111222"},
	} {
		matches, err := s.SimilarCustomers(ctx, q)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(matches) == 0 || matches[0].ID != lucia.ID {
			t.Errorf("%s: %+v", name, matches)
		}
	}
	matches, err := s.SimilarCustomers(ctx, store.SimilarQuery{Name: "Ricardo Torres"})
	if err != nil || len(matches) != 0 {
		t.Fatalf("nadie se parece a Ricardo: %+v %v", matches, err)
	}
}

func TestVincularSugiereYSoloOfreceChatsLibres(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")
	marta := newCustomer(t, s, user, store.NewCustomer{Name: "Marta Pérez", PhoneNumber: "11 4788-2210"})
	line := seedInstance(t, ctx, pool)

	libre := contactWithPhone(t, ctx, pool, "5491147882210@s.whatsapp.net", "Marta", "+5491147882210")
	seedConversation(t, ctx, pool, line, libre)
	sinChat := contactWithPhone(t, ctx, pool, "otro@lid", "Directorio", "+5491147882210")
	otroCliente := newCustomer(t, s, user, store.NewCustomer{Name: "Beto"})
	tomado := contactWithPhone(t, ctx, pool, "5491100000001@s.whatsapp.net", "Beto", "+5491100000001")
	seedConversation(t, ctx, pool, line, tomado)
	if err := s.LinkContact(ctx, otroCliente.ID, tomado, user); err != nil {
		t.Fatal(err)
	}

	suggested, err := s.SuggestedCustomers(ctx, libre)
	if err != nil || len(suggested) != 1 || suggested[0].ID != marta.ID {
		t.Fatalf("sugerencia = %+v, %v", suggested, err)
	}
	if suggested, err := s.SuggestedCustomers(ctx, tomado); err != nil || len(suggested) != 0 {
		t.Fatalf("un contacto con cliente no sugiere: %+v, %v", suggested, err)
	}

	free, err := s.FreeContacts(ctx, "", marta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(free) != 1 || free[0].ContactID != libre || free[0].ConversationID == "" || free[0].InstanceLabel != "Juanse" {
		t.Fatalf("contactos libres = %+v (sin chat: %s)", free, sinChat)
	}
	if found, err := s.FreeContacts(ctx, "4788", ""); err != nil || len(found) != 1 {
		t.Fatalf("buscando por teléfono = %+v, %v", found, err)
	}

	if owner, err := s.ContactCustomer(ctx, libre); err != nil || owner != "" {
		t.Fatalf("un chat libre no es de nadie: %q, %v", owner, err)
	}
	if err := s.LinkContact(ctx, marta.ID, libre, user); err != nil {
		t.Fatal(err)
	}
	if owner, err := s.ContactCustomer(ctx, libre); err != nil || owner != marta.ID {
		t.Fatalf("el chat vinculado es de %q, se esperaba %s: %v", owner, marta.ID, err)
	}
	if err := s.LinkContact(ctx, otroCliente.ID, libre, user); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("vincular un contacto que ya es de otro cliente: %v", err)
	}
	detail, err := s.CustomerDetail(ctx, marta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.HasChat || len(detail.Contacts) != 1 || detail.Contacts[0].ConversationID == "" {
		t.Fatalf("página = %+v", detail)
	}
}

func TestTraerPasajerosCopiaTodoYNoTocaElOriginal(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")
	marta := newCustomer(t, s, user, store.NewCustomer{Name: "Marta Pérez"})

	peru := tripFor(t, s, user, marta.ID, "Perú")
	elenaPeru, err := s.AddPassenger(ctx, store.NewPassenger{TripID: peru.ID, FirstName: "Elena", LastName: "Pérez",
		Kind: "adulto", CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE trips SET created_at = now() - interval '1 year' WHERE id = $1`, peru.ID); err != nil {
		t.Fatal(err)
	}

	japon := tripFor(t, s, user, marta.ID, "Japón")
	martaPax, err := s.AddPassenger(ctx, store.NewPassenger{TripID: japon.ID, FirstName: "Marta", LastName: "Pérez",
		Kind: "adulto", IsHolder: true, DocumentType: "Pasaporte", DocumentCountry: "AR", DocumentNumber: "AA1",
		DocumentExpiresAt: ptrTime(fecha("2031-08-01")), CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	italiano, err := s.AddDocument(ctx, store.NewDocument{PassengerID: martaPax.ID, Type: "Pasaporte", Country: "IT", Number: "IT9"})
	if err != nil {
		t.Fatal(err)
	}
	manual := italiano.ID
	if _, err := s.UpdatePassenger(ctx, martaPax.ID, store.PassengerPatch{TravelDocumentID: &manual}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddPermit(ctx, store.NewPermit{PassengerID: martaPax.ID, Kind: "visa", Country: "US", Label: "B1/B2"}); err != nil {
		t.Fatal(err)
	}
	tomas, err := s.AddPassenger(ctx, store.NewPassenger{TripID: japon.ID, FirstName: "Tomás", LastName: "Gómez",
		Kind: "menor", CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	// Elena viajó a Perú y también a Japón: la versión de Japón es la vigente.
	elenaJapon, err := s.AddPassenger(ctx, store.NewPassenger{TripID: japon.ID, FirstName: "elena ", LastName: "PEREZ",
		Kind: "adulto", CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRequirements(ctx, japon.ID, []evaluacion.Requirement{req(martaPax.ID, "JP", evaluacion.KeyVisa, evaluacion.Verificar)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(ctx, store.NewConfirmation{TripID: japon.ID, PassengerID: martaPax.ID, Country: "JP",
		Key: evaluacion.KeyVisa, UserID: user}); err != nil {
		t.Fatal(err)
	}

	chile := tripFor(t, s, user, marta.ID, "Chile")
	// Chile ya tiene titular: Marta llega sin titularidad.
	if _, err := s.AddPassenger(ctx, store.NewPassenger{TripID: chile.ID, FirstName: "Tomás", LastName: "Gomez",
		Kind: "menor", IsHolder: true, CreatedBy: user}); err != nil {
		t.Fatal(err)
	}

	candidates, err := s.TripCompanions(ctx, chile.ID)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]store.Companion{}
	for _, c := range candidates {
		byName[personKey(c.FirstName, c.LastName)] = c
	}
	if len(candidates) != 3 {
		t.Fatalf("candidatos = %+v", candidates)
	}
	if c := byName["elena perez"]; c.PassengerID != elenaJapon.ID || c.TripCount != 2 || !c.Selected {
		t.Errorf("Elena = %+v, se esperaba la versión de Japón, en dos viajes y marcada", c)
	}
	if c := byName["tomas gomez"]; !c.InTrip || c.Selected || c.PassengerID != tomas.ID {
		t.Errorf("Tomás ya está: %+v", c)
	}
	if c := byName["marta perez"]; c.Document == nil || c.Document.Country != "IT" || c.PermitCount != 1 || !c.Selected {
		t.Errorf("Marta = %+v", c)
	}
	_ = elenaPeru

	copied, err := s.CopyPassengers(ctx, chile.ID, []string{martaPax.ID, tomas.ID, elenaJapon.ID}, user)
	if err != nil {
		t.Fatalf("trayendo: %v", err)
	}
	if len(copied) != 2 {
		t.Fatalf("copiados = %+v; Tomás ya estaba", copied)
	}
	copia := copied[0]
	if copia.IsHolder || copia.FirstName != "Marta" || len(copia.Documents) != 2 || len(copia.Permits) != 1 {
		t.Fatalf("copia de Marta = %+v", copia)
	}
	var viaja store.Document
	for _, d := range copia.Documents {
		if d.ID == copia.TravelDocumentID {
			viaja = d
		}
		if d.ID == italiano.ID {
			t.Fatalf("la copia apunta al documento original")
		}
	}
	if viaja.Country != "IT" || !copia.TravelDocumentManual {
		t.Fatalf("documento de viaje de la copia = %+v (manual %v)", viaja, copia.TravelDocumentManual)
	}
	var confirmaciones int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM requirement_confirmations WHERE trip_id = $1`, chile.ID).Scan(&confirmaciones); err != nil || confirmaciones != 0 {
		t.Fatalf("confirmaciones copiadas = %d, %v", confirmaciones, err)
	}

	nuevo := "Martita"
	if _, err := s.UpdatePassenger(ctx, copia.ID, store.PassengerPatch{FirstName: &nuevo}); err != nil {
		t.Fatal(err)
	}
	original, err := readPassenger(ctx, pool, martaPax.ID)
	if err != nil || original.FirstName != "Marta" {
		t.Fatalf("editar la copia tocó el original: %+v %v", original, err)
	}

	if _, err := s.CopyPassengers(ctx, chile.ID, []string{copia.ID}, user); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("traer del mismo viaje: %v", err)
	}
	otro := newCustomer(t, s, user, store.NewCustomer{Name: "Otro"})
	ajeno := tripFor(t, s, user, otro.ID, "Brasil")
	if _, err := s.CopyPassengers(ctx, ajeno.ID, []string{martaPax.ID}, user); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("traer de otro cliente: %v", err)
	}

	detail, err := s.CustomerDetail(ctx, marta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Trips) != 3 || len(detail.Companions) != 4 {
		t.Fatalf("página: %d viajes, %d personas (%+v)", len(detail.Trips), len(detail.Companions), detail.Companions)
	}
}

// En un viaje sin titular, el primer titular que llega queda; el segundo no.
func TestTraerDosTitularesDejaElPrimero(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "oficina@agencia.test")
	cliente := newCustomer(t, s, user, store.NewCustomer{Name: "Marta"})
	a := tripFor(t, s, user, cliente.ID, "A")
	b := tripFor(t, s, user, cliente.ID, "B")
	uno, err := s.AddPassenger(ctx, store.NewPassenger{TripID: a.ID, FirstName: "Uno", LastName: "X", Kind: "adulto", IsHolder: true, CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	dos, err := s.AddPassenger(ctx, store.NewPassenger{TripID: b.ID, FirstName: "Dos", LastName: "X", Kind: "adulto", IsHolder: true, CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	c := tripFor(t, s, user, cliente.ID, "C")
	copied, err := s.CopyPassengers(ctx, c.ID, []string{dos.ID, uno.ID}, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 2 || !copied[0].IsHolder || copied[1].IsHolder {
		t.Fatalf("titulares = %v, %v", copied[0].IsHolder, copied[1].IsHolder)
	}
	_ = pool
}
