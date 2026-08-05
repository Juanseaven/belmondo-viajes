package mirror

import (
	"context"
	"time"
)

// purgeInterval es cada cuánto se revisa. Un día: la tabla crece rápido en
// filas, no en urgencia.
const purgeInterval = 24 * time.Hour

// purgeBatch acota cada DELETE. Lotes chicos porque esto compite con la ingesta
// por conexiones del pool y la ingesta siempre tiene prioridad.
const purgeBatch = 1000

// runPurge borra periódicamente los webhooks ya resueltos que superan la
// retención configurada.
//
// Existe porque webhook_events es el problema operativo real del módulo: guarda
// el payload JSONB completo de cada evento, incluidos los ACK de entrega y
// lectura, que multiplican por varias veces el volumen de mensajes. Sin purga
// crece del orden de millones de filas al mes. No es que consulte lento: llena
// el disco.
//
// Es una goroutine del módulo y no un cron externo a propósito: así la retención
// viaja con el código que la necesita, en vez de ser configuración que vive fuera
// del repositorio y que nadie recuerda hasta que falla.
func (m *Module) runPurge(ctx context.Context) {
	defer close(m.upkeepDone)

	days := m.k.Cfg.EventRetentionDays
	if days <= 0 {
		m.log.Info("purga de webhook_events desactivada")
		return
	}

	// Una primera pasada al arrancar: si el proceso llevaba caído más de un día,
	// esperar otras 24 h para empezar a limpiar no tiene sentido.
	m.purgeOnce(ctx, days)

	ticker := time.NewTicker(purgeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.purgeOnce(ctx, days)
		}
	}
}

func (m *Module) purgeOnce(ctx context.Context, days int) {
	before := time.Now().AddDate(0, 0, -days)

	deleted, err := m.upkeep.PurgeEvents(ctx, before, purgeBatch)
	if err != nil {
		if ctx.Err() != nil {
			return // apagado en curso, no es un fallo
		}
		// No es fatal: se reintenta en la pasada siguiente. Lo que no puede es
		// pasar desapercibido, porque el síntoma final es el disco lleno.
		m.log.Error("purga de webhook_events fallida",
			"error", err, "deleted_before_failure", deleted)
		return
	}
	if deleted > 0 {
		m.log.Info("webhook_events purgados",
			"deleted", deleted, "older_than_days", days)
	}
}
