package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/db"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/db/dbtest"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/store"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m, "requisitos_postgres")) }

func testDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	ctx, pool := dbtest.Pool(t)
	if err := db.Migrate(ctx, pool, domain.ModuleName, os.DirFS("../../migrations"),
		slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migraciones: %v", err)
	}
	return ctx, pool
}

var (
	brasil = domain.Pair{Nationality: "AR", Country: "BR"}
	canada = domain.Pair{Nationality: "AR", Country: "CA"}
)

// Pedir reglas crea las que faltan en «buscando»: el viaje que las necesita no
// espera a nadie para saber que todavía no se sabe.
func TestPedirReglasCreaLasQueFaltanEnBuscando(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)

	rules, created, err := s.Ensure(ctx, []domain.Pair{brasil, canada})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(rules) != 2 || rules[brasil].Status != domain.RuleBuscando || len(created) != 2 {
		t.Fatalf("reglas = %+v, creadas = %v", rules, created)
	}

	_, created, err = s.Ensure(ctx, []domain.Pair{brasil})
	if err != nil || len(created) != 0 {
		t.Fatalf("la segunda vez no crea nada: %v, %v", created, err)
	}
}

func TestUnaReglaGuardadaSeLeeEntera(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	if _, _, err := s.Ensure(ctx, []domain.Pair{canada}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	visa := domain.VisaConsular
	fields := domain.Fields{Visa: domain.Field[domain.Visa]{Value: &visa,
		Sources: []domain.Citation{{Source: domain.SourceWikipedia, Quote: "Visa required"}}}}

	err := s.SaveRule(ctx, store.Save{At: now,
		Rule: store.StoredRule{Rule: domain.Rule{Pair: canada, Status: domain.RuleVigente, Fields: fields, Version: 1, UpdatedAt: now},
			WikiFields: &fields, WikiHash: "abc", WikiRevision: "200", WikiURL: "https://wiki", CheckedAt: &now},
		Conflicts: []domain.Conflict{{Pair: canada, Field: "visa", Wikipedia: "visa_consular", PassportIndex: "visa free", DataDate: "2026-02-17"}},
		Usage:     &store.Usage{Model: "claude-opus-5-5", InputTokens: 1200, OutputTokens: 300},
	})
	if err != nil {
		t.Fatalf("guardando: %v", err)
	}

	got, err := s.StoredRule(ctx, canada)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if got.Status != domain.RuleVigente || got.Version != 1 || got.Fields.Visa.Value == nil ||
		*got.Fields.Visa.Value != domain.VisaConsular || got.WikiHash != "abc" || got.WikiFields == nil ||
		got.CheckedAt == nil || !got.CheckedAt.Equal(now) {
		t.Fatalf("regla = %+v", got)
	}

	var versions, conflicts, usage int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM entry_rule_versions`).Scan(&versions)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM source_conflicts`).Scan(&conflicts)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM extraction_usage`).Scan(&usage)
	if versions != 1 || conflicts != 1 || usage != 1 {
		t.Fatalf("versiones = %d, conflictos = %d, consumo = %d", versions, conflicts, usage)
	}

	if _, err := s.StoredRule(ctx, domain.Pair{Nationality: "AR", Country: "JP"}); err != store.ErrNoRule {
		t.Fatalf("error = %v, se esperaba ErrNoRule", err)
	}
}

// Qué se revisa: lo que se está buscando ya; lo vigente, una vez por semana;
// lo que espera las 72 horas o no tuvo fuentes, una vez por día.
func TestLasReglasPendientesDeRevision(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	pairs := map[string]domain.Pair{}
	for _, c := range []string{"BR", "CA", "JP", "US", "MX", "ES"} {
		pairs[c] = domain.Pair{Nationality: "AR", Country: c}
	}
	ensure := func(p domain.Pair) {
		if _, _, err := s.Ensure(ctx, []domain.Pair{p}); err != nil {
			t.Fatal(err)
		}
	}
	save := func(p domain.Pair, status domain.RuleStatus, checked time.Time, pending *time.Time) {
		ensure(p)
		if err := s.SaveRule(ctx, store.Save{At: checked, Rule: store.StoredRule{
			Rule: domain.Rule{Pair: p, Status: status}, CheckedAt: &checked, PendingSince: pending}}); err != nil {
			t.Fatal(err)
		}
	}

	ensure(pairs["BR"])                                                  // buscando: ya
	save(pairs["CA"], domain.RuleVigente, now.Add(-8*24*time.Hour), nil) // vigente de hace 8 días: toca
	save(pairs["JP"], domain.RuleVigente, now.Add(-2*24*time.Hour), nil) // vigente de hace 2 días: no
	pending := now.Add(-30 * time.Hour)
	save(pairs["US"], domain.RuleVigente, now.Add(-25*time.Hour), &pending) // en espera, revisada ayer: toca
	save(pairs["MX"], domain.RuleSinFuentes, now.Add(-25*time.Hour), nil)   // sin fuentes, ayer: toca
	save(pairs["ES"], domain.RuleSinFuentes, now.Add(-2*time.Hour), nil)    // sin fuentes, hace 2 h: no

	due, err := s.Due(ctx, now, 10)
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	got := map[string]bool{}
	for _, p := range due {
		got[p.Country] = true
	}
	for _, c := range []string{"BR", "CA", "US", "MX"} {
		if !got[c] {
			t.Errorf("%s debería revisarse", c)
		}
	}
	for _, c := range []string{"JP", "ES"} {
		if got[c] {
			t.Errorf("%s no debería revisarse todavía", c)
		}
	}
	if len(due) == 0 || due[0] != pairs["BR"] {
		t.Errorf("lo que se está buscando va primero: %v", due)
	}
}

func TestLaCopiaDePassportIndexSeGuardaYSeLee(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)

	if got, err := s.PassportIndex(ctx); err != nil || got != nil {
		t.Fatalf("sin copia: %+v, %v", got, err)
	}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	cp := store.PassportIndexCopy{Entries: map[[2]string]string{{"AR", "BR"}: "90", {"AR", "US"}: "visa required"},
		DataDate: "2026-02-17", FetchedAt: now}
	if err := s.SavePassportIndex(ctx, cp); err != nil {
		t.Fatalf("guardando: %v", err)
	}
	cp.DataDate = "2026-03-01"
	if err := s.SavePassportIndex(ctx, cp); err != nil {
		t.Fatalf("reemplazando: %v", err)
	}
	got, err := s.PassportIndex(ctx)
	if err != nil || got == nil || got.DataDate != "2026-03-01" || got.Entries[[2]string{"AR", "US"}] != "visa required" ||
		!got.FetchedAt.Equal(now) {
		t.Fatalf("copia = %+v, %v", got, err)
	}
}

func TestLasReglasSeLeenPorCombinacion(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	if _, _, err := s.Ensure(ctx, []domain.Pair{brasil, canada}); err != nil {
		t.Fatal(err)
	}
	rules, err := s.Rules(ctx, []domain.Pair{canada, {Nationality: "AR", Country: "JP"}})
	if err != nil || len(rules) != 1 || rules[canada].Pair != canada {
		t.Fatalf("reglas = %+v, %v", rules, err)
	}
}
