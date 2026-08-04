// Package httpx son los ayudantes de respuesta HTTP que comparten el kernel y
// los módulos.
//
// Existe para que la forma de un error sea la misma en todo el sistema: el
// frontend tiene un solo manejador de errores y no uno por módulo.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Códigos de error del kernel. Cada módulo define los suyos.
const (
	CodeUnauthenticated = "UNAUTHENTICATED"
	CodeForbidden       = "FORBIDDEN"
	CodeNotFound        = "NOT_FOUND"
	CodeInvalidInput    = "INVALID_INPUT"
	CodeConflict        = "CONFLICT"
	CodeInternal        = "INTERNAL"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorBody{errorDetail{Code: code, Message: message}})
}

func Unauthorized(w http.ResponseWriter, msg string) {
	Error(w, http.StatusUnauthorized, CodeUnauthenticated, msg)
}

func Forbidden(w http.ResponseWriter, msg string) {
	Error(w, http.StatusForbidden, CodeForbidden, msg)
}

func NotFound(w http.ResponseWriter, msg string) {
	Error(w, http.StatusNotFound, CodeNotFound, msg)
}

func BadRequest(w http.ResponseWriter, msg string) {
	Error(w, http.StatusBadRequest, CodeInvalidInput, msg)
}

// Internal registra la causa real y devuelve un mensaje genérico: los detalles de
// un fallo interno no son asunto del cliente.
func Internal(w http.ResponseWriter, log *slog.Logger, msg string, err error) {
	log.Error(msg, "error", err)
	Error(w, http.StatusInternalServerError, CodeInternal, "error interno")
}

// DecodeJSON lee el cuerpo rechazando campos desconocidos: un typo en el nombre
// de un campo debe fallar, no ignorarse en silencio.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
