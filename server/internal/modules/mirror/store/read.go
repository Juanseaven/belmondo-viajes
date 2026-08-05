package store

import (
	"context"
	"time"
)

// Este archivo define los puertos de LECTURA del espejo. Están separados de
// store.go —que es la ingesta— porque tienen formas y consumidores distintos: la
// ingesta escribe entidades del dominio, esto devuelve vistas armadas para una
// pantalla concreta.

// Line es una línea de WhatsApp tal como la ve el filtro de la bandeja.
type Line struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`  // evolution_name; la clave hacia Evolution
	Label       string     `json:"label"` // owner_label: el nombre que se muestra
	DisplayName string     `json:"displayName,omitempty"`
	PhoneNumber string     `json:"phoneNumber,omitempty"`
	Status      string     `json:"status"` // created | connecting | open | close
	IsActive    bool       `json:"isActive"`
	LastSeenAt  *time.Time `json:"lastSeenAt,omitempty"`

	// Suma de unread_count de sus conversaciones. Permite ver qué línea tiene
	// movimiento sin ir entrando una por una.
	Unread int `json:"unread"`
}

// InboxItem es una fila de la bandeja: todo lo que se pinta sin abrir el hilo.
//
// Se resuelve en UNA consulta con los JOIN a contacts e instances. La tentación
// natural es listar conversaciones y luego pedir el contacto de cada una; eso es
// un N+1 que no se nota con datos de prueba y arruina la bandeja con datos
// reales.
type InboxItem struct {
	ID          string `json:"id"`
	InstanceID  string `json:"instanceId"`
	InstanceLbl string `json:"instanceLabel"`
	ContactID   string `json:"contactId"`

	// El nombre editado a mano gana al que publica el propio contacto; si no hay
	// ninguno, el número.
	ContactName string `json:"contactName"`
	PhoneNumber string `json:"phoneNumber,omitempty"`
	AvatarURL   string `json:"avatarUrl,omitempty"`

	IsGroup       bool       `json:"isGroup"`
	LastMessageAt *time.Time `json:"lastMessageAt,omitempty"`
	Preview       string     `json:"preview,omitempty"`
	LastType      string     `json:"lastType,omitempty"`
	Unread        int        `json:"unread"`
}

// Cursor devuelve la posición de esta fila en la bandeja. Debe coincidir con el
// ORDER BY de la consulta o la paginación se saltará filas.
func (i InboxItem) Cursor() Cursor {
	var at time.Time
	if i.LastMessageAt != nil {
		at = *i.LastMessageAt
	}
	return Cursor{At: at, ID: i.ID}
}

// ThreadMessage es un mensaje tal como se pinta en el hilo.
type ThreadMessage struct {
	ID        string    `json:"id"`
	Direction string    `json:"direction"`
	Origin    string    `json:"origin"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Body      string    `json:"body,omitempty"`
	Timestamp time.Time `json:"timestamp"`

	// Quién lo escribió desde la app. Vacío si salió del teléfono: ese mensaje no
	// tiene autor en el sistema.
	SentByID   string `json:"sentById,omitempty"`
	SentByName string `json:"sentByName,omitempty"`

	QuotedID  string     `json:"quotedId,omitempty"`
	EditedAt  *time.Time `json:"editedAt,omitempty"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// Metadatos del adjunto. El binario se pide aparte a /media/{id}, que devuelve
	// una URL firmada de vida corta.
	Media *ThreadMedia `json:"media,omitempty"`
}

type ThreadMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mimeType,omitempty"`
	FileName string `json:"fileName,omitempty"`
	Size     int64  `json:"sizeBytes,omitempty"`
	Duration int    `json:"durationSeconds,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`

	// pending | processing | stored | failed. El mensaje aparece en el panel antes
	// de que el archivo esté subido, así que el frontend necesita saber si puede
	// pedir la URL o tiene que mostrar un marcador de carga.
	Status string `json:"status"`
}

func (m ThreadMessage) Cursor() Cursor {
	return Cursor{At: m.Timestamp, ID: m.ID}
}

// SearchHit es un mensaje encontrado, con lo justo para pintarlo en una lista de
// resultados y poder saltar a su hilo.
type SearchHit struct {
	MessageID      string    `json:"messageId"`
	ConversationID string    `json:"conversationId"`
	InstanceID     string    `json:"instanceId"`
	InstanceLbl    string    `json:"instanceLabel"`
	ContactName    string    `json:"contactName"`
	Snippet        string    `json:"snippet"`
	Timestamp      time.Time `json:"timestamp"`
}

func (h SearchHit) Cursor() Cursor {
	return Cursor{At: h.Timestamp, ID: h.MessageID}
}

// InboxQuery son los filtros de la bandeja.
type InboxQuery struct {
	InstanceID string // vacío = todas las líneas
	Search     string // filtra por nombre o número de contacto
	Cursor     Cursor
	Limit      int
}

// SearchQuery son los filtros de la búsqueda dentro de los mensajes.
type SearchQuery struct {
	Text       string
	InstanceID string
	Cursor     Cursor
	Limit      int
}

// ThreadQuery pide una ventana de mensajes de una conversación.
//
// Dos modos excluyentes:
//   - Before (el normal): los anteriores al cursor, hacia atrás en el tiempo. Es
//     el scroll del hilo.
//   - Around: una ventana centrada en un mensaje concreto. Existe para que un
//     resultado de búsqueda pueda abrir el hilo EN ese mensaje; sin esto, hacer
//     clic en un resultado te deja al final del chat, que es inútil.
type ThreadQuery struct {
	ConversationID string
	Before         Cursor
	AroundID       string
	Limit          int
}

// ReadStore agrupa las consultas que alimentan el panel.
type ReadStore interface {
	Lines(ctx context.Context) ([]Line, error)

	Inbox(ctx context.Context, q InboxQuery) ([]InboxItem, error)

	// Conversation devuelve una fila de la bandeja por id. Devuelve ErrNotFound si
	// no existe.
	Conversation(ctx context.Context, id string) (InboxItem, error)

	// Thread devuelve mensajes del más reciente al más antiguo, salvo en modo
	// Around, donde devuelve la ventana en orden cronológico.
	Thread(ctx context.Context, q ThreadQuery) ([]ThreadMessage, error)

	// MarkRead pone el contador a cero. Es idempotente: llamarlo sobre una
	// conversación ya leída no es un error, porque el frontend lo dispara cada vez
	// que se abre el hilo.
	MarkRead(ctx context.Context, conversationID string) error

	// SearchMessages busca dentro del texto conversado.
	SearchMessages(ctx context.Context, q SearchQuery) ([]SearchHit, error)
}

// Maintenance son las tareas de fondo del módulo.
type Maintenance interface {
	// PurgeEvents borra los webhooks ya procesados anteriores a `before`, en lotes
	// de `batch`. Devuelve cuántas filas borró.
	//
	// Por lotes y no de un DELETE: webhook_events acumula millones de filas con
	// payloads JSONB, y un borrado único mantendría una transacción abierta el
	// tiempo suficiente para hinchar la tabla y bloquear la ingesta.
	PurgeEvents(ctx context.Context, before time.Time, batch int) (int64, error)
}
