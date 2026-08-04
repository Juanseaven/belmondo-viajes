package httpapi

import (
	"net/http"
	"slices"

	"github.com/gorilla/websocket"
)

// wsHandler expone el hub de tiempo real. Vive en el kernel, no en un módulo:
// cualquier módulo puede emitir por él y el navegador mantiene una sola conexión
// para todos.
//
// La autenticación la aplica el mux de rutas con sesión. Como el navegador no
// permite mandar cabeceras en el handshake de un WebSocket, el token viaja por
// query param (ver auth.bearerToken).
func wsHandler(d Deps) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		// El Origin se valida contra la lista blanca: sin esto, cualquier web
		// podría abrir un socket con la sesión del usuario.
		CheckOrigin: func(r *http.Request) bool {
			return slices.Contains(d.AllowedOrigins, r.Header.Get("Origin"))
		},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			d.Log.Warn("fallo en el upgrade a websocket", "error", err)
			return
		}
		d.Hub.Serve(conn)
	}
}
