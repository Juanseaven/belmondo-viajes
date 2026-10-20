// Package requisitos arma las reglas de entrada de cada país para cada
// nacionalidad, solas, a partir de Wikipedia, Passport Index y unas pocas
// constantes.
//
// Nadie carga ni aprueba reglas. El módulo operaciones las pide para evaluar
// sus viajes; si una no existe, nace en «buscando» y se busca enseguida. Cada
// semana se revisan todas, y solo se le paga a Claude cuando cambió el texto
// de Wikipedia del que sale una regla.
//
// No conoce a operaciones: avisa los cambios por un gancho que conecta
// cmd/api/main.go, y así el grafo de imports sigue siendo acíclico.
package requisitos

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/modules"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/fuentes"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/store/postgres"
)

const Name = domain.ModuleName

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Cada cuánto se mira si hay reglas para revisar, y cuántas por vuelta. Una
// regla nueva no espera la vuelta: despierta la búsqueda.
const (
	tick     = time.Minute
	perRound = 20
)

type queue interface {
	Due(ctx context.Context, now time.Time, limit int) ([]domain.Pair, error)
	Ensure(ctx context.Context, pairs []domain.Pair) (map[domain.Pair]domain.Rule, []domain.Pair, error)
}

type refresher interface {
	Refresh(ctx context.Context, pair domain.Pair) error
}

type Module struct {
	queue     queue
	refresher refresher
	log       *slog.Logger
	now       func() time.Time
	wake      chan struct{}

	mu       sync.Mutex
	onChange []func(ctx context.Context, pairs []domain.Pair)

	cancel context.CancelFunc
	done   chan struct{}
}

func New(k *kernel.Kernel) *Module {
	st := postgres.New(k.DB)
	log := k.Log.With("module", Name)
	m := &Module{queue: st, log: log, now: time.Now, wake: make(chan struct{}, 1)}

	r := &Refresher{Wiki: fuentes.NewWikipedia(k.Cfg.RequisitosContact), Store: st, Now: time.Now, Log: log,
		OnChange: m.notify}
	// Una interfaz con un *Claude nil adentro no es nil: se asigna solo si hay.
	if c := fuentes.NewClaude(k.Cfg.AnthropicAPIKey, ""); c != nil {
		r.Claude = c
	} else {
		log.Warn("sin ANTHROPIC_API_KEY: las reglas nuevas quedan sin fuentes y todo cuenta como «verificar»")
	}
	if k.Cfg.PassportIndexEnabled {
		r.PI = fuentes.NewPassportIndex()
	}
	if k.Cfg.RequisitosContact == "" {
		log.Warn("sin REQUISITOS_CONTACTO: Wikipedia pide un contacto en el User-Agent")
	}
	m.refresher = r
	return m
}

var _ modules.Module = (*Module)(nil)

func (m *Module) Name() string { return Name }

func (m *Module) Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		panic(fmt.Sprintf("requisitos: migraciones embebidas ilegibles: %v", err))
	}
	return sub
}

func (m *Module) Mount(r modules.Router) {
	r.Authed("GET /paises", http.HandlerFunc(m.countries))
}

// countries es el catálogo con nombres en español, para elegir países por
// nombre y no por código.
func (m *Module) countries(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"items": domain.Countries()})
}

// Rules devuelve las reglas de las combinaciones pedidas. Las que no existen
// nacen en «buscando» y despiertan la búsqueda.
func (m *Module) Rules(ctx context.Context, pairs []domain.Pair) (map[domain.Pair]domain.Rule, error) {
	rules, created, err := m.queue.Ensure(ctx, pairs)
	if err != nil {
		return nil, err
	}
	if len(created) > 0 {
		select {
		case m.wake <- struct{}{}:
		default:
		}
	}
	return rules, nil
}

// OnChange registra a quien quiere enterarse de que cambió una regla.
func (m *Module) OnChange(fn func(ctx context.Context, pairs []domain.Pair)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = append(m.onChange, fn)
}

func (m *Module) notify(ctx context.Context, pairs []domain.Pair) {
	m.mu.Lock()
	fns := append([]func(context.Context, []domain.Pair){}, m.onChange...)
	m.mu.Unlock()
	for _, fn := range fns {
		fn(ctx, pairs)
	}
}

func (m *Module) Start(ctx context.Context) error {
	ctx, m.cancel = context.WithCancel(ctx)
	m.done = make(chan struct{})
	go m.loop(ctx)
	return nil
}

func (m *Module) Shutdown(ctx context.Context) error {
	if m.cancel == nil {
		return nil
	}
	m.cancel()
	select {
	case <-m.done:
	case <-ctx.Done():
	}
	return nil
}

func (m *Module) loop(ctx context.Context) {
	defer close(m.done)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	m.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.wake:
		}
		m.runOnce(ctx)
	}
}

// runOnce revisa las reglas que tocan. Un error en una no frena a las demás:
// cada regla se reintenta en su próxima vuelta.
func (m *Module) runOnce(ctx context.Context) {
	due, err := m.queue.Due(ctx, m.now(), perRound)
	if err != nil {
		m.log.Error("no se pudieron listar las reglas a revisar", "error", err)
		return
	}
	for _, pair := range due {
		if ctx.Err() != nil {
			return
		}
		if err := m.refresher.Refresh(ctx, pair); err != nil {
			m.log.Error("no se pudo revisar la regla", "nationality", pair.Nationality, "country", pair.Country, "error", err)
		}
	}
}
