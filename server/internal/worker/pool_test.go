package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type recorder struct {
	mu    sync.Mutex
	order map[string][]int64
}

func (r *recorder) Handle(_ context.Context, job Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order[job.PartitionKey] = append(r.order[job.PartitionKey], job.EventID)
	return nil
}

// La garantía central del diseño: los eventos de una misma conversación se
// procesan en el orden en que llegaron, aunque haya varios shards activos.
func TestOrdenPreservadoPorConversacion(t *testing.T) {
	rec := &recorder{order: map[string][]int64{}}
	p := New(8, 128, rec, quietLogger())
	p.Start(context.Background())

	const perChat = 50
	chats := []string{"linea-ana|573001@s.whatsapp.net", "linea-luis|573002@s.whatsapp.net"}

	for i := 0; i < perChat; i++ {
		for _, chat := range chats {
			if err := p.Submit(Job{EventID: int64(i), PartitionKey: chat}); err != nil {
				t.Fatalf("submit: %v", err)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	for _, chat := range chats {
		got := rec.order[chat]
		if len(got) != perChat {
			t.Fatalf("%s: se procesaron %d de %d eventos", chat, len(got), perChat)
		}
		for i, id := range got {
			if id != int64(i) {
				t.Fatalf("%s: desorden en la posición %d, se esperaba %d y llegó %d",
					chat, i, i, id)
			}
		}
	}
}

// Una conversación siempre cae en el mismo shard: es lo que hace posible la
// garantía anterior.
func TestShardEstablePorClave(t *testing.T) {
	p := New(16, 1, &recorder{order: map[string][]int64{}}, quietLogger())
	key := "linea-ana|573001@s.whatsapp.net"

	want := p.shardFor(key)
	for i := 0; i < 1000; i++ {
		if got := p.shardFor(key); got != want {
			t.Fatalf("shard inestable: %d vs %d", got, want)
		}
	}
}

// Con el shard lleno, Submit devuelve ErrQueueFull en vez de bloquear la
// goroutine del request. El handler lo traduce a 503 y Evolution reintenta.
//
// El pool NO se arranca a propósito: con workers vivos, cuántos jobs caben antes
// de saturar depende de si alguno ya fue desencolado, y el test se vuelve una
// carrera. Sin consumidores, la capacidad es exactamente queueSize.
func TestContrapresionNoBloquea(t *testing.T) {
	const queueSize = 2
	p := New(1, queueSize, &recorder{order: map[string][]int64{}}, quietLogger())

	key := "linea-ana|573001@s.whatsapp.net"
	for i := 0; i < queueSize; i++ {
		if err := p.Submit(Job{EventID: int64(i), PartitionKey: key}); err != nil {
			t.Fatalf("submit %d no debía fallar: %v", i, err)
		}
	}

	// El siguiente debe volver de inmediato con error, no esperar hueco.
	done := make(chan error, 1)
	go func() { done <- p.Submit(Job{EventID: 99, PartitionKey: key}) }()

	select {
	case err := <-done:
		if !errors.Is(err, ErrQueueFull) {
			t.Fatalf("se esperaba ErrQueueFull, llegó %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Submit bloqueó: agotaría las conexiones HTTP bajo ráfaga")
	}
}

type panicker struct{}

func (panicker) Handle(context.Context, Job) error { panic("payload inesperado") }

// Un pánico no puede matar la goroutine del shard: si lo hiciera, todas las
// conversaciones de ese shard dejarían de procesarse en silencio.
func TestPanicoNoMataElShard(t *testing.T) {
	p := New(1, 4, panicker{}, quietLogger())
	p.Start(context.Background())

	if err := p.Submit(Job{EventID: 1, PartitionKey: "a"}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("el shard murió por el pánico: %v", err)
	}
}
