package media

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

func TestProcessUploadsInlineWebhookMediaWithoutCallingEvolution(t *testing.T) {
	t.Parallel()

	downloader := &mediaDownloaderFake{}
	blobs := &mediaBlobsFake{}
	database := &mediaStoreFake{}
	service := NewService(downloader, blobs, database, 1,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := service.process(context.Background(), store.PendingMedia{
		ID: "media-1", InstanceName: "juanse", WAMessageID: "WA-1",
		MimeType: "image/jpeg", FileName: "foto.jpg", Data: []byte("image bytes"),
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if downloader.called {
		t.Fatal("Evolution downloader was called despite inline webhook bytes")
	}
	if string(blobs.data) != "image bytes" || database.storedID != "media-1" {
		t.Fatalf("upload/store = %q / %q", blobs.data, database.storedID)
	}
}

// El panel muestra «Descargando adjunto…» hasta que se entera de que el
// archivo quedó guardado: el aviso sale recién después de guardarlo.
func TestProcessAnnouncesStoredMedia(t *testing.T) {
	t.Parallel()

	database := &mediaStoreFake{}
	service := NewService(&mediaDownloaderFake{}, &mediaBlobsFake{}, database, 1,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	var announced store.PendingMedia
	service.OnStored(func(_ context.Context, job store.PendingMedia) {
		if database.storedID == "" {
			t.Error("announced before the media was stored")
		}
		announced = job
	})

	if err := service.process(context.Background(), store.PendingMedia{
		ID: "media-2", MessageID: "msg-2", InstanceName: "juanse", WAMessageID: "WA-2",
		MimeType: "image/jpeg", Data: []byte("bytes"),
	}); err != nil {
		t.Fatalf("process: %v", err)
	}
	if announced.ID != "media-2" || announced.MessageID != "msg-2" {
		t.Fatalf("announced = %+v", announced)
	}
}

type mediaDownloaderFake struct{ called bool }

func (f *mediaDownloaderFake) DownloadMedia(context.Context, string, string) ([]byte, string, error) {
	f.called = true
	return nil, "", nil
}

type mediaBlobsFake struct{ data []byte }

func (f *mediaBlobsFake) Put(_ context.Context, _, _ string, data []byte) error {
	f.data = data
	return nil
}
func (*mediaBlobsFake) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (*mediaBlobsFake) DeletePrefix(context.Context, string) error { return nil }

type mediaStoreFake struct {
	storedID        string
	processingCalls int
}

func (*mediaStoreFake) NextPending(context.Context, int) ([]store.PendingMedia, error) {
	return nil, nil
}
func (f *mediaStoreFake) MarkProcessing(context.Context, string) error {
	f.processingCalls++
	return nil
}
func (f *mediaStoreFake) MarkStored(_ context.Context, id, _, _ string, _ int64) error {
	f.storedID = id
	return nil
}
func (*mediaStoreFake) MarkFailed(context.Context, string, string) error { return nil }
func (*mediaStoreFake) StorageKey(context.Context, string) (string, string, error) {
	return "", "", nil
}

func TestProcessDoesNotClaimRecoveredMediaTwice(t *testing.T) {
	database := &mediaStoreFake{}
	service := NewService(&mediaDownloaderFake{}, &mediaBlobsFake{}, database, 1,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := service.process(context.Background(), store.PendingMedia{
		ID: "media-claimed", InstanceName: "juanse", WAMessageID: "WA-CLAIMED",
		MimeType: "image/jpeg", Data: []byte("image bytes"), Attempts: 5, Claimed: true,
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if database.processingCalls != 0 {
		t.Fatalf("MarkProcessing calls = %d, want 0", database.processingCalls)
	}
}
