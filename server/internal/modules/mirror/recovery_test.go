package mirror

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

func TestReconcileLineReconnectsTheExistingClosedSession(t *testing.T) {
	t.Parallel()

	lines := &lineStoreFake{}
	directory := &lineConnectionFake{states: []string{"close", "open"}}
	contacts := &contactStoreFake{}
	module := &Module{
		lines: lines, directory: directory, contacts: contacts,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	line := store.Line{ID: "existing-id", Name: "juanse-existing", Status: "close", IsActive: true}

	if err := module.reconcileLine(context.Background(), line, false); err != nil {
		t.Fatalf("reconcileLine: %v", err)
	}
	if directory.configured != line.Name {
		t.Fatalf("configured instance = %q, want %q", directory.configured, line.Name)
	}
	if lines.state != "open" {
		t.Fatalf("stored state = %q, want open", lines.state)
	}
	if contacts.instance != line.Name {
		t.Fatalf("directory stored for = %q, want %q", contacts.instance, line.Name)
	}
}

func TestReconcileLineDoesNotMarkAnAlreadyOpenLineAsConnecting(t *testing.T) {
	t.Parallel()

	lines := &lineStoreFake{}
	directory := &lineConnectionFake{states: []string{"open", "open"}}
	module := &Module{
		lines: lines, directory: directory, contacts: &contactStoreFake{},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	line := store.Line{ID: "existing-id", Name: "juanse-existing", Status: "open", IsActive: true}

	if err := module.reconcileLine(context.Background(), line, true); err != nil {
		t.Fatalf("reconcileLine: %v", err)
	}
	if len(lines.states) != 1 || lines.states[0] != "open" {
		t.Fatalf("stored state transitions = %v, want [open]", lines.states)
	}
}

func TestShouldRecoverLineSkipsClosedInstancesThatWereNeverPaired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line store.Line
		want bool
	}{
		{name: "open line refreshes webhook", line: store.Line{Status: "open"}, want: true},
		{name: "previous session reconnects", line: store.Line{Status: "close", WasConnected: true}, want: true},
		{name: "abandoned first pairing stays idle", line: store.Line{Status: "close", WasConnected: false}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRecoverLine(tt.line); got != tt.want {
				t.Fatalf("shouldRecoverLine(%+v) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

type lineConnectionFake struct {
	states     []string
	stateCalls int
	configured string
}

func (f *lineConnectionFake) ConfigureWebhook(_ context.Context, instance string) error {
	f.configured = instance
	return nil
}

func (f *lineConnectionFake) ConnectionState(context.Context, string) (string, error) {
	idx := f.stateCalls
	if idx >= len(f.states) {
		idx = len(f.states) - 1
	}
	f.stateCalls++
	return f.states[idx], nil
}

func (*lineConnectionFake) Contacts(context.Context, string) ([]domain.ContactIdentity, error) {
	return []domain.ContactIdentity{{PhoneJID: "5491112345678@s.whatsapp.net", Name: "Contacto"}}, nil
}

type contactStoreFake struct{ instance string }

func (f *contactStoreFake) UpsertDirectory(_ context.Context, instance string, _ []domain.ContactIdentity) error {
	f.instance = instance
	return nil
}

func (*contactStoreFake) SetGroupName(context.Context, string, string, string) error { return nil }
