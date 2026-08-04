# viajes_crm — Arquitectura modular

Fecha: 2026-08-04
Estado: aprobado para planificación

## Problema

El panel espejo de WhatsApp resolvió un problema puntual de la agencia
([spec](2026-08-01-panel-espejo-whatsapp-design.md)). La agencia tiene más
problemas: el primero con diseño propio es
[passport-eligibility](../../../../passport-eligibility/docs/superpowers/specs/2026-07-20-passport-eligibility-mvp-design.md),
que hoy es solo un spec sin código.

La intención es construir **un panel de administración con varios módulos**, de a
poco, hasta cubrir la operación de la agencia. Este spec no construye ningún
módulo nuevo: deja el sistema en una forma donde **agregar uno sea barato**.

## Qué construimos

El andamiaje. Concretamente, cuatro cosas que hoy hacen caro agregar un módulo:

1. **Las migraciones son un archivo compartido.** `0001_init.sql` se aplica a mano
   con `psql`. Un módulo nuevo tendría que editar SQL ajeno.
2. **No hay autenticación.** `requireAuth` es un stub que deja pasar todo. Todo
   módulo la necesita y no puede resolverla cada uno por su cuenta.
3. **El espejo vive en la raíz de `internal/`**, mezclado con lo que es
   infraestructura compartida. "¿Dónde pongo mi código?" no tiene respuesta.
4. **El módulo Go se llama `whatsapp-mirror` y el frontend está vacío.** Es el
   momento más barato para renombrar y para que el frontend nazca con forma.

Y un cambio de producto que llegó con la decisión de expandir (§ siguiente).

## Cambio de producto: de panel de auditoría a panel de la agencia

El spec del panel espejo estableció que **los vendedores no son usuarios**: son
`instances`, siguen en WhatsApp nativo, y no saben que el panel existe. Esa
premisa se levanta.

`users` pasa a ser el **directorio del personal de la agencia**. Los vendedores
tienen cuenta, entran al panel, ven la bandeja compartida completa y pueden
enviar mensajes desde cualquier línea conectada. No hay filtro por dueño de
línea ni restricción de módulos: todos entran a todo.

### Lo que sobrevive del diseño original

La atribución de origen, que era el eje de la auditoría, **no solo sobrevive:
mejora de sentido**.

| | Antes | Ahora |
|---|---|---|
| `origin='panel'` | Lo escribió la dirección | Se escribió desde esta app, con autor en `sent_by_user_id` |
| `origin='device'` | Lo escribió el vendedor desde su móvil | Se escribió desde el teléfono, sin autor en el sistema |

El mecanismo de Redis (`PanelSendTracker`) que distingue ambos casos queda
intacto: sigue siendo la única forma de separar dos salientes que WhatsApp
entrega idénticos (`fromMe:true`). Deja de responder "¿fue la dirección o el
vendedor?" y pasa a responder "¿fue por la app o por el teléfono?", que es una
pregunta más honesta y sigue siendo la que hace útil la trazabilidad.

### Lo que se cae

El rol `auditor` (solo lectura). Si todos pueden enviar, no tiene sentido.

## Estructura: kernel y módulos

`internal/` se parte en dos mitades con una regla direccional:

> **El kernel no conoce a ningún módulo. Los módulos no se conocen entre sí.**

```
server/internal/
  kernel/
    config/       entorno; el kernel valida lo suyo, cada módulo lo suyo
    db/           pool de pgx + runner de migraciones
    auth/         usuarios, JWT, middleware, /api/me
    httpapi/      router raíz, CORS, logging
    ws/           hub de tiempo real compartido
    blobs/        R2 (hoy internal/media/r2.go)
    migrations/   users, extensiones, touch_updated_at()
  modules/
    module.go     el contrato
    mirror/       el espejo completo: webhook, worker, ingest, evolution,
                  media, store, domain de WhatsApp
      migrations/
```

`domain/` desaparece de la raíz: su contenido es vocabulario de WhatsApp
(`Origin`, `MessageType`, `MessageStatus`) y se muda a `modules/mirror/`. Si más
adelante aparece vocabulario genuinamente compartido entre dos módulos, se
promueve entonces, con un caso real que lo justifique.

El nombre `kernel` es deliberado frente a `shared/` o `common/`: esos nombres son
imanes de basura, porque cualquier cosa se puede argumentar como compartida.
`kernel` sugiere que agregarle algo es una decisión.

## El contrato `Module`

```go
// internal/modules/module.go

// Module es todo lo que el arranque necesita saber de una funcionalidad para
// montarla. Agregar un módulo al sistema es escribir una implementación de esta
// interfaz y añadir una línea en cmd/api/main.go.
type Module interface {
	// Name es el prefijo de rutas y la clave de migraciones. Estable: cambiarlo
	// rompe el historial de migraciones del módulo.
	Name() string

	// Migrations son los .sql embebidos del módulo, aplicados en orden
	// lexicográfico.
	Migrations() fs.FS

	// Mount registra los endpoints.
	Mount(r Router)

	// Start arranca los procesos de fondo (pools, consumidores). No bloquea.
	Start(ctx context.Context) error

	// Shutdown drena. Se invoca en orden inverso al de arranque.
	Shutdown(ctx context.Context) error
}

type Router interface {
	// Authed monta bajo /api/<name>/ exigiendo JWT válido. El patrón es
	// relativo al prefijo e incluye el método: Authed("GET /conversations", h)
	// queda en GET /api/mirror/conversations.
	Authed(pattern string, h http.Handler)

	// Public monta en la raíz sin autenticación. Para webhooks de terceros que
	// no pueden presentar un JWT; el módulo se autentica como pueda (apikey,
	// firma HMAC). Que la excepción tenga nombre evita que se cuele por
	// descuido.
	Public(pattern string, h http.Handler)
}
```

`Public` existe por un caso real: Evolution API no puede presentar un JWT y se
autentica con la apikey del payload.

### Inyección de infraestructura

Cada módulo recibe por constructor lo que el kernel provee:

```go
// internal/kernel/kernel.go
type Kernel struct {
	Cfg   *config.Config
	DB    *pgxpool.Pool
	Redis *redis.Client
	Blobs blobs.Store
	Hub   *ws.Hub
	Log   *slog.Logger
}
```

Un módulo que necesite algo propio (un cliente de OCR, por ejemplo) lo construye
él mismo dentro de su paquete. El `Kernel` no crece con dependencias de un solo
módulo.

### El arranque

```go
// cmd/api/main.go
mods := []modules.Module{
	mirror.New(k),
	// passport.New(k),
}
```

Esa lista es la definición operativa de "el sistema": el runner de migraciones,
el router y el ciclo de vida la recorren. El orden de apagado es el inverso del
de arranque, como ya hace el `shutdown` actual.

## Migraciones por módulo

```sql
CREATE TABLE schema_migrations (
    module     text        NOT NULL,
    version    text        NOT NULL,   -- nombre del archivo, ej. '0001_init.sql'
    applied_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (module, version)
);
```

Esa tabla la crea el propio runner con `CREATE TABLE IF NOT EXISTS` antes de
mirar nada, **no es una migración**: no puede serlo, porque es lo que decide qué
migraciones faltan.

Después el runner recorre el `embed.FS` de cada módulo en orden lexicográfico y
aplica lo que falte, **cada archivo dentro de su propia transacción**. Las
migraciones del kernel se aplican primero, bajo el nombre de módulo `kernel`,
porque `modules/mirror` tiene claves foráneas contra `users`.

La PK compuesta es lo que hace que dos módulos no se pisen: cada uno numera su
`0001` sin coordinarse con nadie.

Esto reemplaza el `docker compose exec postgres psql < 0001_init.sql` manual que
documenta el README.

### Reparto del esquema actual

Se asume que **no hay datos en producción**, así que `0001_init.sql` se parte en
dos en vez de escribir migraciones de reparación:

| Destino | Contenido |
|---|---|
| `kernel/migrations/0001_init.sql` | extensiones (`pgcrypto`, `citext`, `pg_trgm`), `touch_updated_at()`, `user_role`, `users` |
| `modules/mirror/migrations/0001_init.sql` | el resto: `instances`, `contacts`, `conversations`, `messages`, `media_assets`, `outbound_messages`, `webhook_events` y sus tipos e índices |

Las extensiones y `touch_updated_at()` van al kernel porque más de un módulo las
va a querer y son idempotentes (`CREATE EXTENSION IF NOT EXISTS`,
`CREATE OR REPLACE FUNCTION`).

## Autenticación y usuarios

Nuevo paquete `internal/kernel/auth`. Deja de ser un `TODO`.

- Contraseñas con bcrypt (el esquema ya lo declara así).
- JWT HS256 firmado con `JWT_SECRET`, vigencia `JWT_TTL`. Ambos ya existen en
  `config`.
- El middleware verifica el token y pone el `user_id` en el contexto; los
  handlers lo leen con `auth.UserID(ctx)`.

```
POST   /api/auth/login    → { token, user }
POST   /api/auth/logout
GET    /api/me            → { user, modules: ["mirror"] }
GET    /api/users         (solo admin)
POST   /api/users         (solo admin)
PATCH  /api/users/{id}    (solo admin)
DELETE /api/users/{id}    (solo admin)
```

`GET /api/me` devuelve la lista de módulos registrados en el arranque, y con eso
el frontend arma el menú. **Hoy devuelve todos, para todos.** El valor no está en
lo que hace hoy sino en dónde está: el día que un módulo deba restringirse, el
cambio es en esa función y en ningún otro lugar del sistema.

### Roles

```sql
user_role AS ENUM ('admin', 'member')
```

`admin` es el único que gestiona usuarios. `member` hace todo lo demás. El rol no
controla el acceso a módulos.

## Cambios en el esquema del espejo

Derivados del cambio de producto, no del refactor:

| Cambio | Razón |
|---|---|
| `instances.owner_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL` | El vendedor ahora tiene cuenta; la línea puede apuntar a ella. Nullable porque una línea puede no tener dueño con cuenta. |
| `instances.owner_label` se queda | Sigue siendo la etiqueta que se muestra, y cubre las líneas sin `owner_user_id`. |
| Comentario de `conversations.unread_count` | Pasa a ser un contador **global** de sin-revisar, no "pendientes del admin". |
| Comentario de `messages.sent_by_user_id` | "Quién lo escribió desde esta app", no "qué admin lo escribió". |

Las constraints `messages_panel_has_author` y `messages_origin_matches_direction`
**no cambian**: siguen siendo correctas bajo la nueva semántica.

El estado de lectura por usuario (que cada quien tenga sus propios no-leídos)
requeriría una tabla aparte. No se hace: con el equipo de una agencia el
contador global alcanza, y la tabla es fácil de agregar después sin tocar nada
de lo existente.

## Frontend

`web/` está vacío salvo el Dockerfile (Next.js con `output: "standalone"`). Nace
con la misma forma que el backend, para que la respuesta a "¿dónde pongo mi
código?" sea la misma en los dos lados:

```
web/src/
  kernel/
    auth/       login, guard de sesión, contexto de usuario
    api/        cliente HTTP (token, errores) + hook de websocket
    shell/      layout del panel: sidebar, header
  modules/
    registry.ts   la única lista
    mirror/
      module.ts   metadata: nombre, etiqueta, icono, ruta
      ...
  app/
    login/
    (panel)/
      layout.tsx  shell + guard de sesión
```

```ts
export const modules: ModuleDef[] = [
  mirror,
  // passport,
];
```

El sidebar **no** se arma del registry solo: se arma cruzando el registry con los
módulos que devuelve `GET /api/me`. El backend es la autoridad sobre qué existe;
el frontend solo sabe dibujarlo.

### Websockets

El sobre del evento que emite el `Hub` gana un campo `module`, y la suscripción
del cliente lo filtra igual que hoy filtra por `instance_id`. Sin eso, el día que
haya un segundo módulo emitiendo eventos, cada cliente recibe el tráfico de todos.
Es barato ahora y molesto de retrofitear.

## Renombrado

| | Antes | Después |
|---|---|---|
| Módulo Go | `github.com/juanseaven/whatsapp-mirror/server` | `github.com/juanseaven/viajes-crm/server` |
| Producto | "Panel espejo de WhatsApp" | "viajes_crm" |
| Carpeta del repo | `whatsapp-crm` | sin cambio |

La carpeta del repo no se toca: renombrarla mueve el directorio de trabajo y no
aporta nada al objetivo.

El README se reescribe: el sistema deja de ser un panel de auditoría de WhatsApp
y pasa a ser un panel modular cuyo primer módulo es el espejo. El spec del panel
espejo se conserva sin editar como registro histórico, con una nota al principio
que apunte a este documento para las premisas que quedaron revocadas.

## Testing

- **Runner de migraciones**, integración contra Postgres real: aplicar dos veces
  es idempotente; un módulo no ve ni reaplica las migraciones de otro; el kernel
  corre antes que los módulos.
- **Contrato `Module`**: un módulo falso en tests verifica que `Mount` y `Start`
  se invocan para cada módulo registrado y que `Shutdown` ocurre en orden inverso
  al arranque.
- **Auth**: tabla de casos de JWT (ausente, expirado, firma inválida, usuario
  desactivado). El 401 no debe filtrar si el usuario existe.
- **Los tests del espejo se conservan tal cual** (`worker/pool_test.go` y los que
  haya). Si la mudanza les cambia algo más que los `import`, el refactor dejó de
  ser mecánico y hay que revisarlo. Es el criterio de aceptación del refactor.

## Fuera de alcance

- **La API del módulo espejo**: listar bandeja, listar mensajes con paginación,
  enviar texto y media, alta de línea con QR. Va en el spec inmediatamente
  siguiente, y sirve como primera prueba de que agregar superficie a un módulo
  sobre el contrato es barato.
- **El módulo passport en sí.**
- Permisos por módulo o RBAC.
- Multi-tenant. Sigue siendo un despliegue por agencia, sin `tenant_id`.
- Estado de lectura por usuario.
- **Bus de eventos entre módulos.** La integración obvia a futuro es "llegó una
  foto por WhatsApp → evaluarla como pasaporte", pero diseñar ese contrato sin
  tener el segundo módulo construido es adivinar su forma. Cuando exista
  passport, el caso real dirá qué hace falta. Mientras tanto, un módulo que
  necesite hablar con otro lo hará por una interfaz explícita inyectada en su
  constructor.

## Suposiciones

1. **No hay datos en producción.** El esquema se reparte en dos migraciones
   iniciales en vez de migrarse. Si es falso, hay que sustituir el reparto por
   una migración de `ALTER` y sembrar `schema_migrations` con lo ya aplicado.
2. **El repositorio no está bajo git todavía** (verificado al escribir este
   spec). El refactor es grande y mecánico: conviene inicializarlo antes de
   empezar, para poder revertir.
