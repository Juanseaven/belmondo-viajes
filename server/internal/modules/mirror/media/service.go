// Package media baja los adjuntos de Evolution y los sube a R2.
//
// Va en un pool aparte del de eventos, a propósito: subir un pasaporte de 4 MB
// tarda cientos de milisegundos y no puede ocupar un shard de conversaciones.
// La ingesta de mensajes nunca espera al media.
package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"mime"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/blobs"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// Downloader es Evolution visto por este paquete: lo único que necesita es
// convertir un id de mensaje en bytes descifrados.
type Downloader interface {
	DownloadMedia(ctx context.Context, instance, waMessageID string) ([]byte, string, error)
}

const maxAttempts = 5

type Service struct {
	down   Downloader
	up     blobs.Store
	store  store.MediaStore
	jobs   chan store.PendingMedia
	wg     sync.WaitGroup
	log    *slog.Logger
	closer sync.Once
}

func NewService(d Downloader, u blobs.Store, s store.MediaStore, queueSize int, log *slog.Logger) *Service {
	if queueSize < 1 {
		queueSize = 128
	}
	return &Service{
		down: d, up: u, store: s,
		jobs: make(chan store.PendingMedia, queueSize),
		log:  log,
	}
}

func (s *Service) Start(ctx context.Context, workers int) {
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.run(ctx, i)
	}
	s.log.Info("pipeline de media iniciado", "workers", workers)
}

// Enqueue lo llama el processor. No bloquea: si la cola está llena, el adjunto se
// queda en 'pending' en la base y lo recogerá el barrido de reintentos. Perder el
// puesto en la cola no es perder el archivo.
func (s *Service) Enqueue(job store.PendingMedia) {
	select {
	case s.jobs <- job:
	default:
		s.log.Warn("cola de media llena, queda pendiente en la base",
			"media_id", job.ID)
	}
}

func (s *Service) run(ctx context.Context, id int) {
	defer s.wg.Done()
	for job := range s.jobs {
		jobCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		if err := s.process(jobCtx, job); err != nil {
			s.log.Error("fallo procesando media",
				"worker", id, "media_id", job.ID, "attempts", job.Attempts, "error", err)
			_ = s.store.MarkFailed(context.WithoutCancel(jobCtx), job.ID, err.Error())
		}
		cancel()
	}
}

func (s *Service) process(ctx context.Context, job store.PendingMedia) error {
	if job.Attempts >= maxAttempts {
		return fmt.Errorf("agotados los %d intentos", maxAttempts)
	}
	if err := s.store.MarkProcessing(ctx, job.ID); err != nil {
		return fmt.Errorf("marcando en proceso: %w", err)
	}

	data, mimeType, err := s.down.DownloadMedia(ctx, job.InstanceName, job.WAMessageID)
	if err != nil {
		return fmt.Errorf("descargando de evolution: %w", err)
	}
	if mimeType == "" {
		mimeType = job.MimeType
	}

	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	key := StorageKey(job.InstanceName, job.WAMessageID, job.FileName, mimeType)

	if err := s.up.Put(ctx, key, mimeType, data); err != nil {
		return fmt.Errorf("subiendo a R2: %w", err)
	}
	return s.store.MarkStored(ctx, job.ID, key, digest, int64(len(data)))
}

// StorageKey construye la ruta en R2:
//
//	{instancia}/{yyyy-mm}/{wa_message_id}.{ext}
//
// El prefijo por instancia permite reglas de ciclo de vida y borrados por línea
// (por ejemplo, cuando un vendedor deja la agencia); el de mes evita directorios
// con cientos de miles de objetos.
func StorageKey(instance, waMessageID, fileName, mimeType string) string {
	ext := filepath.Ext(fileName)
	if ext == "" {
		if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		ext = ".bin"
	}
	// Los audios nativos de WhatsApp llegan como "audio/ogg; codecs=opus" y el
	// resolutor de mime devuelve .oga, que muchos navegadores no reproducen.
	if strings.HasPrefix(mimeType, "audio/ogg") {
		ext = ".ogg"
	}
	return fmt.Sprintf("%s/%s/%s%s",
		sanitize(instance), time.Now().UTC().Format("2006-01"), sanitize(waMessageID), ext)
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}

// Requeue reencola los adjuntos que quedaron pendientes o fallaron. Se llama al
// arrancar (recupera lo perdido en un reinicio) y de forma periódica.
func (s *Service) Requeue(ctx context.Context, limit int) error {
	pending, err := s.store.NextPending(ctx, limit)
	if err != nil {
		return err
	}
	for _, job := range pending {
		s.Enqueue(job)
	}
	if len(pending) > 0 {
		s.log.Info("media pendiente reencolada", "count", len(pending))
	}
	return nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.closer.Do(func() { close(s.jobs) })

	drained := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
