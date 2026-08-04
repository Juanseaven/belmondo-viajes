package modules

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"testing"
	"testing/fstest"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fake es un módulo de mentira que anota en un diario compartido qué le pasó y
// cuándo. Comprobar el ORDEN es todo el objetivo de estos tests.
type fake struct {
	name     string
	journal  *[]string
	startErr error
}

func (f *fake) Name() string      { return f.name }
func (f *fake) Migrations() fs.FS { return fstest.MapFS{} }
func (f *fake) Mount(r Router)    { *f.journal = append(*f.journal, "mount:"+f.name) }
func (f *fake) Start(context.Context) error {
	*f.journal = append(*f.journal, "start:"+f.name)
	return f.startErr
}
func (f *fake) Shutdown(context.Context) error {
	*f.journal = append(*f.journal, "stop:"+f.name)
	return nil
}

var _ Module = (*fake)(nil)

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("secuencia = %v, se esperaba %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("secuencia = %v, se esperaba %v", got, want)
		}
	}
}

func TestShutdownOcurreEnOrdenInverso(t *testing.T) {
	var journal []string
	reg := NewRegistry(quietLogger(),
		&fake{name: "a", journal: &journal},
		&fake{name: "b", journal: &journal},
		&fake{name: "c", journal: &journal},
	)

	if err := reg.Start(context.Background()); err != nil {
		t.Fatalf("Start devolvió error: %v", err)
	}
	reg.Shutdown(context.Background())

	equal(t, journal, []string{
		"start:a", "start:b", "start:c",
		"stop:c", "stop:b", "stop:a",
	})
}

// Un arranque a medias no puede dejar módulos corriendo: si el tercero falla, el
// primero y el segundo tienen que apagarse solos.
func TestStartFallidoApagaLoQueYaArranco(t *testing.T) {
	var journal []string
	boom := errors.New("no arrancó")
	reg := NewRegistry(quietLogger(),
		&fake{name: "a", journal: &journal},
		&fake{name: "b", journal: &journal},
		&fake{name: "c", journal: &journal, startErr: boom},
	)

	err := reg.Start(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("Start devolvió %v, se esperaba %v", err, boom)
	}

	// 'c' falló, así que nunca se dio por arrancado y no se apaga.
	equal(t, journal, []string{
		"start:a", "start:b", "start:c",
		"stop:b", "stop:a",
	})
}

// Un Start cancelado no debe impedir el apagado: si Shutdown heredara el
// contexto muerto, los módulos ya arrancados se quedarían sin drenar.
func TestStartFallidoApagaAunqueElContextoEsteCancelado(t *testing.T) {
	var journal []string
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reg := NewRegistry(quietLogger(),
		&fake{name: "a", journal: &journal},
		&fake{name: "b", journal: &journal, startErr: errors.New("no arrancó")},
	)
	_ = reg.Start(ctx)

	equal(t, journal, []string{"start:a", "start:b", "stop:a"})
}

func TestShutdownSinStartNoHaceNada(t *testing.T) {
	var journal []string
	reg := NewRegistry(quietLogger(), &fake{name: "a", journal: &journal})

	reg.Shutdown(context.Background())

	equal(t, journal, nil)
}

// Shutdown se llama dos veces en el camino de error (una desde Start, otra desde
// el apagado del servidor). La segunda tiene que ser un no-op.
func TestShutdownEsIdempotente(t *testing.T) {
	var journal []string
	reg := NewRegistry(quietLogger(), &fake{name: "a", journal: &journal})

	_ = reg.Start(context.Background())
	reg.Shutdown(context.Background())
	reg.Shutdown(context.Background())

	equal(t, journal, []string{"start:a", "stop:a"})
}

func TestNamesConservaElOrdenDeDeclaracion(t *testing.T) {
	var journal []string
	reg := NewRegistry(quietLogger(),
		&fake{name: "mirror", journal: &journal},
		&fake{name: "passport", journal: &journal},
	)

	equal(t, reg.Names(), []string{"mirror", "passport"})
}

// Comprueba que el contrato es implementable sin nada del proyecto: si Module
// creciera con un método que exige un tipo del kernel, este fake dejaría de
// compilar y ese es exactamente el aviso que queremos.
var _ = func(r Router) { r.Authed("GET /x", http.NotFoundHandler()) }
