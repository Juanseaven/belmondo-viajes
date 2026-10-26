package mirror

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

// Provisioner da de alta y desvincula líneas de WhatsApp.
type Provisioner interface {
	CreateInstance(ctx context.Context, instanceID, name string) (string, error)
	// ConfigureWebhook llama a /instance/connect: si el cliente no está
	// corriendo lo arranca y vuelve a emitir QRs.
	ConfigureWebhook(ctx context.Context, instance string) error
	Logout(ctx context.Context, instance string) error
	DeleteInstance(ctx context.Context, instance string) error
}

type QRCache interface {
	Set(ctx context.Context, instanceName, code string) error
	Get(ctx context.Context, instanceName string) (string, bool)
	Clear(ctx context.Context, instanceName string)
}

// createLine registra la línea y la crea en Evolution, EN ESE ORDEN.
//
// El orden es la decisión central de esta función. Si se creara primero en
// Evolution y el INSERT fallara, quedaría una instancia fantasma: vinculada,
// recibiendo mensajes, y el backend descartándolos todos porque no encuentra su
// fila. Un fallo invisible que solo se nota cuando alguien pregunta por qué no
// aparecen las conversaciones de un vendedor.
//
// Al revés, el fallo es visible y reparable: una línea que figura "sin conectar"
// en el panel, que se puede reintentar o borrar.
func (m *Module) createLine(ctx context.Context, ownerLabel, displayName string) (string, string, error) {
	ownerLabel = strings.TrimSpace(ownerLabel)
	if ownerLabel == "" {
		return "", "", fmt.Errorf("el nombre del dueño de la línea es obligatorio")
	}

	// Cerrar el modal antes de escanear y volver a abrirlo creaba una línea por
	// intento. La que nunca se vinculó se retoma con su misma instancia.
	pending, ok, err := m.lines.PendingByLabel(ctx, ownerLabel)
	if err != nil {
		return "", "", fmt.Errorf("buscando una línea pendiente: %w", err)
	}
	if ok {
		if err := m.provisioner.ConfigureWebhook(ctx, pending.Name); err != nil {
			return pending.ID, pending.Name, fmt.Errorf("reconectando la línea pendiente en Evolution: %w", err)
		}
		m.log.Info("línea pendiente retomada", "instance", pending.Name, "owner", ownerLabel)
		return pending.ID, pending.Name, nil
	}

	name := instanceName(ownerLabel)

	id, err := m.lines.Create(ctx, name, ownerLabel, strings.TrimSpace(displayName))
	if err != nil {
		return "", "", fmt.Errorf("registrando la línea: %w", err)
	}

	qr, err := m.provisioner.CreateInstance(ctx, id, name)
	if err != nil {
		// La fila queda en estado 'created'. No se borra: deja constancia del
		// intento y permite reintentar sin perder el nombre ya reservado.
		m.log.Error("evolution rechazó la creación de la instancia",
			"instance", name, "error", err)
		return id, name, fmt.Errorf("creando la instancia en Evolution: %w", err)
	}
	// Evolution entrega el QR por webhook. Algunos provisioners pueden devolver
	// uno sincrónicamente, pero un valor vacío no debe tapar el 404 temporal que
	// hace que el frontend siga esperando el primer evento.
	if qr != "" {
		if err := m.qr.Set(ctx, name, qr); err != nil {
			m.log.Warn("no se pudo guardar el primer QR", "instance", name, "error", err)
		}
	}
	if err := m.lines.UpdateConnection(ctx, name, "connecting", ""); err != nil {
		return id, name, fmt.Errorf("marcando la línea en conexión: %w", err)
	}

	// A partir de acá empiezan a llegar los qrcode.updated por webhook, que el
	// processor recoge y emite al panel.
	m.log.Info("línea dada de alta", "instance", name, "owner", ownerLabel)
	return id, name, nil
}

// disconnectLine desvincula el teléfono y borra la línea con todo su historial.
func (m *Module) disconnectLine(ctx context.Context, id string) error {
	name, err := m.lines.NameByID(ctx, id)
	if err != nil {
		return err
	}

	// Los errores de Evolution no cortan: puede que la instancia ya no exista de
	// su lado, y en ese caso desactivar la fila igual es lo correcto.
	if err := m.provisioner.Logout(ctx, name); err != nil {
		m.log.Warn("logout en Evolution falló", "instance", name, "error", err)
	}
	if err := m.provisioner.DeleteInstance(ctx, name); err != nil {
		m.log.Warn("delete en Evolution falló", "instance", name, "error", err)
	}
	m.qr.Clear(ctx, name)

	// Los archivos se borran antes que la fila. Si R2 falla, la línea queda
	// visible y se puede reintentar; los borrados por prefijo son idempotentes.
	for _, prefix := range []string{name + "/", "outgoing/" + name + "/"} {
		if err := m.k.Blobs.DeletePrefix(ctx, prefix); err != nil {
			return fmt.Errorf("borrando los archivos de la línea: %w", err)
		}
	}

	if err := m.lines.Delete(ctx, id); err != nil {
		return fmt.Errorf("borrando la línea: %w", err)
	}
	m.log.Info("línea desvinculada y borrada", "instance", name)
	return nil
}

// deacentuar mapea los caracteres acentuados a su letra base.
//
// Descartarlos sin más convertía "Ana María Pérez" en "ana-mara-prez": el nombre
// de la instancia aparece en logs y en la URL de cada envío, así que tiene que
// seguir siendo legible. Cubre lo que aparece en español y portugués, que es lo
// que va a escribir esta agencia.
var deacentuar = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'ñ': 'n', 'ç': 'c',
}

// instanceName deriva el nombre en Evolution a partir del dueño.
//
// Lleva sufijo aleatorio porque `evolution_name` es UNIQUE y dos personas pueden
// llamarse igual: sin él, dar de alta a la segunda Ana fallaría con un error de
// base que no dice nada. También evita reusar el nombre de una línea desvinculada,
// cuya fila sigue existiendo.
func instanceName(ownerLabel string) string {
	slug := strings.Map(func(r rune) rune {
		if base, ok := deacentuar[unicode.ToLower(r)]; ok {
			return base
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return unicode.ToLower(r)
		case r == ' ', r == '-', r == '_':
			return '-'
		default:
			return -1
		}
	}, ownerLabel)

	slug = strings.Trim(slug, "-")
	if len(slug) > 24 {
		slug = strings.Trim(slug[:24], "-")
	}
	if slug == "" {
		slug = "linea"
	}

	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		// Sin aleatoriedad el nombre podría chocar, y eso lo detecta el índice
		// único de todas formas. No vale la pena abortar el alta por esto.
		return slug
	}
	return slug + "-" + hex.EncodeToString(suffix[:])
}
