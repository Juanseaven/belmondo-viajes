package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestLoggingMiddlewarePreservaElUpgradeAWebSocket(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	upgrader := websocket.Upgrader{}

	server := httptest.NewServer(withLogging(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	})))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("upgrade atravesando withLogging: status=%d error=%v", status, err)
	}
	_ = conn.Close()
}

func TestCORSAutorizaPUTParaLosUpsertsDeModulos(t *testing.T) {
	h := withCORS([]string{"https://panel.example"}, http.NotFoundHandler())
	req := httptest.NewRequest(http.MethodOptions, "/api/operaciones/viajes/1/pago", nil)
	req.Header.Set("Origin", "https://panel.example")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PUT") {
		t.Fatalf("métodos CORS = %q, falta PUT", got)
	}
}
