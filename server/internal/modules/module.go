// Package modules define el contrato que cumple toda funcionalidad del sistema.
//
// La regla que gobierna la arquitectura: el kernel no conoce a ningún módulo.
// Un módulo sí puede leer tablas públicas de otro y declarar claves foráneas
// hacia ellas, siempre en una sola dirección; nunca escribe tablas ajenas. Las
// dependencias se pasan a mano al construir el sistema y el grafo debe seguir
// siendo acíclico. Este paquete es la única pieza que el kernel y los módulos
// comparten, y a propósito no importa nada del proyecto.
//
// Agregar un módulo al sistema es escribir una implementación de Module y añadir
// una línea en cmd/api/main.go.
package modules

import (
	"context"
	"io/fs"
	"net/http"
)

type Module interface {
	// Name es el prefijo de rutas y la clave de migraciones. Estable: cambiarlo
	// rompe el historial de migraciones del módulo, porque schema_migrations lo
	// usa como parte de la clave primaria.
	Name() string

	// Migrations son los .sql embebidos del módulo, aplicados en orden
	// lexicográfico. Devolver un FS vacío es válido para un módulo sin tablas.
	Migrations() fs.FS

	// Mount registra los endpoints. Se llama una vez, durante el arranque.
	Mount(r Router)

	// Start arranca los procesos de fondo (pools, consumidores). NO bloquea: el
	// arranque del servidor HTTP viene después y no puede esperar.
	Start(ctx context.Context) error

	// Shutdown drena. Se invoca en orden inverso al de arranque, de modo que un
	// módulo que produce trabajo para otro deje de producirlo primero.
	Shutdown(ctx context.Context) error
}

// Router es la vista del enrutado que recibe un módulo. Deliberadamente estrecha:
// un módulo puede colgar handlers, y nada más. No alcanza al middleware global ni
// a las rutas de otros módulos.
type Router interface {
	// Authed monta bajo /api/<name>/ exigiendo un JWT válido. El patrón es
	// relativo al prefijo e incluye el método, igual que en net/http:
	//
	//	Authed("GET /conversations", h)  →  GET /api/mirror/conversations
	Authed(pattern string, h http.Handler)

	// Public monta en la raíz, sin autenticación.
	//
	// Existe por un caso concreto: los webhooks de terceros no pueden presentar
	// un JWT y se autentican como pueden (apikey, firma HMAC). Esa validación es
	// responsabilidad del módulo. Que la excepción tenga un nombre explícito
	// evita que se cuele una ruta sin auth por descuido.
	Public(pattern string, h http.Handler)
}
