package postgres

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// La pantalla de Viajes contra el esquema real. «Hoy» llega fijo: la regla de
// la hora de Argentina se prueba en el handler, que es quien la calcula.

var listToday = fecha("2026-11-02")

func listDay(offset int) time.Time { return listToday.AddDate(0, 0, offset) }

type listFixture struct {
	ctx  context.Context
	pool *pgxpool.Pool
	s    *Store
	user string
	n    int
}

func newListFixture(t *testing.T) *listFixture {
	ctx, pool := testDB(t)
	return &listFixture{ctx: ctx, pool: pool, s: New(pool), user: seedUser(t, ctx, pool, "listas@agencia.test")}
}

// trip crea un viaje con todos sus controles cerrados, para que cada test abra
// solo los que quiere mirar.
func (f *listFixture) trip(t *testing.T, name, destination string, depart *time.Time) store.Trip {
	t.Helper()
	f.n++
	contact := seedContact(t, f.ctx, f.pool, name+"@s.whatsapp.net", name)
	n := store.NewTrip{ContactID: contact, Destination: destination, CreatedBy: f.user}
	if depart != nil {
		n.DepartAt = *depart
		n.ReturnAt = depart.AddDate(0, 0, 7)
	}
	trip, err := f.s.CreateTrip(f.ctx, n)
	if err != nil {
		t.Fatalf("alta de %s: %v", name, err)
	}
	// El control de documentos sigue al dato aunque sea por acción: sin
	// pasajeros, el próximo recálculo lo reabriría. Se le quita el dato para que
	// solo cuente lo que cada test abre a mano.
	if _, err := f.pool.Exec(f.ctx,
		`UPDATE controls SET status = 'cerrado',
		        derives = CASE WHEN closing = 'derivado' THEN derives END
		  WHERE trip_id = $1`, trip.ID); err != nil {
		t.Fatalf("cerrando controles: %v", err)
	}
	return trip
}

func (f *listFixture) open(t *testing.T, tripID, itemID string, due time.Time) {
	t.Helper()
	tag, err := f.pool.Exec(f.ctx,
		`UPDATE controls SET status = 'abierto', due_at = $3 WHERE trip_id = $1 AND item_id = $2`,
		tripID, itemID, due)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("abriendo %s: %v", itemID, err)
	}
}

func (f *listFixture) pending(t *testing.T) map[string]store.PendingRow {
	t.Helper()
	rows, err := f.s.Pending(f.ctx, listToday)
	if err != nil {
		t.Fatalf("pendientes: %v", err)
	}
	byID := map[string]store.PendingRow{}
	for _, row := range rows {
		if _, dup := byID[row.ID]; dup {
			t.Fatalf("el viaje #%d aparece dos veces", row.Number)
		}
		byID[row.ID] = row
	}
	return byID
}

func TestPendientesAgrupaPorUrgencia(t *testing.T) {
	f := newListFixture(t)
	atrasado := f.trip(t, "Atrasado", "Salta", nil)
	f.open(t, atrasado.ID, "control-visa", listDay(-1))
	semana := f.trip(t, "Semana", "Iguazú", nil)
	f.open(t, semana.ID, "control-equipaje", listDay(3))
	lejos := f.trip(t, "Lejos", "Madrid", nil)
	f.open(t, lejos.ID, "control-equipaje", listDay(20))
	salePronto := f.trip(t, "Pronto", "Mendoza", ptrTime(listDay(10)))
	saleTarde := f.trip(t, "Tarde", "Roma", ptrTime(listDay(20)))

	got := f.pending(t)

	if row := got[atrasado.ID]; row.Group != domain.GroupAtrasado || row.Task == nil || row.Task.Task != "Avisar los requisitos migratorios" {
		t.Errorf("atrasado = %#v", row)
	}
	if row := got[semana.ID]; row.Group != domain.GroupSemana {
		t.Errorf("semana = %#v", row)
	}
	if _, ok := got[lejos.ID]; ok {
		t.Errorf("un plazo a 20 días no debería aparecer")
	}
	if row := got[salePronto.ID]; row.Group != domain.GroupSalen || row.Task != nil {
		t.Errorf("sale pronto al día = %#v", row)
	}
	if _, ok := got[saleTarde.ID]; ok {
		t.Errorf("un viaje que sale en 20 días no debería aparecer")
	}
}

func TestPendientesUsaLaSenaYOlvidaLaCobrada(t *testing.T) {
	f := newListFixture(t)
	trip := f.trip(t, "Seña", "Cancún", nil)
	due := listDay(-2)
	payment := store.Payment{TripID: trip.ID, DepositUSD: float64ptr(800), DepositDueAt: &due, UpdatedBy: f.user}
	if _, err := f.s.PutPayment(f.ctx, payment); err != nil {
		t.Fatalf("pago: %v", err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE controls SET status = 'cerrado' WHERE trip_id = $1`, trip.ID); err != nil {
		t.Fatalf("cerrando controles: %v", err)
	}

	row, ok := f.pending(t)[trip.ID]
	if !ok || row.Group != domain.GroupAtrasado || row.Task.Task != "Cobrar la seña de USD 800" {
		t.Fatalf("seña vencida = %#v", row)
	}

	paid := listDay(-1)
	payment.DepositPaidAt = &paid
	if _, err := f.s.PutPayment(f.ctx, payment); err != nil {
		t.Fatalf("cobro: %v", err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE controls SET status = 'cerrado' WHERE trip_id = $1`, trip.ID); err != nil {
		t.Fatalf("cerrando controles: %v", err)
	}
	if _, ok := f.pending(t)[trip.ID]; ok {
		t.Fatalf("una seña cobrada no está pendiente")
	}
}

func TestPendientesIgnoraLosViajesCerrados(t *testing.T) {
	f := newListFixture(t)
	trip := f.trip(t, "Cerrado", "Salta", ptrTime(listDay(3)))
	f.open(t, trip.ID, "control-visa", listDay(-5))
	cerrado := domain.TripCerrado
	if _, err := f.s.UpdateTrip(f.ctx, trip.ID, store.TripPatch{Status: &cerrado}); err != nil {
		t.Fatalf("cerrando viaje: %v", err)
	}
	if _, ok := f.pending(t)[trip.ID]; ok {
		t.Fatalf("un viaje cerrado nunca está pendiente")
	}
}

// «Abrir chat» va a la conversación más reciente de cualquiera de los contactos
// del cliente, no a la del contacto con el que se abrió el viaje.
func TestLaConversacionEsLaMasRecienteDelCliente(t *testing.T) {
	f := newListFixture(t)
	trip := f.trip(t, "Ana", "Salta", nil)
	f.open(t, trip.ID, "control-visa", listDay(-1))
	var first string
	if err := f.pool.QueryRow(f.ctx, `SELECT contact_id::text FROM customer_contacts WHERE customer_id = $1`, trip.CustomerID).Scan(&first); err != nil {
		t.Fatalf("contacto: %v", err)
	}
	second := seedContact(t, f.ctx, f.pool, "ana-oficina@s.whatsapp.net", "Ana oficina")
	if err := f.s.LinkContact(f.ctx, trip.CustomerID, second, f.user); err != nil {
		t.Fatalf("vinculando: %v", err)
	}
	lineA := seedNamedInstance(t, f.ctx, f.pool, "linea-a")
	lineB := seedNamedInstance(t, f.ctx, f.pool, "linea-b")
	old := seedConversationAt(t, f.ctx, f.pool, lineA, first, time.Now().Add(-48*time.Hour))
	recent := seedConversationAt(t, f.ctx, f.pool, lineB, second, time.Now().Add(-time.Hour))
	_ = old

	if row := f.pending(t)[trip.ID]; row.ConversationID != recent {
		t.Fatalf("conversación = %q, se esperaba la más reciente %q", row.ConversationID, recent)
	}
}

func TestListadoBuscaPorPasajeroYSinTildes(t *testing.T) {
	f := newListFixture(t)
	cancun := f.trip(t, "Agus Alonso", "Cancún", nil)
	f.trip(t, "Beto", "Salta", nil)
	if _, err := f.s.AddPassenger(f.ctx, store.NewPassenger{TripID: cancun.ID, FirstName: "Tomás",
		LastName: "Pérez", Kind: "menor", CreatedBy: f.user}); err != nil {
		t.Fatalf("pasajero: %v", err)
	}

	for _, q := range []string{"cancun", "TOMAS", "perez", "agus"} {
		page, err := f.s.ListTrips(f.ctx, store.TripFilter{Query: q, Today: listToday, Limit: 50})
		if err != nil {
			t.Fatalf("buscando %q: %v", q, err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != cancun.ID {
			t.Errorf("buscando %q: %#v", q, page)
		}
	}
	for _, q := range []string{strconv.FormatInt(cancun.Number, 10), "#" + strconv.FormatInt(cancun.Number, 10)} {
		page, err := f.s.ListTrips(f.ctx, store.TripFilter{Query: q, Today: listToday, Limit: 50})
		if err != nil || page.Total != 1 || page.Items[0].ID != cancun.ID {
			t.Errorf("buscando por número %q: %#v, %v", q, page, err)
		}
	}
}

func TestListadoCombinaFiltros(t *testing.T) {
	f := newListFixture(t)
	confirmadoAtrasado := f.trip(t, "Uno", "Salta", ptrTime(listDay(10)))
	f.open(t, confirmadoAtrasado.ID, "control-visa", listDay(-1))
	confirmado := domain.TripConfirmado
	// El alta asigna como vendedor a quien crea el viaje: se reasigna uno y se
	// desasigna otro para que el filtro distinga.
	seller := seedUser(t, f.ctx, f.pool, "otra@agencia.test")
	if _, err := f.s.UpdateTrip(f.ctx, confirmadoAtrasado.ID, store.TripPatch{Status: &confirmado, SellerUserID: &seller}); err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	sinVendedor := f.trip(t, "Dos", "Salta", ptrTime(listDay(10)))
	nobody := ""
	if _, err := f.s.UpdateTrip(f.ctx, sinVendedor.ID, store.TripPatch{SellerUserID: &nobody}); err != nil {
		t.Fatalf("desasignando: %v", err)
	}
	cerrado := f.trip(t, "Tres", "Salta", ptrTime(listDay(10)))
	cerradoStatus := domain.TripCerrado
	if _, err := f.s.UpdateTrip(f.ctx, cerrado.ID, store.TripPatch{Status: &cerradoStatus}); err != nil {
		t.Fatalf("cerrando: %v", err)
	}

	cases := []struct {
		name   string
		filter store.TripFilter
		want   int
	}{
		{"abiertos", store.TripFilter{OpenOnly: true}, 2},
		{"todos", store.TripFilter{}, 3},
		{"confirmados", store.TripFilter{Statuses: []domain.TripStatus{domain.TripConfirmado}}, 1},
		{"atrasados", store.TripFilter{LateOnly: true}, 1},
		{"vendedor", store.TripFilter{SellerID: seller}, 1},
		{"sin vendedor", store.TripFilter{SellerNone: true, OpenOnly: true}, 1},
		{"salida en rango", store.TripFilter{DepartFrom: ptrTime(listDay(5)), DepartTo: ptrTime(listDay(15))}, 3},
		{"salida fuera de rango", store.TripFilter{DepartFrom: ptrTime(listDay(11))}, 0},
	}
	for _, c := range cases {
		c.filter.Today, c.filter.Limit = listToday, 50
		page, err := f.s.ListTrips(f.ctx, c.filter)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if page.Total != c.want || len(page.Items) != c.want {
			t.Errorf("%s: total %d, filas %d, se esperaban %d", c.name, page.Total, len(page.Items), c.want)
		}
	}
	// Los chips cuentan cada estado con los demás filtros, sin el de estado.
	counts, err := f.s.ListTrips(f.ctx, store.TripFilter{Statuses: []domain.TripStatus{domain.TripConfirmado}, Today: listToday, Limit: 50})
	if err != nil {
		t.Fatalf("contando: %v", err)
	}
	if got := counts.ByStatus; got["consulta"] != 1 || got["confirmado"] != 1 || got["cerrado"] != 1 || counts.Total != 1 {
		t.Errorf("por estado = %v, total %d; se esperaba uno de cada y total 1", got, counts.Total)
	}
	withSeller, _ := f.s.ListTrips(f.ctx, store.TripFilter{SellerID: seller, Today: listToday, Limit: 50})
	if got := withSeller.ByStatus; len(got) != 1 || got["confirmado"] != 1 {
		t.Errorf("por estado con vendedor = %v, se esperaba solo el confirmado", got)
	}

	page, _ := f.s.ListTrips(f.ctx, store.TripFilter{LateOnly: true, Today: listToday, Limit: 50})
	if len(page.Items) == 1 && (page.Items[0].Next == nil || page.Items[0].Next.Task != "Avisar los requisitos migratorios") {
		t.Errorf("lo que sigue = %#v", page.Items[0].Next)
	}
}

// El cursor recorre todo sin repetir ni saltear, en cada orden, incluidos los
// viajes sin fecha de salida, que van al final.
func TestListadoPaginaPorCursorEnCadaOrden(t *testing.T) {
	f := newListFixture(t)
	for i, depart := range []*time.Time{ptrTime(listDay(3)), nil, ptrTime(listDay(1)), ptrTime(listDay(3)), nil} {
		trip := f.trip(t, "Pag"+string(rune('A'+i)), "Salta", depart)
		total := float64(1000 * (i % 3))
		if _, err := f.s.PutPayment(f.ctx, store.Payment{TripID: trip.ID, TotalUSD: &total, UpdatedBy: f.user}); err != nil {
			t.Fatalf("pago: %v", err)
		}
	}

	for _, sort := range []store.TripSort{store.SortSalida, store.SortNumero, store.SortTotal} {
		seen := map[string]bool{}
		var order []store.TripRow
		cursor := ""
		for page := 0; page < 10; page++ {
			result, err := f.s.ListTrips(f.ctx, store.TripFilter{Sort: sort, Cursor: cursor, Limit: 2, Today: listToday})
			if err != nil {
				t.Fatalf("%s: %v", sort, err)
			}
			for _, row := range result.Items {
				if seen[row.ID] {
					t.Fatalf("%s: #%d repetido", sort, row.Number)
				}
				seen[row.ID] = true
				order = append(order, row)
			}
			if result.NextCursor == "" {
				break
			}
			cursor = result.NextCursor
		}
		if len(seen) != 5 {
			t.Fatalf("%s: se vieron %d viajes, se esperaban 5", sort, len(seen))
		}
		if sort == store.SortSalida {
			if order[0].DepartAt == nil || !order[0].DepartAt.Equal(listDay(1)) || order[3].DepartAt != nil || order[4].DepartAt != nil {
				t.Fatalf("orden por salida: %v", departs(order))
			}
		}
	}
}

func departs(rows []store.TripRow) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		if row.DepartAt == nil {
			out[i] = "—"
		} else {
			out[i] = row.DepartAt.Format("2006-01-02")
		}
	}
	return out
}

func ptrTime(t time.Time) *time.Time { return &t }

func seedNamedInstance(t *testing.T, ctx context.Context, p *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := p.QueryRow(ctx,
		`INSERT INTO instances (evolution_name, owner_label, status) VALUES ($1, 'Juanse', 'open') RETURNING id`,
		name).Scan(&id); err != nil {
		t.Fatalf("sembrando línea: %v", err)
	}
	return id
}

func seedConversationAt(t *testing.T, ctx context.Context, p *pgxpool.Pool, instanceID, contactID string, at time.Time) string {
	t.Helper()
	var id string
	if err := p.QueryRow(ctx,
		`INSERT INTO conversations (instance_id, contact_id, remote_jid, last_message_at, last_message_preview, last_message_type)
		 VALUES ($1, $2, 'jid', $3, 'hola', 'text') RETURNING id`, instanceID, contactID, at).Scan(&id); err != nil {
		t.Fatalf("sembrando conversación: %v", err)
	}
	return id
}
