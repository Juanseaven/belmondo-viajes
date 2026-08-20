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

type mediaStoreFake struct{ storedID string }

func (*mediaStoreFake) NextPending(context.Context, int) ([]store.PendingMedia, error) {
	return nil, nil
}
func (*mediaStoreFake) MarkProcessing(context.Context, string) error { return nil }
func (f *mediaStoreFake) MarkStored(_ context.Context, id, _, _ string, _ int64) error {
	f.storedID = id
	return nil
}
func (*mediaStoreFake) MarkFailed(context.Context, string, string) error { return nil }
func (*mediaStoreFake) StorageKey(context.Context, string) (string, string, error) {
	return "", "", nil
}
