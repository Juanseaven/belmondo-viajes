package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

const (
	avatarTTL         = 6 * time.Hour
	avatarNegativeTTL = 15 * time.Minute
	avatarTimeout     = 8 * time.Second
	avatarMaxBytes    = 2 << 20
	avatarCacheBudget = 32 << 20
)

var ErrAvatarUnavailable = errors.New("avatar no disponible")

type avatarTargetStore interface {
	Target(context.Context, string) (store.SendTarget, error)
}

type avatarProvider interface {
	AvatarURL(context.Context, string, string) (string, error)
}

type AvatarImage struct {
	Data        []byte
	ContentType string
}

type avatarCacheEntry struct {
	image       AvatarImage
	unavailable bool
	expiresAt   time.Time
	lastUsed    time.Time
}

type avatarCall struct {
	done  chan struct{}
	image AvatarImage
	err   error
}

type AvatarService struct {
	targets  avatarTargetStore
	provider avatarProvider
	http     *http.Client
	now      func() time.Time

	maxImageBytes int64
	maxCacheBytes int64

	mu         sync.Mutex
	cache      map[string]avatarCacheEntry
	inflight   map[string]*avatarCall
	cacheBytes int64
}

func NewAvatarService(targets avatarTargetStore, provider avatarProvider, httpClient *http.Client) *AvatarService {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	client := *httpClient
	previousCheck := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("demasiadas redirecciones de avatar")
		}
		if err := validateAvatarURL(req.URL); err != nil {
			return err
		}
		if previousCheck != nil {
			return previousCheck(req, via)
		}
		return nil
	}
	return &AvatarService{
		targets:       targets,
		provider:      provider,
		http:          &client,
		now:           time.Now,
		maxImageBytes: avatarMaxBytes,
		maxCacheBytes: avatarCacheBudget,
		cache:         make(map[string]avatarCacheEntry),
		inflight:      make(map[string]*avatarCall),
	}
}

func (s *AvatarService) Load(ctx context.Context, conversationID string) (AvatarImage, error) {
	target, err := s.targets.Target(ctx, conversationID)
	if err != nil {
		return AvatarImage{}, err
	}
	if !target.LineActive {
		return AvatarImage{}, ErrAvatarUnavailable
	}

	if image, err, ok := s.cached(conversationID); ok {
		return image, err
	}

	s.mu.Lock()
	if image, err, ok := s.cachedLocked(conversationID, s.now()); ok {
		s.mu.Unlock()
		return image, err
	}
	if pending := s.inflight[conversationID]; pending != nil {
		s.mu.Unlock()
		select {
		case <-pending.done:
			return pending.image, pending.err
		case <-ctx.Done():
			return AvatarImage{}, fmt.Errorf("%w: %v", ErrAvatarUnavailable, ctx.Err())
		}
	}
	call := &avatarCall{done: make(chan struct{})}
	s.inflight[conversationID] = call
	s.mu.Unlock()

	// La carga pertenece al servicio, no al primer navegador que llegó. Si ese
	// request se cancela (cambio de conversación, navegación), los demás callers
	// siguen esperando el mismo trabajo y una foto válida no se cachea como 404.
	sharedCtx := context.WithoutCancel(ctx)
	go func() {
		call.image, call.err = s.loadFresh(sharedCtx, target)
		s.finish(conversationID, call)
	}()
	return waitAvatarCall(ctx, call)
}

func waitAvatarCall(ctx context.Context, call *avatarCall) (AvatarImage, error) {
	select {
	case <-call.done:
		return call.image, call.err
	case <-ctx.Done():
		return AvatarImage{}, fmt.Errorf("%w: %v", ErrAvatarUnavailable, ctx.Err())
	}
}

func (s *AvatarService) loadFresh(ctx context.Context, target store.SendTarget) (AvatarImage, error) {
	ctx, cancel := context.WithTimeout(ctx, avatarTimeout)
	defer cancel()

	rawURL, err := s.provider.AvatarURL(ctx, target.InstanceName, target.RemoteJID)
	if err != nil {
		return AvatarImage{}, fmt.Errorf("%w: %v", ErrAvatarUnavailable, err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return AvatarImage{}, fmt.Errorf("%w: URL inválida", ErrAvatarUnavailable)
	}
	if err := validateAvatarURL(parsed); err != nil {
		return AvatarImage{}, fmt.Errorf("%w: %v", ErrAvatarUnavailable, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return AvatarImage{}, fmt.Errorf("%w: %v", ErrAvatarUnavailable, err)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return AvatarImage{}, fmt.Errorf("%w: %v", ErrAvatarUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AvatarImage{}, fmt.Errorf("%w: origen devolvió %d", ErrAvatarUnavailable, resp.StatusCode)
	}

	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return AvatarImage{}, fmt.Errorf("%w: tipo de contenido inválido", ErrAvatarUnavailable)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, s.maxImageBytes+1))
	if err != nil {
		return AvatarImage{}, fmt.Errorf("%w: %v", ErrAvatarUnavailable, err)
	}
	if int64(len(data)) > s.maxImageBytes {
		return AvatarImage{}, fmt.Errorf("%w: imagen demasiado grande", ErrAvatarUnavailable)
	}
	return AvatarImage{Data: data, ContentType: contentType}, nil
}

func validateAvatarURL(u *url.URL) error {
	host := strings.ToLower(u.Hostname())
	if u.Scheme != "https" || (host != "whatsapp.net" && !strings.HasSuffix(host, ".whatsapp.net")) {
		return errors.New("origen de avatar no permitido")
	}
	return nil
}

func (s *AvatarService) cached(conversationID string) (AvatarImage, error, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cachedLocked(conversationID, s.now())
}

func (s *AvatarService) cachedLocked(conversationID string, now time.Time) (AvatarImage, error, bool) {
	entry, ok := s.cache[conversationID]
	if !ok {
		return AvatarImage{}, nil, false
	}
	if !now.Before(entry.expiresAt) {
		s.cacheBytes -= int64(len(entry.image.Data))
		delete(s.cache, conversationID)
		return AvatarImage{}, nil, false
	}
	entry.lastUsed = now
	s.cache[conversationID] = entry
	if entry.unavailable {
		return AvatarImage{}, ErrAvatarUnavailable, true
	}
	return entry.image, nil, true
}

func (s *AvatarService) finish(conversationID string, call *avatarCall) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	entry := avatarCacheEntry{lastUsed: now}
	if call.err != nil {
		entry.unavailable = true
		entry.expiresAt = now.Add(avatarNegativeTTL)
	} else {
		entry.image = call.image
		entry.expiresAt = now.Add(avatarTTL)
		s.makeRoomLocked(int64(len(call.image.Data)), now)
		s.cacheBytes += int64(len(call.image.Data))
	}
	s.cache[conversationID] = entry
	delete(s.inflight, conversationID)
	close(call.done)
}

func (s *AvatarService) makeRoomLocked(incoming int64, now time.Time) {
	for key, entry := range s.cache {
		if !now.Before(entry.expiresAt) {
			s.cacheBytes -= int64(len(entry.image.Data))
			delete(s.cache, key)
		}
	}
	for s.cacheBytes+incoming > s.maxCacheBytes {
		var oldestKey string
		var oldest time.Time
		for key, entry := range s.cache {
			if entry.unavailable {
				continue
			}
			if oldestKey == "" || entry.lastUsed.Before(oldest) {
				oldestKey, oldest = key, entry.lastUsed
			}
		}
		if oldestKey == "" {
			break
		}
		entry := s.cache[oldestKey]
		s.cacheBytes -= int64(len(entry.image.Data))
		delete(s.cache, oldestKey)
	}
}
