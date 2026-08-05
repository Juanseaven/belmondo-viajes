package mirror

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/auth"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

// maxAttachmentBytes acota el adjunto saliente. WhatsApp rechaza por encima de
// ~16 MB en la mayoría de los tipos; cortar antes evita subir a R2 algo que
// después va a fallar igual.
const maxAttachmentBytes = 16 << 20

// Códigos propios del módulo, además de los del kernel.
const (
	codeLineOffline = "LINE_OFFLINE"
	codeSendFailed  = "SEND_FAILED"
)

// sendMessage envía por la línea de la conversación.
//
// Acepta JSON para texto y multipart cuando hay adjunto. El Content-Type
// discrimina: el cliente hace lo natural en cada caso y no tiene que envolver un
// texto en un formulario.
func (m *Module) sendMessage(w http.ResponseWriter, r *http.Request) {
	req := SendRequest{
		ConversationID: r.PathValue("id"),
		UserID:         auth.UserID(r.Context()),
	}

	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		err = m.readMultipartSend(r, &req)
	} else {
		err = m.readJSONSend(r, &req)
	}
	if err != nil {
		httpx.BadRequest(w, err.Error())
		return
	}

	if req.IdempotencyKey == "" {
		// Sin clave no hay protección contra el doble envío, y es el cliente quien
		// puede generarla de forma estable entre reintentos.
		httpx.BadRequest(w, "falta idempotencyKey")
		return
	}
	if req.Body == "" && req.Attachment == nil {
		httpx.BadRequest(w, "el mensaje está vacío")
		return
	}

	record, err := m.send.Send(r.Context(), req)
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpx.NotFound(w, "conversación no encontrada")
		return
	case errors.Is(err, ErrLineOffline):
		// 409 y no 500: el sistema funciona, la línea es la que está caída, y hasta
		// que alguien reescanee el QR reintentar no sirve de nada.
		httpx.Error(w, http.StatusConflict, codeLineOffline,
			"la línea no está conectada: hay que volver a vincularla")
		return
	case errors.Is(err, ErrSendRejected):
		m.log.Error("envío rechazado", "conversation", req.ConversationID, "error", err)
		httpx.Error(w, http.StatusBadGateway, codeSendFailed,
			"WhatsApp rechazó el mensaje; probá de nuevo")
		return
	case err != nil:
		httpx.Internal(w, m.log, "enviando el mensaje", err)
		return
	}

	// 202 y no 201: el mensaje está aceptado y entregado a WhatsApp, pero la fila
	// definitiva en `messages` la crea el webhook que vuelve un segundo después.
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"id":             record.ID,
		"idempotencyKey": record.IdempotencyKey,
		"waMessageId":    record.WAMessageID,
		"status":         record.Status,
	})
}

func (m *Module) readJSONSend(r *http.Request, req *SendRequest) error {
	var in struct {
		Body           string `json:"body"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return errors.New("cuerpo inválido")
	}
	req.Body = strings.TrimSpace(in.Body)
	req.IdempotencyKey = in.IdempotencyKey
	return nil
}

func (m *Module) readMultipartSend(r *http.Request, req *SendRequest) error {
	// El límite del reader corta ANTES de bufferizar: sin él, un cuerpo enorme se
	// copia entero a memoria y recién ahí se rechaza.
	r.Body = http.MaxBytesReader(nil, r.Body, maxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return errors.New("el archivo es demasiado grande o el formulario es inválido")
	}

	req.Body = strings.TrimSpace(r.FormValue("body"))
	req.IdempotencyKey = r.FormValue("idempotencyKey")

	file, header, err := r.FormFile("file")
	if err != nil {
		return errors.New("falta el archivo")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxAttachmentBytes+1))
	if err != nil {
		return errors.New("no se pudo leer el archivo")
	}
	if len(data) > maxAttachmentBytes {
		return errors.New("el archivo supera los 16 MB")
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	req.Attachment = &Attachment{
		FileName: header.Filename,
		MimeType: mimeType,
		Data:     data,
	}
	return nil
}

// createLineHandler da de alta una línea. Solo admin: conectar un número expone
// todas sus conversaciones al equipo entero.
func (m *Module) createLineHandler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OwnerLabel  string `json:"ownerLabel"`
		DisplayName string `json:"displayName"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.BadRequest(w, "cuerpo inválido")
		return
	}

	id, name, err := m.createLine(r.Context(), in.OwnerLabel, in.DisplayName)
	if err != nil && id == "" {
		httpx.BadRequest(w, err.Error())
		return
	}
	if err != nil {
		// La fila existe pero Evolution falló. Se devuelve 502 CON el id: el panel
		// muestra la línea como pendiente y permite reintentar o borrarla, en vez
		// de dejar un alta a medias invisible.
		httpx.JSON(w, http.StatusBadGateway, map[string]any{
			"id":       id,
			"instance": name,
			"error": map[string]string{
				"code":    codeSendFailed,
				"message": "la línea quedó registrada pero Evolution no pudo crearla",
			},
		})
		return
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":       id,
		"instance": name,
		"status":   "created",
	})
}

func (m *Module) deleteLineHandler(w http.ResponseWriter, r *http.Request) {
	err := m.disconnectLine(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.NotFound(w, "línea no encontrada")
		return
	}
	if err != nil {
		httpx.Internal(w, m.log, "desvinculando la línea", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// lineQR devuelve el último código de vinculación conocido.
//
// El QR rota cada ~30 s y el panel también los recibe en vivo por websocket; este
// endpoint existe para que abrir el modal muestre algo al instante en vez de
// esperar al siguiente.
func (m *Module) lineQR(w http.ResponseWriter, r *http.Request) {
	name, err := m.lines.NameByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.NotFound(w, "línea no encontrada")
		return
	}
	if err != nil {
		httpx.Internal(w, m.log, "leyendo la línea", err)
		return
	}

	code, ok := m.qr.Get(r.Context(), name)
	if !ok {
		// 404 y no error: una línea ya vinculada simplemente no tiene QR vigente.
		httpx.NotFound(w, "no hay un código vigente")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"instance": name, "qr": code})
}
