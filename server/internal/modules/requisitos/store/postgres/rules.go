// Package postgres implementa la persistencia del módulo de requisitos.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/store"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func split(pairs []domain.Pair) ([]string, []string) {
	nats := make([]string, len(pairs))
	countries := make([]string, len(pairs))
	for i, p := range pairs {
		nats[i], countries[i] = p.Nationality, p.Country
	}
	return nats, countries
}

// Ensure devuelve las reglas pedidas, creando en «buscando» las que faltan.
// También devuelve cuáles creó, para despertar a quien las busca.
func (s *Store) Ensure(ctx context.Context, pairs []domain.Pair) (map[domain.Pair]domain.Rule, []domain.Pair, error) {
	if len(pairs) == 0 {
		return map[domain.Pair]domain.Rule{}, nil, nil
	}
	nats, countries := split(pairs)
	rows, err := s.pool.Query(ctx,
		`INSERT INTO entry_rules (nationality, country)
		 SELECT * FROM unnest($1::text[], $2::text[])
		 ON CONFLICT DO NOTHING
		 RETURNING nationality, country`, nats, countries)
	if err != nil {
		return nil, nil, fmt.Errorf("creando reglas: %w", err)
	}
	created, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Pair, error) {
		var p domain.Pair
		err := row.Scan(&p.Nationality, &p.Country)
		return p, err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo reglas creadas: %w", err)
	}
	rules, err := s.Rules(ctx, pairs)
	return rules, created, err
}

// Rules lee las reglas que existen de las combinaciones pedidas.
func (s *Store) Rules(ctx context.Context, pairs []domain.Pair) (map[domain.Pair]domain.Rule, error) {
	nats, countries := split(pairs)
	rows, err := s.pool.Query(ctx,
		`SELECT r.nationality, r.country, r.status, r.fields, r.version, r.updated_at, r.details
		   FROM entry_rules r
		   JOIN unnest($1::text[], $2::text[]) AS p(nationality, country)
		     ON p.nationality = r.nationality AND p.country = r.country`, nats, countries)
	if err != nil {
		return nil, fmt.Errorf("leyendo reglas: %w", err)
	}
	defer rows.Close()
	out := make(map[domain.Pair]domain.Rule, len(pairs))
	for rows.Next() {
		var r domain.Rule
		var fields, details []byte
		if err := rows.Scan(&r.Nationality, &r.Country, &r.Status, &fields, &r.Version, &r.UpdatedAt, &details); err != nil {
			return nil, fmt.Errorf("armando reglas: %w", err)
		}
		if err := json.Unmarshal(fields, &r.Fields); err != nil {
			return nil, fmt.Errorf("leyendo los campos de %s-%s: %w", r.Nationality, r.Country, err)
		}
		if err := json.Unmarshal(details, &r.Details); err != nil {
			return nil, fmt.Errorf("leyendo el detalle de %s-%s: %w", r.Nationality, r.Country, err)
		}
		out[r.Pair] = r
	}
	return out, rows.Err()
}

func (s *Store) StoredRule(ctx context.Context, p domain.Pair) (store.StoredRule, error) {
	var r store.StoredRule
	var fields, wikiFields, details, research []byte
	var hash, revision, url *string
	err := s.pool.QueryRow(ctx,
		`SELECT nationality, country, status, fields, version, updated_at,
		        wiki_fields, wiki_hash, wiki_revision, wiki_url, pending_since, checked_at,
		        details, research, research_due_at, research_failures
		   FROM entry_rules WHERE nationality = $1 AND country = $2`, p.Nationality, p.Country,
	).Scan(&r.Nationality, &r.Country, &r.Status, &fields, &r.Version, &r.UpdatedAt,
		&wikiFields, &hash, &revision, &url, &r.PendingSince, &r.CheckedAt,
		&details, &research, &r.ResearchDueAt, &r.ResearchFailures)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.StoredRule{}, store.ErrNoRule
	}
	if err != nil {
		return store.StoredRule{}, fmt.Errorf("leyendo la regla: %w", err)
	}
	if err := json.Unmarshal(fields, &r.Fields); err != nil {
		return store.StoredRule{}, fmt.Errorf("leyendo los campos: %w", err)
	}
	if wikiFields != nil {
		var w domain.Fields
		if err := json.Unmarshal(wikiFields, &w); err != nil {
			return store.StoredRule{}, fmt.Errorf("leyendo los campos de Wikipedia: %w", err)
		}
		r.WikiFields = &w
	}
	if err := json.Unmarshal(details, &r.Details); err != nil {
		return store.StoredRule{}, fmt.Errorf("leyendo el detalle: %w", err)
	}
	if research != nil {
		var res domain.Research
		if err := json.Unmarshal(research, &res); err != nil {
			return store.StoredRule{}, fmt.Errorf("leyendo la búsqueda: %w", err)
		}
		r.Research = &res
	}
	r.WikiHash, r.WikiRevision, r.WikiURL = deref(hash), deref(revision), deref(url)
	return r, nil
}

// SaveRule guarda la regla y, en la misma transacción, su versión (si es
// nueva), los conflictos y el consumo de la extracción.
func (s *Store) SaveRule(ctx context.Context, save store.Save) error {
	r := save.Rule
	fields, err := json.Marshal(r.Fields)
	if err != nil {
		return err
	}
	var wikiFields, research []byte
	if r.WikiFields != nil {
		if wikiFields, err = json.Marshal(r.WikiFields); err != nil {
			return err
		}
	}
	if r.Research != nil {
		if research, err = json.Marshal(r.Research); err != nil {
			return err
		}
	}
	details, err := json.Marshal(r.Details)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updatedAt := r.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = save.At
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO entry_rules (nationality, country, status, fields, version, wiki_fields, wiki_hash,
		                          wiki_revision, wiki_url, pending_since, checked_at, updated_at,
		                          details, research, research_due_at, research_failures)
		 VALUES ($1, $2, $3, $4, $5, $6, nullif($7, ''), nullif($8, ''), nullif($9, ''), $10, $11, $12,
		         $13, $14, $15, $16)
		 ON CONFLICT (nationality, country) DO UPDATE SET
		   status = EXCLUDED.status, fields = EXCLUDED.fields, version = EXCLUDED.version,
		   wiki_fields = EXCLUDED.wiki_fields, wiki_hash = EXCLUDED.wiki_hash,
		   wiki_revision = EXCLUDED.wiki_revision, wiki_url = EXCLUDED.wiki_url,
		   pending_since = EXCLUDED.pending_since, checked_at = EXCLUDED.checked_at,
		   updated_at = EXCLUDED.updated_at, details = EXCLUDED.details, research = EXCLUDED.research,
		   research_due_at = EXCLUDED.research_due_at, research_failures = EXCLUDED.research_failures`,
		r.Nationality, r.Country, r.Status, fields, r.Version, wikiFields, r.WikiHash,
		r.WikiRevision, r.WikiURL, r.PendingSince, r.CheckedAt, updatedAt,
		details, research, r.ResearchDueAt, r.ResearchFailures); err != nil {
		return fmt.Errorf("guardando la regla: %w", err)
	}
	if r.Version > 0 {
		if _, err := tx.Exec(ctx,
			`INSERT INTO entry_rule_versions (nationality, country, version, status, fields, details, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
			r.Nationality, r.Country, r.Version, r.Status, fields, details, save.At); err != nil {
			return fmt.Errorf("guardando la versión: %w", err)
		}
	}
	for _, c := range save.Conflicts {
		if _, err := tx.Exec(ctx,
			`INSERT INTO source_conflicts (nationality, country, field, wikipedia, other, source, data_date, created_at)
			 VALUES ($1, $2, $3, $4, $5, coalesce(nullif($6, ''), 'passport_index'), $7, $8)`,
			c.Nationality, c.Country, c.Field, c.Wikipedia, c.Other, c.Source, c.DataDate, save.At); err != nil {
			return fmt.Errorf("guardando el conflicto: %w", err)
		}
	}
	if u := save.Usage; u != nil && u.Model != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO extraction_usage (nationality, country, model, input_tokens, output_tokens, web_searches,
			                               purpose, cache_read_tokens, cache_write_tokens, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, coalesce(nullif($7, ''), 'wikipedia'), $8, $9, $10)`,
			r.Nationality, r.Country, u.Model, u.InputTokens, u.OutputTokens, u.WebSearches, u.Purpose,
			u.CacheReadTokens, u.CacheWriteTokens, save.At); err != nil {
			return fmt.Errorf("guardando el consumo: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// Due devuelve las reglas que toca revisar: las que se están buscando primero;
// las vigentes, una vez por semana; las que esperan las 72 horas o no tuvieron
// fuentes, una vez por día; y las que tienen pendiente la búsqueda en páginas
// oficiales.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]domain.Pair, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT nationality, country FROM entry_rules
		  WHERE status = 'buscando'
		     OR checked_at IS NULL
		     OR checked_at < $1::timestamptz - interval '7 days'
		     OR ((pending_since IS NOT NULL OR status = 'sin_fuentes')
		         AND checked_at < $1::timestamptz - interval '1 day')
		     OR research_due_at <= $1::timestamptz
		  ORDER BY (status = 'buscando') DESC, checked_at NULLS FIRST
		  LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("buscando reglas a revisar: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Pair, error) {
		var p domain.Pair
		err := row.Scan(&p.Nationality, &p.Country)
		return p, err
	})
}

func (s *Store) PassportIndex(ctx context.Context) (*store.PassportIndexCopy, error) {
	var raw []byte
	c := store.PassportIndexCopy{}
	err := s.pool.QueryRow(ctx,
		`SELECT entries, data_date, fetched_at FROM passport_index_copy WHERE id = 1`).Scan(&raw, &c.DataDate, &c.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leyendo Passport Index: %w", err)
	}
	var flat map[string]string
	if err := json.Unmarshal(raw, &flat); err != nil {
		return nil, fmt.Errorf("leyendo Passport Index: %w", err)
	}
	c.Entries = make(map[[2]string]string, len(flat))
	for k, v := range flat {
		if from, to, ok := strings.Cut(k, "-"); ok {
			c.Entries[[2]string{from, to}] = v
		}
	}
	return &c, nil
}

func (s *Store) SavePassportIndex(ctx context.Context, c store.PassportIndexCopy) error {
	flat := make(map[string]string, len(c.Entries))
	for k, v := range c.Entries {
		flat[k[0]+"-"+k[1]] = v
	}
	raw, err := json.Marshal(flat)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO passport_index_copy (id, entries, data_date, fetched_at) VALUES (1, $1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET entries = EXCLUDED.entries, data_date = EXCLUDED.data_date,
		                                fetched_at = EXCLUDED.fetched_at`, raw, c.DataDate, c.FetchedAt)
	if err != nil {
		return fmt.Errorf("guardando Passport Index: %w", err)
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
