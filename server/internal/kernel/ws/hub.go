// Package ws hace fan-out de los eventos ya normalizados a los navegadores.
//
// Es infraestructura del kernel: cualquier módulo emite por el mismo hub y el
// navegador mantiene una sola conexión para todos.
//
// Regla que gobierna el diseño: un cliente lento NUNCA puede frenar el pipeline
// que emite. Si un navegador no consume, se le desconecta; al reconectar se
// resincroniza por REST.
package ws

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Event es lo que viaja al frontend.
type Event struct {
	// Module es quién emite ("mirror"). Sin este campo, el día que haya un
	// segundo módulo emitiendo, cada pestaña recibe el tráfico de todos y tiene
	// que descartarlo en el navegador.
	Module string `json:"module"`

	Type       string          `json:"type"` // message.new, message.status, instance.status
	InstanceID string          `json:"instanceId,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	At         time.Time       `json:"at"`
}

// Client es una pestaña del navegador conectada.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte

	// Filtros server-side. Filtrar aquí y no en el navegador evita mandar por la
	// red eventos que el usuario no está mirando.
	//
	// Un mapa vacío significa "sin filtro", no "nada": una pestaña recién
	// conectada recibe todo hasta que se suscribe.
	modules   map[string]bool
	instances map[string]bool
	mu        sync.RWMutex
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingInterval   = (pongWait * 9) / 10
	clientBufSize  = 256
	maxMessageSize = 4096
)

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	log     *slog.Logger
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{clients: make(map[*Client]struct{}), log: log}
}

func (h *Hub) add(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	n := len(h.clients)
	h.mu.Unlock()
	h.log.Debug("cliente ws conectado", "total", n)
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	n := len(h.clients)
	h.mu.Unlock()
	h.log.Debug("cliente ws desconectado", "total", n)
}

// Broadcast lo llaman los workers. Debe ser no bloqueante en todos los casos:
// está en la ruta crítica de ingesta de mensajes.
func (h *Hub) Broadcast(ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		h.log.Error("no se pudo serializar el evento ws", "error", err)
		return
	}

	// RLock: varios workers pueden emitir a la vez sin serializarse entre ellos.
	h.mu.RLock()
	targets := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		if c.wants(ev.Module, ev.InstanceID) {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- data:
		default:
			// Buffer lleno: el navegador no consume. Se le corta en lugar de
			// bloquear al worker. Cerrar la conexión hace que readPump falle y
			// llame a hub.remove, que es quien limpia el registro.
			h.log.Warn("cliente ws lento, desconectando")
			c.closeSlow()
		}
	}
}

func (c *Client) wants(module, instanceID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.modules) > 0 && !c.modules[module] {
		return false
	}
	// El filtro por línea solo aplica a eventos que llevan línea: los de ámbito
	// de módulo (una instancia que se cae, sin id todavía) llegan siempre.
	if len(c.instances) == 0 || instanceID == "" {
		return true
	}
	return c.instances[instanceID]
}

func (c *Client) closeSlow() {
	_ = c.conn.SetWriteDeadline(time.Now().Add(time.Second))
	_ = c.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "cliente lento"))
	_ = c.conn.Close()
}

// Serve arranca las dos goroutines de la conexión. El upgrade y la
// autenticación los hace httpapi antes de llegar aquí.
func (h *Hub) Serve(conn *websocket.Conn) {
	c := &Client{
		hub:       h,
		conn:      conn,
		send:      make(chan []byte, clientBufSize),
		modules:   map[string]bool{},
		instances: map[string]bool{},
	}
	h.add(c)
	go c.writePump()
	go c.readPump()
}

// readPump existe sobre todo por los pings: sin leer, el keepalive no funciona y
// las conexiones muertas se acumulan tras un proxy. También recibe el filtro de
// líneas que el frontend actualiza al cambiar de pestaña.
func (c *Client) readPump() {
	defer func() {
		c.hub.remove(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var cmd struct {
			Action    string   `json:"action"`
			Modules   []string `json:"modules"`
			Instances []string `json:"instances"`
		}
		if err := json.Unmarshal(raw, &cmd); err != nil {
			continue
		}
		if cmd.Action == "subscribe" {
			c.mu.Lock()
			c.modules = toSet(cmd.Modules)
			c.instances = toSet(cmd.Instances)
			c.mu.Unlock()
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func toSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

// Connected expone el número de clientes para el endpoint de salud.
func (h *Hub) Connected() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
