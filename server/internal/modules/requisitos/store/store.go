// Package store define qué necesita el módulo de requisitos de su
// persistencia, sin decir cómo se guarda.
package store

import (
	"errors"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

var ErrNoRule = errors.New("requisitos: la regla no existe")

// StoredRule es la regla con lo que hace falta para refrescarla sin volver a
// pagar una extracción: los campos que salieron de Wikipedia antes de
// combinarlos y el hash del texto del que salieron.
type StoredRule struct {
	domain.Rule
	// WikiFields son los campos de Wikipedia solos. Con ellos se vuelve a
	// combinar cuando cambia Passport Index, sin llamar a Claude.
	WikiFields   *domain.Fields
	WikiHash     string
	WikiRevision string
	WikiURL      string
	// PendingSince marca una fila de Wikipedia que cambió hace menos de 72
	// horas: la regla sigue con el texto anterior y se vuelve a mirar cada día.
	PendingSince *time.Time
	CheckedAt    *time.Time
	// Research es la última búsqueda en páginas oficiales que pasó la
	// comprobación. Nil: nunca se encontró nada.
	Research *domain.Research
	// ResearchDueAt es cuándo toca buscar de nuevo. Nil: no se busca (no hay
	// buscador configurado).
	ResearchDueAt    *time.Time
	ResearchFailures int
}

// Usage es el consumo de una extracción o de una búsqueda.
type Usage struct {
	Model        string
	InputTokens  int64
	OutputTokens int64
	WebSearches  int64
	// Lo leído y lo escrito en la caché, que input_tokens no incluye.
	CacheReadTokens  int64
	CacheWriteTokens int64
	// Purpose es «wikipedia» (la extracción) u «oficial» (la búsqueda).
	Purpose string
}

// Save es lo que se guarda después de refrescar una regla.
type Save struct {
	Rule      StoredRule
	Conflicts []domain.Conflict
	Usage     *Usage
	At        time.Time
}

// PassportIndexCopy es la última matriz descargada.
type PassportIndexCopy struct {
	Entries   map[[2]string]string
	DataDate  string
	FetchedAt time.Time
}
