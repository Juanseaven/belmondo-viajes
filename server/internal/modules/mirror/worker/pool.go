// Package worker procesa los eventos de webhook fuera del ciclo de petición HTTP.
//
// El problema que resuelve: un pool plano de goroutines desordena los mensajes de
// una misma conversación. Dos eventos del mismo chat pueden tomarse a la vez y
// persistirse invertidos, dejando el historial mal en el panel.
//
// La solución es particionar, como las particiones de Kafka: hash(clave) elige el
// shard, y cada shard es UNA goroutine con su propia cola. Todos los eventos de
// una conversación caen siempre en la misma goroutine — orden garantizado —
// mientras conversaciones distintas siguen procesándose en paralelo.
package worker

import (
	"context"
	"errors"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"
)

// ErrQueueFull lo devuelve Submit cuando el shard destino está saturado.
//
// El handler HTTP lo traduce a 503. NO se descarta el evento: Evolution reintenta
// con backoff exponencial, así que la contrapresión degrada la latencia, nunca la
// integridad. Descartar aquí sería perder un mensaje de un cliente.
var ErrQueueFull = errors.New("cola del worker llena")

const (
	defaultJobTimeout     = 30 * time.Second
	historySyncJobTimeout = 5 * time.Minute
)

// Job es una unidad de trabajo ya persistida en webhook_events.
type Job struct {
	EventID      int64  // fila en webhook_events, para marcarla procesada
	Event        string // messages.upsert, connection.update, ...
	State        string // root-level state used by Receipt events
	Instance     string
	PartitionKey string // instancia|remoteJid — determina el shard
	Payload      []byte // el `data` crudo
	ReceivedAt   time.Time
}

// Handler es la lógica de negocio. El pool no sabe qué hace: solo garantiza
// cuándo y en qué orden se llama.
type Handler interface {
	Handle(ctx context.Context, job Job) error
}

type Pool struct {
	shards  []chan Job
	handler Handler
	log     *slog.Logger
	wg      sync.WaitGroup

	closeOnce sync.Once
	done      chan struct{} // cerrado al iniciar el apagado; corta Submit
}

// New crea el pool. shards controla el paralelismo ENTRE conversaciones;
// queueSize es el buffer de cada shard antes de aplicar contrapresión.
func New(shards, queueSize int, h Handler, log *slog.Logger) *Pool {
	if shards < 1 {
		shards = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	p := &Pool{
		shards:  make([]chan Job, shards),
		handler: h,
		log:     log,
		done:    make(chan struct{}),
	}
	for i := range p.shards {
		p.shards[i] = make(chan Job, queueSize)
	}
	return p
}

// Start lanza una goroutine por shard.
func (p *Pool) Start(ctx context.Context) {
	for i := range p.shards {
		p.wg.Add(1)
		go p.run(ctx, i, p.shards[i])
	}
	p.log.Info("pool de workers iniciado",
		"shards", len(p.shards), "queue_size", cap(p.shards[0]))
}

// Submit encola sin bloquear nunca. Se llama desde el handler HTTP, donde
// bloquear significaría agotar las conexiones del servidor bajo una ráfaga.
func (p *Pool) Submit(job Job) error {
	select {
	case <-p.done:
		return errors.New("pool cerrado")
	default:
	}

	shard := p.shardFor(job.PartitionKey)
	select {
	case p.shards[shard] <- job:
		return nil
	default:
		// Sin `default` esto bloquearía la goroutine del request. Preferimos
		// devolver 503 y dejar que Evolution reintente.
		p.log.Warn("shard saturado, aplicando contrapresión",
			"shard", shard, "event", job.Event, "instance", job.Instance)
		return ErrQueueFull
	}
}

// run es el bucle de un shard. Al ser una única goroutine por canal, el orden de
// procesamiento dentro del shard es exactamente el de encolado.
func (p *Pool) run(ctx context.Context, id int, queue <-chan Job) {
	defer p.wg.Done()

	for job := range queue {
		// Timeout por job: una consulta colgada no puede paralizar todas las
		// conversaciones que comparten este shard. Un HistorySync es un lote
		// completo y necesita más margen que un mensaje individual.
		timeout := defaultJobTimeout
		if job.Event == "HistorySync" {
			timeout = historySyncJobTimeout
		}
		jobCtx, cancel := context.WithTimeout(ctx, timeout)
		err := p.safeHandle(jobCtx, job)
		cancel()

		if err != nil {
			// No se reintenta aquí: el evento ya está en webhook_events con
			// status='failed' y un reprocesador lo recogerá. Reintentar dentro
			// del shard bloquearía el resto de la conversación.
			p.log.Error("job fallido",
				"shard", id, "event_id", job.EventID, "event", job.Event, "error", err)
			continue
		}

		if lag := time.Since(job.ReceivedAt); lag > 5*time.Second {
			p.log.Warn("latencia alta de procesamiento",
				"shard", id, "lag", lag, "event", job.Event)
		}
	}
	p.log.Debug("shard drenado", "shard", id)
}

// safeHandle aísla el pánico de un job. Sin esto, un payload inesperado de
// WhatsApp mata la goroutine y todas las conversaciones de ese shard dejan de
// procesarse en silencio.
func (p *Pool) safeHandle(ctx context.Context, job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			p.log.Error("pánico recuperado en worker",
				"event_id", job.EventID, "panic", r)
			err = errors.New("pánico procesando el job")
		}
	}()
	return p.handler.Handle(ctx, job)
}

// shardFor fija cada conversación a un shard. FNV-1a: barato y con buena
// dispersión, que es todo lo que necesitamos (no es criptográfico).
func (p *Pool) shardFor(key string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(len(p.shards)))
}

// Shutdown deja de aceptar trabajo y espera a que las colas se drenen.
//
// Importante para no perder mensajes en un despliegue: se cierran los canales, se
// espera a que los workers terminen lo encolado, y si ctx expira antes se
// devuelve error para que el proceso salga con código distinto de cero.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.closeOnce.Do(func() {
		close(p.done)
		for _, ch := range p.shards {
			close(ch)
		}
	})

	drained := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		p.log.Info("pool de workers drenado limpiamente")
		return nil
	case <-ctx.Done():
		return errors.New("timeout drenando el pool: puede haber eventos sin procesar")
	}
}

// Depth expone la ocupación de cada shard. Sirve de métrica de salud: si un shard
// crece de forma sostenida, hay una conversación que genera más eventos de los
// que se pueden procesar.
func (p *Pool) Depth() []int {
	d := make([]int, len(p.shards))
	for i, ch := range p.shards {
		d[i] = len(ch)
	}
	return d
}
