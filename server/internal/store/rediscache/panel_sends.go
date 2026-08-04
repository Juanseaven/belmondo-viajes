// Package rediscache implementa los puertos de store que necesitan estado
// efímero y compartido entre réplicas.
package rediscache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// PanelSends resuelve la pregunta central de la auditoría: este mensaje saliente,
// ¿lo escribió el vendedor desde su teléfono o el administrador desde el panel?
//
// WhatsApp marca ambos como fromMe:true y el payload no los distingue. La única
// señal disponible es que, al enviar desde el panel, Evolution nos devuelve el
// key.id antes de que el webhook llegue.
//
// Redis y no Postgres porque es un rendezvous de segundos entre dos procesos, con
// expiración automática y sin valor histórico. La constancia permanente vive en
// la tabla outbound_messages.
type PanelSends struct {
	rdb *redis.Client
}

func NewPanelSends(rdb *redis.Client) *PanelSends {
	return &PanelSends{rdb: rdb}
}

func key(waMessageID string) string {
	return "panel:sent:" + waMessageID
}

// Mark se llama JUSTO tras recibir el key.id de Evolution y antes de responder al
// administrador. El webhook tarda típicamente menos de un segundo, así que la
// ventana es holgada.
func (p *PanelSends) Mark(ctx context.Context, waMessageID, userID string, ttl time.Duration) error {
	return p.rdb.Set(ctx, key(waMessageID), userID, ttl).Err()
}

// Claim consume la marca de forma atómica: GETDEL evita que dos réplicas
// procesando el mismo evento la reclamen ambas.
//
// ok=false significa que el mensaje NO salió del panel, es decir: lo escribió el
// vendedor desde su móvil. Ese es el caso normal en un espejo pasivo.
func (p *PanelSends) Claim(ctx context.Context, waMessageID string) (string, bool) {
	userID, err := p.rdb.GetDel(ctx, key(waMessageID)).Result()
	if err != nil || userID == "" {
		// redis.Nil (no existía) o Redis caído. En ambos casos se clasifica como
		// 'device': se pierde precisión en la atribución, nunca el mensaje.
		return "", false
	}
	return userID, true
}
