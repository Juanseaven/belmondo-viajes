package mirror

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

type avatarTargets struct {
	target store.SendTarget
	err    error
	calls  atomic.Int32
}

func (f *avatarTargets) Target(context.Context, string) (store.SendTarget, error) {
	f.calls.Add(1)
	return f.target, f.err
}

type avatarProviderFunc func(context.Context, string, string) (string, error)

func (f avatarProviderFunc) AvatarURL(ctx context.Context, instance, jid string) (string, error) {
	return f(ctx, instance, jid)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func avatarTarget() store.SendTarget {
	return store.SendTarget{
		ConversationID: "conversation-1", InstanceName: "line-1",
		RemoteJID: "5491112345678@s.whatsapp.net", LineStatus: "open", LineActive: true,
	}
}

func imageResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAvatarLoadCachesAValidWhatsAppImageAndStillResolvesTarget(t *testing.T) {
	targets := &avatarTargets{target: avatarTarget()}
	var providerCalls, originCalls atomic.Int32
	provider := avatarProviderFunc(func(_ context.Context, instance, jid string) (string, error) {
		providerCalls.Add(1)
		if instance != "line-1" || jid != "5491112345678@s.whatsapp.net" {
			t.Fatalf("target = %q %q", instance, jid)
		}
		return "https://pps.whatsapp.net/avatar.jpg", nil
	})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		originCalls.Add(1)
		return imageResponse("jpeg-data"), nil
	})}
	service := NewAvatarService(targets, provider, client)

	for range 2 {
		got, err := service.Load(context.Background(), "conversation-1")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if string(got.Data) != "jpeg-data" || got.ContentType != "image/jpeg" {
			t.Fatalf("image = %+v", got)
		}
	}
	if providerCalls.Load() != 1 || originCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, origin calls = %d", providerCalls.Load(), originCalls.Load())
	}
	if targets.calls.Load() != 2 {
		t.Fatalf("target calls = %d, want one per request", targets.calls.Load())
	}
}

func TestAvatarLoadRejectsUnsafeURLsBeforeDownload(t *testing.T) {
	for _, rawURL := range []string{
		"http://pps.whatsapp.net/avatar.jpg",
		"https://whatsapp.net.evil.example/avatar.jpg",
	} {
		t.Run(rawURL, func(t *testing.T) {
			var originCalls atomic.Int32
			service := NewAvatarService(
				&avatarTargets{target: avatarTarget()},
				avatarProviderFunc(func(context.Context, string, string) (string, error) { return rawURL, nil }),
				&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					originCalls.Add(1)
					return imageResponse("bad"), nil
				})},
			)
			if _, err := service.Load(context.Background(), "conversation-1"); !errors.Is(err, ErrAvatarUnavailable) {
				t.Fatalf("error = %v, want ErrAvatarUnavailable", err)
			}
			if originCalls.Load() != 0 {
				t.Fatalf("origin calls = %d", originCalls.Load())
			}
		})
	}
}

func TestAvatarLoadRejectsRedirectOutsideWhatsApp(t *testing.T) {
	var originCalls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		originCalls.Add(1)
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://evil.example/avatar.jpg"}},
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    r,
		}, nil
	})}
	service := NewAvatarService(
		&avatarTargets{target: avatarTarget()},
		avatarProviderFunc(func(context.Context, string, string) (string, error) {
			return "https://pps.whatsapp.net/avatar.jpg", nil
		}), client,
	)
	if _, err := service.Load(context.Background(), "conversation-1"); !errors.Is(err, ErrAvatarUnavailable) {
		t.Fatalf("error = %v, want ErrAvatarUnavailable", err)
	}
	if originCalls.Load() != 1 {
		t.Fatalf("origin calls = %d, redirect target must not be fetched", originCalls.Load())
	}
}

func TestAvatarLoadRejectsInvalidContentAndOversize(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "not an image", contentType: "text/html", body: "hello"},
		{name: "too large", contentType: "image/png", body: strings.Repeat("x", 11)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewAvatarService(
				&avatarTargets{target: avatarTarget()},
				avatarProviderFunc(func(context.Context, string, string) (string, error) {
					return "https://pps.whatsapp.net/avatar", nil
				}),
				&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					resp := imageResponse(tt.body)
					resp.Header.Set("Content-Type", tt.contentType)
					return resp, nil
				})},
			)
			service.maxImageBytes = 10
			if _, err := service.Load(context.Background(), "conversation-1"); !errors.Is(err, ErrAvatarUnavailable) {
				t.Fatalf("error = %v, want ErrAvatarUnavailable", err)
			}
		})
	}
}

func TestAvatarLoadCoalescesConcurrentRequestsAndCachesAbsence(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	provider := avatarProviderFunc(func(context.Context, string, string) (string, error) {
		calls.Add(1)
		<-release
		return "", errors.New("hidden")
	})
	service := NewAvatarService(&avatarTargets{target: avatarTarget()}, provider, http.DefaultClient)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Load(context.Background(), "conversation-1")
			errs <- err
		}()
	}
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, ErrAvatarUnavailable) {
			t.Fatalf("error = %v", err)
		}
	}
	if _, err := service.Load(context.Background(), "conversation-1"); !errors.Is(err, ErrAvatarUnavailable) {
		t.Fatalf("cached error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", calls.Load())
	}
}

func TestAvatarLoadDoesNotLetTheFirstCallerCancelSharedWork(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	provider := avatarProviderFunc(func(context.Context, string, string) (string, error) {
		once.Do(func() { close(started) })
		<-release
		return "https://pps.whatsapp.net/avatar.jpg", nil
	})
	service := NewAvatarService(
		&avatarTargets{target: avatarTarget()}, provider,
		&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return imageResponse("shared-avatar"), nil
		})},
	)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Load(firstCtx, "conversation-1")
		firstDone <- err
	}()
	<-started
	cancelFirst()
	select {
	case err := <-firstDone:
		if !errors.Is(err, ErrAvatarUnavailable) {
			t.Fatalf("first caller error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		close(release)
		t.Fatal("cancelling the first caller cancelled or blocked shared work")
	}

	secondDone := make(chan error, 1)
	go func() {
		image, err := service.Load(context.Background(), "conversation-1")
		if err == nil && string(image.Data) != "shared-avatar" {
			err = errors.New("unexpected shared image")
		}
		secondDone <- err
	}()
	close(release)
	if err := <-secondDone; err != nil {
		t.Fatalf("second caller: %v", err)
	}
}

func TestAvatarLoadRefreshesExpiredEntryAndEvictsOldestWithinBudget(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	var providerCalls atomic.Int32
	targets := &avatarTargets{target: avatarTarget()}
	provider := avatarProviderFunc(func(context.Context, string, string) (string, error) {
		n := providerCalls.Add(1)
		return "https://pps.whatsapp.net/avatar/" + string(rune('0'+n)), nil
	})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return imageResponse(r.URL.Path[len(r.URL.Path)-1:]), nil
	})}
	service := NewAvatarService(targets, provider, client)
	service.now = func() time.Time { return now }
	service.maxCacheBytes = 2

	if _, err := service.Load(context.Background(), "conversation-1"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(6*time.Hour + time.Second)
	if _, err := service.Load(context.Background(), "conversation-1"); err != nil {
		t.Fatal(err)
	}
	if providerCalls.Load() != 2 {
		t.Fatalf("provider calls after expiry = %d", providerCalls.Load())
	}

	targets.target.ConversationID = "conversation-2"
	now = now.Add(time.Second)
	if _, err := service.Load(context.Background(), "conversation-2"); err != nil {
		t.Fatal(err)
	}
	targets.target.ConversationID = "conversation-3"
	now = now.Add(time.Second)
	if _, err := service.Load(context.Background(), "conversation-3"); err != nil {
		t.Fatal(err)
	}
	if service.cacheBytes > service.maxCacheBytes {
		t.Fatalf("cache bytes = %d, budget = %d", service.cacheBytes, service.maxCacheBytes)
	}
	if _, ok := service.cache["conversation-1"]; ok {
		t.Fatal("oldest avatar was not evicted")
	}
}

func TestAvatarLoadPreservesStoreErrorsAndRejectsInactiveLines(t *testing.T) {
	dbErr := errors.New("database unavailable")
	service := NewAvatarService(&avatarTargets{err: dbErr}, nil, nil)
	if _, err := service.Load(context.Background(), "missing"); !errors.Is(err, dbErr) {
		t.Fatalf("error = %v", err)
	}

	target := avatarTarget()
	target.LineActive = false
	service = NewAvatarService(&avatarTargets{target: target}, nil, nil)
	if _, err := service.Load(context.Background(), "conversation-1"); !errors.Is(err, ErrAvatarUnavailable) {
		t.Fatalf("inactive line error = %v", err)
	}
}

func TestServeAvatarMapsSuccessAbsenceAndStoreFailure(t *testing.T) {
	newModule := func(targets *avatarTargets, provider avatarProvider) *Module {
		m := newTestModule(&fakeRead{})
		m.avatars = NewAvatarService(targets, provider, &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return imageResponse("avatar-bytes"), nil
			}),
		})
		return m
	}

	t.Run("success", func(t *testing.T) {
		m := newModule(&avatarTargets{target: avatarTarget()}, avatarProviderFunc(
			func(context.Context, string, string) (string, error) {
				return "https://pps.whatsapp.net/avatar.jpg", nil
			},
		))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetPathValue("id", "conversation-1")
		m.serveAvatar(rec, req)

		if rec.Code != http.StatusOK || rec.Body.String() != "avatar-bytes" {
			t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "private, max-age=21600" {
			t.Fatalf("Cache-Control = %q", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		for _, targets := range []*avatarTargets{
			{err: store.ErrNotFound},
			{target: avatarTarget()},
		} {
			provider := avatarProviderFunc(func(context.Context, string, string) (string, error) {
				return "", errors.New("hidden")
			})
			m := newModule(targets, provider)
			rec := do(m.serveAvatar, "/", map[string]string{"id": "conversation-1"})
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "private, max-age=900" {
				t.Fatalf("Cache-Control = %q", got)
			}
		}
	})

	t.Run("store failure", func(t *testing.T) {
		m := newModule(&avatarTargets{err: errors.New("database unavailable")}, nil)
		rec := do(m.serveAvatar, "/", map[string]string{"id": "conversation-1"})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
}
