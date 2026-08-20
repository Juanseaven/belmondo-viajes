package mirror

import (
	"context"
	"fmt"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

const lineRecoveryInterval = 30 * time.Second

type lineDirectory interface {
	ConfigureWebhook(ctx context.Context, instance string) error
	ConnectionState(ctx context.Context, instance string) (string, error)
	Contacts(ctx context.Context, instance string) ([]domain.ContactIdentity, error)
}

func shouldRecoverLine(line store.Line) bool {
	return line.Status == "open" || line.WasConnected
}

// reconcileLine compares the local projection with Evolution's live state.
// Reconnecting always reuses the same instance, so conversations remain linked
// to the same line whether credentials are restored or a fresh QR is required.
func (m *Module) reconcileLine(ctx context.Context, line store.Line, forceConfigure bool) error {
	state, stateErr := m.directory.ConnectionState(ctx, line.Name)
	if state == "open" {
		if forceConfigure {
			if err := m.directory.ConfigureWebhook(ctx, line.Name); err != nil {
				return fmt.Errorf("actualizando la sesión conectada en Evolution: %w", err)
			}
		}
		if line.Status == "open" && !forceConfigure {
			return nil
		}
		return m.markLineOpen(ctx, line)
	}
	if state == "connecting" && !forceConfigure {
		if line.Status != "connecting" {
			return m.lines.UpdateConnection(ctx, line.Name, "connecting", "")
		}
		return nil
	}

	if line.Status != "connecting" {
		if err := m.lines.UpdateConnection(ctx, line.Name, "connecting", ""); err != nil {
			return fmt.Errorf("marcando la reconexión: %w", err)
		}
	}
	if err := m.directory.ConfigureWebhook(ctx, line.Name); err != nil {
		_ = m.lines.UpdateConnection(ctx, line.Name, "close", "")
		return fmt.Errorf("reactivando la sesión en Evolution: %w", err)
	}

	state, err := m.directory.ConnectionState(ctx, line.Name)
	if err != nil {
		if stateErr != nil {
			return fmt.Errorf("consultando el estado de la sesión: %w", err)
		}
		return nil
	}
	if state == "open" {
		return m.markLineOpen(ctx, line)
	}
	return nil
}

func (m *Module) markLineOpen(ctx context.Context, line store.Line) error {
	if err := m.lines.UpdateConnection(ctx, line.Name, "open", ""); err != nil {
		return fmt.Errorf("guardando la conexión recuperada: %w", err)
	}
	contacts, err := m.directory.Contacts(ctx, line.Name)
	if err != nil {
		return fmt.Errorf("sincronizando la agenda recuperada: %w", err)
	}
	if err := m.contacts.UpsertDirectory(ctx, line.Name, contacts); err != nil {
		return fmt.Errorf("guardando la agenda recuperada: %w", err)
	}
	return nil
}

func (m *Module) runLineRecovery(ctx context.Context) {
	defer close(m.recoveryDone)

	ticker := time.NewTicker(lineRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lines, err := m.read.Lines(ctx)
			if err != nil {
				m.log.Warn("no se pudieron listar líneas para recuperar", "error", err)
				continue
			}
			for _, line := range lines {
				if line.Status == "open" || !shouldRecoverLine(line) {
					continue
				}
				if err := m.reconcileLine(ctx, line, false); err != nil && ctx.Err() == nil {
					m.log.Warn("no se pudo reconciliar la línea", "instance", line.Name, "error", err)
				}
			}
		}
	}
}
