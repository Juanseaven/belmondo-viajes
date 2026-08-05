package rediscache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// QRCodes guarda el último código QR de cada línea en vinculación.
//
// Redis y no Postgres porque el QR **rota cada ~30 segundos** y no vale nada una
// vez que la línea quedó vinculada: es estado efímero por definición, y
// persistirlo solo dejaría basura con una imagen dentro.
//
// Se guarda el último en vez de solo emitirlo por websocket para que abrir el
// modal muestre algo al instante, en lugar de esperar hasta medio minuto al
// siguiente.
type QRCodes struct {
	rdb *redis.Client
}

func NewQRCodes(rdb *redis.Client) *QRCodes { return &QRCodes{rdb: rdb} }

// qrTTL supera holgadamente el ciclo de rotación, para que el hueco entre dos
// códigos nunca deje el modal en blanco.
const qrTTL = 90 * time.Second

func qrKey(instanceName string) string { return "mirror:qr:" + instanceName }

// Set guarda el QR recién emitido. `code` es el data URI en base64 que manda
// Evolution.
func (q *QRCodes) Set(ctx context.Context, instanceName, code string) error {
	return q.rdb.Set(ctx, qrKey(instanceName), code, qrTTL).Err()
}

// Get devuelve el último QR conocido. ok=false si no hay ninguno vigente, que es
// el caso normal de una línea ya vinculada.
func (q *QRCodes) Get(ctx context.Context, instanceName string) (string, bool) {
	code, err := q.rdb.Get(ctx, qrKey(instanceName)).Result()
	if err != nil || code == "" {
		return "", false
	}
	return code, true
}

// Clear borra el QR al vincularse la línea: mostrarlo después sería ofrecer un
// código que ya no sirve.
func (q *QRCodes) Clear(ctx context.Context, instanceName string) {
	_ = q.rdb.Del(ctx, qrKey(instanceName)).Err()
}
