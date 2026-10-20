package requisitos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/fuentes"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/store"
)

// Cuánto espera un cambio de Wikipedia antes de usarse, y cada cuánto se
// vuelve a bajar la matriz de Passport Index.
const (
	stabilityWindow  = 72 * time.Hour
	passportIndexTTL = 7 * 24 * time.Hour
)

type wikiSource interface {
	Revision(ctx context.Context, title string, at time.Time) (fuentes.Revision, error)
}

type extractor interface {
	Extract(ctx context.Context, pair domain.Pair, countryName, text string) (domain.Extraction, fuentes.Usage, error)
}

type passportIndexSource interface {
	Download(ctx context.Context) (fuentes.PassportIndexSnapshot, error)
}

type ruleStore interface {
	StoredRule(ctx context.Context, pair domain.Pair) (store.StoredRule, error)
	SaveRule(ctx context.Context, s store.Save) error
	PassportIndex(ctx context.Context) (*store.PassportIndexCopy, error)
	SavePassportIndex(ctx context.Context, c store.PassportIndexCopy) error
}

// Refresher arma o actualiza una regla a partir de las fuentes.
//
// Claude es caro y lento comparado con todo lo demás, así que solo se lo llama
// cuando cambió el texto de Wikipedia del que sale la regla, y ese cambio
// sobrevivió 72 horas. Todo lo demás —volver a combinar con Passport Index,
// las constantes— se recalcula gratis.
type Refresher struct {
	Wiki wikiSource
	// Claude es nil sin clave: las reglas nuevas quedan sin fuentes.
	Claude extractor
	// PI es nil con Passport Index apagado: Wikipedia sola alcanza.
	PI       passportIndexSource
	Store    ruleStore
	Now      func() time.Time
	Log      *slog.Logger
	OnChange func(ctx context.Context, pairs []domain.Pair)

	piMu   sync.Mutex
	piCopy *store.PassportIndexCopy
}

func (r *Refresher) Refresh(ctx context.Context, pair domain.Pair) error {
	now := r.Now()
	existing, err := r.Store.StoredRule(ctx, pair)
	if errors.Is(err, store.ErrNoRule) {
		existing = store.StoredRule{Rule: domain.Rule{Pair: pair, Status: domain.RuleBuscando}}
	} else if err != nil {
		return err
	}

	next := existing
	next.CheckedAt = &now
	var usage *store.Usage
	wikiFields := existing.WikiFields

	text, rev, stable, ok := r.wikiText(ctx, pair, now)
	switch {
	case !ok:
		// Wikipedia no respondió o no tiene el país: sigue lo que había.
	case !stable:
		// La fila cambió hace menos de 72 horas: sigue el texto aceptado. Una
		// regla nueva no usa ninguna de las dos versiones —la de hace 72 horas
		// puede ser justo la vandalizada que después se revirtió— y espera en
		// verificar a que la fila se estabilice.
		if next.PendingSince == nil {
			next.PendingSince = &now
		}
	case hash(text) == existing.WikiHash && existing.WikiFields != nil:
		next.PendingSince = nil
	case r.Claude == nil:
		// Sin clave no hay extracción.
	default:
		ex, u, err := r.Claude.Extract(ctx, pair, countryName(pair.Country), text)
		usage = &store.Usage{Model: u.Model, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens}
		if err != nil {
			r.Log.Warn("no se pudo extraer la regla", "nationality", pair.Nationality, "country", pair.Country, "error", err)
			break
		}
		fields := domain.FromWikipedia(ex, text, domain.Citation{URL: rev.URL, Revision: rev.ID})
		wikiFields = &fields
		next.WikiHash, next.WikiRevision, next.WikiURL = hash(text), rev.ID, rev.URL
		next.PendingSince = nil
	}

	if wikiFields == nil {
		next.Status, next.Fields = domain.RuleSinFuentes, domain.Fields{}
		return r.save(ctx, existing, next, nil, usage, now)
	}

	fields, conflicts := domain.Combine(pair, *wikiFields, r.passportIndex(ctx, pair, now))
	next.WikiFields = wikiFields
	next.Status, next.Fields = domain.RuleVigente, fields
	return r.save(ctx, existing, next, conflicts, usage, now)
}

// save guarda la regla. Solo una regla que cambió sube de versión, registra sus
// conflictos y avisa: un refresco que no cambia nada no re-evalúa ningún viaje.
func (r *Refresher) save(ctx context.Context, before, next store.StoredRule,
	conflicts []domain.Conflict, usage *store.Usage, now time.Time) error {
	changed := before.Status != next.Status || !sameJSON(before.Fields, next.Fields)
	if changed {
		next.Version++
		next.UpdatedAt = now
	} else {
		conflicts = nil
	}
	if err := r.Store.SaveRule(ctx, store.Save{Rule: next, Conflicts: conflicts, Usage: usage, At: now}); err != nil {
		return fmt.Errorf("guardando la regla: %w", err)
	}
	if changed && r.OnChange != nil {
		r.OnChange(ctx, []domain.Pair{next.Pair})
	}
	return nil
}

// wikiText devuelve el recorte de Wikipedia que se puede usar. Es estable si
// la fila es idéntica hoy y hace 72 horas; si no lo es, no devuelve texto y
// avisa que la fila está en movimiento.
func (r *Refresher) wikiText(ctx context.Context, pair domain.Pair, now time.Time) (string, fuentes.Revision, bool, bool) {
	nationality, ok := domain.CountryByCode(pair.Nationality)
	if !ok || nationality.VisaPage == "" {
		return "", fuentes.Revision{}, false, false
	}
	country, ok := domain.CountryByCode(pair.Country)
	if !ok {
		return "", fuentes.Revision{}, false, false
	}

	cur, err := r.Wiki.Revision(ctx, nationality.VisaPage, time.Time{})
	if err != nil {
		r.Log.Warn("no se pudo leer Wikipedia", "page", nationality.VisaPage, "error", err)
		return "", fuentes.Revision{}, false, false
	}
	curText, curOK := fuentes.Snippet(cur.Content, country)

	old, err := r.Wiki.Revision(ctx, nationality.VisaPage, now.Add(-stabilityWindow))
	var oldText string
	var oldOK bool
	if err == nil {
		oldText, oldOK = fuentes.Snippet(old.Content, country)
	}

	switch {
	case curOK && oldOK && curText == oldText:
		return curText, cur, true, true
	case curOK || oldOK:
		return "", fuentes.Revision{}, false, true
	default:
		return "", fuentes.Revision{}, false, false
	}
}

// passportIndex devuelve la fila de la matriz para la combinación. La matriz se
// baja una vez por semana y se guarda; si la descarga falla, se usa la última
// copia.
func (r *Refresher) passportIndex(ctx context.Context, pair domain.Pair, now time.Time) *domain.PassportIndexEntry {
	if r.PI == nil {
		return nil
	}
	r.piMu.Lock()
	defer r.piMu.Unlock()

	if r.piCopy == nil {
		saved, err := r.Store.PassportIndex(ctx)
		if err != nil {
			r.Log.Warn("no se pudo leer la copia de Passport Index", "error", err)
		}
		r.piCopy = saved
	}
	if r.piCopy == nil || now.Sub(r.piCopy.FetchedAt) > passportIndexTTL {
		snap, err := r.PI.Download(ctx)
		if err != nil {
			r.Log.Warn("no se pudo bajar Passport Index, se usa la última copia", "error", err)
		} else {
			fresh := store.PassportIndexCopy{Entries: snap.Entries, DataDate: snap.DataDate, FetchedAt: now}
			if err := r.Store.SavePassportIndex(ctx, fresh); err != nil {
				r.Log.Warn("no se pudo guardar Passport Index", "error", err)
			}
			r.piCopy = &fresh
		}
	}
	if r.piCopy == nil {
		return nil
	}
	requirement, ok := r.piCopy.Entries[[2]string{pair.Nationality, pair.Country}]
	if !ok {
		return nil
	}
	return &domain.PassportIndexEntry{Requirement: requirement, DataDate: r.piCopy.DataDate}
}

func countryName(code string) string {
	if c, ok := domain.CountryByCode(code); ok {
		return c.Name
	}
	return code
}

func hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func sameJSON(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}
