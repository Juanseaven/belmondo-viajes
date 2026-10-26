package mirror

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

func TestCreateLineWaitsForAsyncQRCodeWithoutCachingAnEmptyValue(t *testing.T) {
	t.Parallel()

	lines := &lineStoreFake{id: "249aad2e-68f9-464f-bc84-aca560c38f0e"}
	provider := &provisionerFake{}
	qr := &qrCacheFake{}
	module := &Module{
		lines: lines, provisioner: provider, qr: qr,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	id, name, err := module.createLine(context.Background(), "Juan", "Ventas")
	if err != nil {
		t.Fatalf("createLine: %v", err)
	}
	if id != lines.id || provider.instanceID != lines.id || provider.name != name {
		t.Fatalf("id/name flow: returned=%q/%q provider=%q/%q", id, name, provider.instanceID, provider.name)
	}
	if qr.setCalls != 0 {
		t.Fatalf("empty QR was cached %d times", qr.setCalls)
	}
	if lines.state != "connecting" {
		t.Fatalf("line state = %q, want connecting", lines.state)
	}
}

// Cerrar el modal mientras se espera el QR y volver a abrirlo no puede dejar una
// línea fantasma por intento: la que nunca llegó a vincularse se retoma.
func TestCreateLineResumesAPendingLineWithTheSameName(t *testing.T) {
	t.Parallel()

	lines := &lineStoreFake{
		id:      "249aad2e-68f9-464f-bc84-aca560c38f0e",
		pending: &store.PendingLine{ID: "6e4c0a43-55d1-4c1e-9d7c-2a6b0f5e1a11", Name: "ventas-25d818"},
	}
	provider := &provisionerFake{}
	module := &Module{
		lines: lines, provisioner: provider, qr: &qrCacheFake{},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	id, name, err := module.createLine(context.Background(), "Ventas", "")
	if err != nil {
		t.Fatalf("createLine: %v", err)
	}
	if id != lines.pending.ID || name != "ventas-25d818" {
		t.Fatalf("createLine = %q/%q, want the pending line", id, name)
	}
	if lines.created || provider.name != "" {
		t.Fatalf("a second line was created: store=%v evolution=%q", lines.created, provider.name)
	}
	if provider.reconnected != "ventas-25d818" || lines.pendingLabel != "Ventas" {
		t.Fatalf("pending line not reconnected: reconnected=%q label=%q", provider.reconnected, lines.pendingLabel)
	}
}

// Si alguien vuelve a vincular el mismo teléfono, el producto lo considera una
// línea nueva. Esta prueba evita que "Desvincular" vuelva a convertirse en una
// baja lógica que deja atrás conversaciones, mensajes o adjuntos.
func TestDisconnectLinePurgesTheLineAndItsStoredFiles(t *testing.T) {
	t.Parallel()

	const (
		id   = "249aad2e-68f9-464f-bc84-aca560c38f0e"
		name = "juanse-c21c00"
	)
	lines := &lineStoreFake{id: id, name: name}
	provider := &provisionerFake{}
	qr := &qrCacheFake{}
	files := &blobStoreFake{}
	module := &Module{
		k:     &kernel.Kernel{Blobs: files},
		lines: lines, provisioner: provider, qr: qr,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	if err := module.disconnectLine(context.Background(), id); err != nil {
		t.Fatalf("disconnectLine: %v", err)
	}
	if lines.deletedID != id {
		t.Fatalf("deleted id = %q, want %q", lines.deletedID, id)
	}
	if lines.deactivatedID != "" {
		t.Fatalf("line was only deactivated: %q", lines.deactivatedID)
	}
	wantPrefixes := []string{name + "/", "outgoing/" + name + "/"}
	if len(files.deletedPrefixes) != len(wantPrefixes) {
		t.Fatalf("deleted prefixes = %v, want %v", files.deletedPrefixes, wantPrefixes)
	}
	for idx, want := range wantPrefixes {
		if files.deletedPrefixes[idx] != want {
			t.Fatalf("deleted prefix %d = %q, want %q", idx, files.deletedPrefixes[idx], want)
		}
	}
	if provider.logoutName != name || provider.deleteName != name || qr.cleared != name {
		t.Fatalf("provider/qr cleanup = logout %q, delete %q, qr %q", provider.logoutName, provider.deleteName, qr.cleared)
	}
}

type provisionerFake struct {
	instanceID string
	name       string
	qr         string
	logoutName  string
	deleteName  string
	reconnected string
}

func (p *provisionerFake) CreateInstance(_ context.Context, instanceID, name string) (string, error) {
	p.instanceID, p.name = instanceID, name
	return p.qr, nil
}
func (p *provisionerFake) ConfigureWebhook(_ context.Context, name string) error {
	p.reconnected = name
	return nil
}
func (p *provisionerFake) Logout(_ context.Context, name string) error {
	p.logoutName = name
	return nil
}
func (p *provisionerFake) DeleteInstance(_ context.Context, name string) error {
	p.deleteName = name
	return nil
}

type qrCacheFake struct {
	instance, value, cleared string
	setCalls                 int
}

func (q *qrCacheFake) Set(_ context.Context, instance, value string) error {
	q.setCalls++
	q.instance, q.value = instance, value
	return nil
}
func (*qrCacheFake) Get(context.Context, string) (string, bool) { return "", false }
func (q *qrCacheFake) Clear(_ context.Context, instance string) { q.cleared = instance }

type blobStoreFake struct{ deletedPrefixes []string }

func (*blobStoreFake) Put(context.Context, string, string, []byte) error { return nil }
func (*blobStoreFake) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (b *blobStoreFake) DeletePrefix(_ context.Context, prefix string) error {
	b.deletedPrefixes = append(b.deletedPrefixes, prefix)
	return nil
}

type lineStoreFake struct {
	id            string
	name          string
	state         string
	states        []string
	deletedID     string
	deactivatedID string
	pending       *store.PendingLine
	pendingLabel  string
	created       bool
}

func (l *lineStoreFake) PendingByLabel(_ context.Context, label string) (store.PendingLine, bool, error) {
	l.pendingLabel = label
	if l.pending == nil {
		return store.PendingLine{}, false, nil
	}
	return *l.pending, true, nil
}

func (l *lineStoreFake) IDByName(context.Context, string) (string, error) { return l.id, nil }
func (l *lineStoreFake) UpdateConnection(_ context.Context, name, state, _ string) error {
	l.name, l.state = name, state
	l.states = append(l.states, state)
	return nil
}
func (*lineStoreFake) Touch(context.Context, string, time.Time) error { return nil }
func (l *lineStoreFake) Create(_ context.Context, name, _, _ string) (string, error) {
	l.name = name
	l.created = true
	return l.id, nil
}
func (l *lineStoreFake) NameByID(context.Context, string) (string, error) { return l.name, nil }
func (l *lineStoreFake) Deactivate(_ context.Context, id string) error {
	l.deactivatedID = id
	return nil
}
func (l *lineStoreFake) Delete(_ context.Context, id string) error {
	l.deletedID = id
	return nil
}

var _ store.InstanceStore = (*lineStoreFake)(nil)
