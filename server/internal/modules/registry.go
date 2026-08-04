package modules

import (
	"context"
	"log/slog"
)

// Registry es la lista de módulos activos y su ciclo de vida.
//
// Existe para que el orden de arranque y apagado sea código testeable y no una
// propiedad accidental de cómo está escrito main().
type Registry struct {
	mods    []Module
	started []Module
	log     *slog.Logger
}

func NewRegistry(log *slog.Logger, mods ...Module) *Registry {
	return &Registry{mods: mods, log: log}
}

func (r *Registry) All() []Module { return r.mods }

func (r *Registry) Names() []string {
	names := make([]string, len(r.mods))
	for i, m := range r.mods {
		names[i] = m.Name()
	}
	return names
}

// Start arranca en orden de declaración.
//
// Si uno falla, se apagan los que ya arrancaron antes de devolver el error:
// dejar goroutines vivas tras un arranque fallido es una fuga silenciosa que solo
// se nota cuando el proceso no termina de morir.
func (r *Registry) Start(ctx context.Context) error {
	for _, m := range r.mods {
		if err := m.Start(ctx); err != nil {
			r.Shutdown(context.WithoutCancel(ctx))
			return err
		}
		r.started = append(r.started, m)
	}
	return nil
}

// Shutdown drena en orden INVERSO al arranque, para que un módulo que produce
// trabajo para otro deje de producirlo primero.
//
// Solo toca los que llegaron a arrancar, y no corta ante el primer error: un
// módulo que no drena a tiempo no puede impedir que los demás lo intenten.
func (r *Registry) Shutdown(ctx context.Context) {
	for i := len(r.started) - 1; i >= 0; i-- {
		m := r.started[i]
		if err := m.Shutdown(ctx); err != nil {
			r.log.Error("módulo no drenado a tiempo", "module", m.Name(), "error", err)
		}
	}
	r.started = nil
}
