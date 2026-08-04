// Package kernel es la mitad compartida del sistema: lo que todo módulo necesita
// y que no conoce a ningún módulo en particular.
//
// La dependencia va siempre en un sentido: los módulos importan al kernel, jamás
// al revés. Si alguna vez hiciera falta lo contrario, la pieza en cuestión no era
// del kernel.
package kernel

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/blobs"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/config"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
)

// Kernel es la infraestructura que el arranque construye una vez y presta a cada
// módulo por constructor.
//
// Solo entra aquí lo que sirve a más de un módulo. Un cliente de OCR, por
// ejemplo, lo construye el módulo que lo usa dentro de su propio paquete: si el
// Kernel creciera con las dependencias de cada módulo, dejaría de ser el kernel.
type Kernel struct {
	Cfg   *config.Config
	DB    *pgxpool.Pool
	Redis *redis.Client
	Blobs blobs.Store
	Hub   *ws.Hub
	Log   *slog.Logger
}
