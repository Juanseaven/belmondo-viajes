// Package evolution is the outbound boundary to Evolution Go.
package evolution

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
)

var subscriptions = []string{
	"MESSAGE",
	"SEND_MESSAGE",
	"READ_RECEIPT",
	"HISTORY_SYNC",
	"CONNECTION",
	"CONTACT",
	"GROUP",
	"QRCODE",
}

type Client struct {
	baseURL    string
	globalKey  string
	webhookURL string
	http       *http.Client
}

func New(baseURL, globalKey, webhookURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		globalKey:  globalKey,
		webhookURL: webhookURL,
		http: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 32,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// instanceToken derives a stable, provider-specific credential without storing
// another secret in Postgres. A line name is unique and immutable in this app.
func (c *Client) instanceToken(instance string) string {
	return InstanceToken(c.globalKey, instance)
}

// InstanceToken derives the per-line token expected by Evolution Go.
func InstanceToken(globalKey, instance string) string {
	mac := hmac.New(sha256.New, []byte(globalKey))
	_, _ = mac.Write([]byte("evolution-go-instance:" + instance))
	return hex.EncodeToString(mac.Sum(nil))
}

type SendTextResult struct {
	Key struct {
		ID        string `json:"id"`
		RemoteJID string `json:"remoteJid"`
		FromMe    bool   `json:"fromMe"`
	} `json:"key"`
	Status           string `json:"status"`
	MessageTimestamp any    `json:"messageTimestamp"`
}

// Quote usa exactamente el contrato expuesto por Evolution Go 0.7.2.
type Quote struct {
	MessageID   string `json:"messageId"`
	Participant string `json:"participant,omitempty"`
}

type sendResponse struct {
	Data struct {
		Info struct {
			ID       string `json:"ID"`
			Chat     string `json:"Chat"`
			IsFromMe bool   `json:"IsFromMe"`
		} `json:"Info"`
	} `json:"data"`
}

func (c *Client) SendText(ctx context.Context, instance, number, text string, quote *Quote) (*SendTextResult, error) {
	body := map[string]any{
		"number": number,
		"text":   text,
		"delay":  800,
	}
	if quote != nil && quote.MessageID != "" {
		body["quoted"] = quote
	}
	var out sendResponse
	if err := c.do(ctx, http.MethodPost, "/send/text", c.instanceToken(instance), body, &out); err != nil {
		return nil, err
	}
	return normalizeSend(out)
}

func (c *Client) SendMedia(ctx context.Context, instance, number, mediaURL, mediatype, caption, fileName string, quote *Quote) (*SendTextResult, error) {
	body := map[string]any{
		"number":   number,
		"url":      mediaURL,
		"type":     mediatype,
		"caption":  caption,
		"filename": fileName,
	}
	if quote != nil && quote.MessageID != "" {
		body["quoted"] = quote
	}
	var out sendResponse
	if err := c.do(ctx, http.MethodPost, "/send/media", c.instanceToken(instance), body, &out); err != nil {
		return nil, err
	}
	return normalizeSend(out)
}

func normalizeSend(out sendResponse) (*SendTextResult, error) {
	if out.Data.Info.ID == "" {
		return nil, fmt.Errorf("Evolution Go aceptó el envío pero no devolvió data.Info.ID")
	}
	result := &SendTextResult{Status: "sent"}
	result.Key.ID = out.Data.Info.ID
	result.Key.RemoteJID = out.Data.Info.Chat
	result.Key.FromMe = out.Data.Info.IsFromMe
	return result, nil
}

// CreateInstance creates the provider record and starts exactly one WhatsApp
// client. The QR arrives asynchronously through the subscribed QRCODE webhook.
//
// Do not call /instance/qr immediately after /instance/connect: Evolution starts
// the client in a goroutine, so that endpoint can still see no runtime client
// and start a second QR loop. Its timeout then kills the already paired session.
func (c *Client) CreateInstance(ctx context.Context, instanceID, name string) (string, error) {
	token := c.instanceToken(name)
	body := map[string]any{
		"instanceId": instanceID,
		"name":       name,
		"token":      token,
	}
	if err := c.do(ctx, http.MethodPost, "/instance/create", c.globalKey, body, nil); err != nil {
		return "", err
	}

	connect := map[string]any{
		"webhookUrl": c.webhookURL,
		"subscribe":  subscriptions,
		"immediate":  true,
	}
	if err := c.do(ctx, http.MethodPost, "/instance/connect", token, connect, nil); err != nil {
		return "", fmt.Errorf("conectando la instancia: %w", err)
	}
	return "", nil
}

// ConfigureWebhook reapplies subscriptions to an already linked line. Evolution
// updates the running client's settings in place when /instance/connect is
// called again, so this does not require another QR or start a second session.
func (c *Client) ConfigureWebhook(ctx context.Context, instance string) error {
	connect := map[string]any{
		"webhookUrl": c.webhookURL,
		"subscribe":  subscriptions,
		"immediate":  true,
	}
	return c.do(ctx, http.MethodPost, "/instance/connect", c.instanceToken(instance), connect, nil)
}

func (c *Client) QRCode(ctx context.Context, instance string) (string, error) {
	var out struct {
		Data struct {
			QRCode string `json:"qrcode"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/instance/qr", c.instanceToken(instance), nil, &out); err != nil {
		return "", err
	}
	if out.Data.QRCode == "" {
		return "", fmt.Errorf("Evolution Go no devolvió un código QR")
	}
	return out.Data.QRCode, nil
}

func (c *Client) Logout(ctx context.Context, instance string) error {
	return c.do(ctx, http.MethodDelete, "/instance/logout", c.instanceToken(instance), nil, nil)
}

func (c *Client) DeleteInstance(ctx context.Context, instance string) error {
	var out struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/instance/all", c.globalKey, nil, &out); err != nil {
		return err
	}
	for _, candidate := range out.Data {
		if candidate.Name == instance {
			path := "/instance/delete/" + url.PathEscape(candidate.ID)
			return c.do(ctx, http.MethodDelete, path, c.globalKey, nil, nil)
		}
	}
	return nil
}

func (c *Client) ConnectionState(ctx context.Context, instance string) (string, error) {
	var out struct {
		Data struct {
			Connected      bool `json:"Connected"`
			LoggedIn       bool `json:"LoggedIn"`
			ConnectedLower bool `json:"connected"`
			LoggedInLower  bool `json:"loggedIn"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/instance/status", c.instanceToken(instance), nil, &out); err != nil {
		return "", err
	}
	if out.Data.LoggedIn || out.Data.LoggedInLower {
		return "open", nil
	}
	if out.Data.Connected || out.Data.ConnectedLower {
		return "connecting", nil
	}
	return "close", nil
}

// Contacts returns the address book synchronized by the linked WhatsApp line.
// FullName is the name chosen by the owner of that line; PushName is the public
// profile name chosen by the remote person. They must remain separate.
func (c *Client) Contacts(ctx context.Context, instance string) ([]domain.ContactIdentity, error) {
	var out struct {
		Data []struct {
			JID          string `json:"Jid"`
			FirstName    string `json:"FirstName"`
			FullName     string `json:"FullName"`
			PushName     string `json:"PushName"`
			BusinessName string `json:"BusinessName"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/user/contacts", c.instanceToken(instance), nil, &out); err != nil {
		return nil, err
	}
	contacts := make([]domain.ContactIdentity, 0, len(out.Data))
	for _, item := range out.Data {
		if item.JID == "" || strings.HasSuffix(item.JID, "@g.us") {
			continue
		}
		contacts = append(contacts, domain.ContactIdentity{
			PhoneJID: item.JID, FirstName: item.FirstName, FullName: item.FullName,
			PushName: item.PushName, BusinessName: item.BusinessName,
		})
	}
	return contacts, nil
}

// AvatarURL asks Evolution Go for the temporary WhatsApp-hosted profile image
// visible to a specific linked line.
func (c *Client) AvatarURL(ctx context.Context, instance, jid string) (string, error) {
	body := struct {
		Number  string `json:"number"`
		Preview bool   `json:"preview"`
	}{Number: jid, Preview: true}
	var out struct {
		Data struct {
			URL string `json:"URL"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/user/avatar", c.instanceToken(instance), body, &out); err != nil {
		return "", err
	}
	if out.Data.URL == "" {
		return "", fmt.Errorf("Evolution Go no devolvió una URL de avatar")
	}
	return out.Data.URL, nil
}

// Evolution Go needs the original protobuf message, not just its WhatsApp ID,
// to download media. Inbound media is therefore transported through the
// webhook's mediaUrl (MinIO) and copied by the media pipeline instead.
func (c *Client) DownloadMedia(_ context.Context, _, waMessageID string) ([]byte, string, error) {
	return nil, "", fmt.Errorf("media %s no disponible por ID en Evolution Go", waMessageID)
}

func (c *Client) do(ctx context.Context, method, path, apiKey string, in, out any) error {
	var reader io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("serializando petición: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", apiKey)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%s %s devolvió %d: %s", method, path, resp.StatusCode, detail)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decodificando respuesta de %s: %w", path, err)
	}
	return nil
}
