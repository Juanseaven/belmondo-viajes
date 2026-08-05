// Package evolution es el cliente saliente hacia Evolution API. Junto con
// webhook, es la única parte del sistema que conoce ese proveedor.
package evolution

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http: &http.Client{
			// Generoso porque getBase64FromMediaMessage descarga y descifra
			// archivos que pueden ser vídeos de varios MB.
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				// Con N líneas conectadas hay N flujos de media concurrentes; el
				// valor por defecto (2 por host) los serializaría.
				MaxIdleConnsPerHost: 32,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// SendTextResult contiene el key.id que devuelve WhatsApp. Es el puente hacia el
// webhook: PanelSendTracker lo marca para que, cuando el evento vuelva con
// fromMe:true, se clasifique como origin='panel' y no como 'device'.
type SendTextResult struct {
	Key struct {
		ID        string `json:"id"`
		RemoteJID string `json:"remoteJid"`
		FromMe    bool   `json:"fromMe"`
	} `json:"key"`
	Status           string `json:"status"`
	MessageTimestamp any    `json:"messageTimestamp"`
}

// SendText envía un mensaje de texto por una línea concreta.
func (c *Client) SendText(ctx context.Context, instance, number, text string) (*SendTextResult, error) {
	body := map[string]any{
		"number": number,
		"text":   text,
		// Retardo humano corto: WhatsApp penaliza el envío instantáneo desde API
		// y esta cuenta es el número real de trabajo de un vendedor.
		"delay":       800,
		"linkPreview": true,
	}
	var out SendTextResult
	if err := c.do(ctx, http.MethodPost, "/message/sendText/"+url.PathEscape(instance), body, &out); err != nil {
		return nil, err
	}
	if out.Key.ID == "" {
		return nil, fmt.Errorf("evolution aceptó el envío pero no devolvió key.id")
	}
	return &out, nil
}

// SendMedia envía un archivo ya alojado en una URL accesible por Evolution
// (típicamente una URL firmada de R2).
func (c *Client) SendMedia(ctx context.Context, instance, number, mediaURL, mediatype, caption, fileName string) (*SendTextResult, error) {
	body := map[string]any{
		"number":    number,
		"mediatype": mediatype, // image | video | document | audio
		"media":     mediaURL,
		"caption":   caption,
		"fileName":  fileName,
	}
	var out SendTextResult
	if err := c.do(ctx, http.MethodPost, "/message/sendMedia/"+url.PathEscape(instance), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadMedia obtiene el binario descifrado de un adjunto.
//
// El webhook solo trae una URL de mmg.whatsapp.net cifrada con mediaKey: no es
// descargable directamente. Evolution mantiene las claves de sesión y es el único
// que puede descifrarla, por eso el rodeo.
func (c *Client) DownloadMedia(ctx context.Context, instance, waMessageID string) ([]byte, string, error) {
	body := map[string]any{
		"message":      map[string]any{"key": map[string]any{"id": waMessageID}},
		"convertToMp4": false,
	}
	var out struct {
		Base64   string `json:"base64"`
		Mimetype string `json:"mimetype"`
		FileName string `json:"fileName"`
	}
	path := "/chat/getBase64FromMediaMessage/" + url.PathEscape(instance)
	if err := c.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return nil, "", err
	}
	if out.Base64 == "" {
		return nil, "", fmt.Errorf("respuesta sin base64 para el mensaje %s", waMessageID)
	}
	raw, err := base64.StdEncoding.DecodeString(out.Base64)
	if err != nil {
		return nil, "", fmt.Errorf("base64 inválido: %w", err)
	}
	return raw, out.Mimetype, nil
}

// CreateInstance da de alta una línea nueva. El QR se recoge después por el
// evento qrcode.updated del webhook.
func (c *Client) CreateInstance(ctx context.Context, name string) error {
	body := map[string]any{
		"instanceName": name,
		"qrcode":       true,
		"integration":  "WHATSAPP-BAILEYS",
	}
	return c.do(ctx, http.MethodPost, "/instance/create", body, nil)
}

// Logout desvincula el teléfono sin borrar la instancia en Evolution.
func (c *Client) Logout(ctx context.Context, instance string) error {
	return c.do(ctx, http.MethodDelete, "/instance/logout/"+url.PathEscape(instance), nil, nil)
}

// DeleteInstance elimina la instancia en Evolution.
//
// La fila de `instances` NO se borra al llamar a esto: las conversaciones y
// mensajes cuelgan de ella con ON DELETE CASCADE, así que borrarla se llevaría
// puesto el historial, que es justamente lo que el panel existe para conservar.
// El módulo la marca inactiva.
func (c *Client) DeleteInstance(ctx context.Context, instance string) error {
	return c.do(ctx, http.MethodDelete, "/instance/delete/"+url.PathEscape(instance), nil, nil)
}

// ConnectionState consulta si una línea sigue vinculada.
func (c *Client) ConnectionState(ctx context.Context, instance string) (string, error) {
	var out struct {
		Instance struct {
			State string `json:"state"`
		} `json:"instance"`
	}
	path := "/instance/connectionState/" + url.PathEscape(instance)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	return out.Instance.State, nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
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
	req.Header.Set("apikey", c.apiKey)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		// Se limita la lectura del cuerpo de error: un 502 de un proxy puede
		// devolver una página HTML entera.
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
