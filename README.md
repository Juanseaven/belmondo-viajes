# viajes_crm

Panel de administración de una agencia de viajes, organizado en módulos. Se
construye de a poco: cada herramienta que la agencia necesita entra como un
módulo nuevo, sin tocar los que ya están.

Hoy hay uno: **mirror**, el espejo de WhatsApp. Refleja en tiempo real todas las
líneas de la agencia en una bandeja compartida, filtrable por número, con envío
desde el panel.

- Arquitectura modular: [docs/superpowers/specs/2026-08-04-arquitectura-modular-design.md](docs/superpowers/specs/2026-08-04-arquitectura-modular-design.md)
- Módulo mirror: [docs/superpowers/specs/2026-08-01-panel-espejo-whatsapp-design.md](docs/superpowers/specs/2026-08-01-panel-espejo-whatsapp-design.md)

## Estado

| | |
|---|---|
| Kernel | Auth (login, JWT, usuarios), migraciones por módulo, websockets, R2 |
| mirror — ingesta | Funciona: webhook → worker particionado → Postgres → R2 → websocket |
| mirror — lectura | Funciona: bandeja unificada o por línea, hilo paginado, búsqueda dentro de los mensajes, marcar revisado |
| mirror — escritura | **No construida.** Enviar mensajes y dar de alta una línea con QR desde el panel son el alcance del próximo spec |
| Frontend | Login, shell con menú por módulos, bandeja del espejo completa en modo lectura |

Dar de alta una línea todavía se hace a mano (ver más abajo).

### API del módulo mirror

Todo bajo `/api/mirror`, con sesión. Los listados paginan por keyset: la
respuesta trae `nextCursor` y se devuelve tal cual en la petición siguiente.

```
GET  /lines                          líneas + sin-revisar de cada una
GET  /conversations                  ?instance= ?q= ?cursor= ?limit=
GET  /conversations/{id}
GET  /conversations/{id}/messages    ?before= | ?around=<messageId>
POST /conversations/{id}/read
GET  /search                         ?q= ?instance=   (mínimo 3 caracteres)
GET  /media/{id}                     URL firmada de vida corta
```

`around` centra la ventana en un mensaje concreto: es lo que permite que un
resultado de búsqueda abra el hilo **en** ese mensaje y no al final del chat.

## Estructura

```
server/
  cmd/api/                 Entrypoint: construye el kernel, declara los módulos
  internal/
    kernel/                La mitad compartida. No conoce ningún módulo
      config/              Entorno; falla al arrancar, no en la primera petición
      db/                  Pool de pgx + runner de migraciones
      auth/                Usuarios, JWT, middleware de sesión
      httpapi/             Router raíz, montaje de módulos, CORS, logging
      httpx/               Forma común de respuestas y errores
      ws/                  Hub de tiempo real, compartido por todos los módulos
      blobs/               Almacenamiento de objetos (R2)
      migrations/          users, extensiones, touch_updated_at()
    modules/
      module.go            El contrato: Name, Migrations, Mount, Start, Shutdown
      registry.go          Ciclo de vida: arranque y apagado en orden inverso
      mirror/              El espejo: webhook, worker, ingest, media, store
        migrations/
web/
  src/
    kernel/                auth, cliente HTTP, websocket, shell del panel
    modules/
      registry.ts          La única lista del frontend
      mirror/
    app/                   Rutas de Next.js
```

**La regla que gobierna todo:** el kernel no conoce a ningún módulo, y los
módulos no se conocen entre sí. Si algo del kernel necesitara importar un módulo,
esa pieza no era del kernel.

## Agregar un módulo

Backend — implementar `modules.Module` y sumar una línea:

```go
// cmd/api/main.go
registry := modules.NewRegistry(log,
    mirror.New(k),
    passport.New(k),   // ← esto
)
```

El runner aplica sus migraciones bajo su propio nombre (cada módulo numera su
`0001` sin coordinarse con nadie), monta sus rutas bajo `/api/<name>/`, arranca
sus procesos y los drena al apagar.

Frontend — crear la carpeta y sumar otra línea:

```ts
// web/src/modules/registry.ts
export const modules: ModuleDef[] = [mirror, passport];
```

El menú se arma cruzando esa lista con los módulos que devuelve `GET /api/me`. El
backend es la autoridad sobre qué existe; el frontend solo sabe dibujarlo.

## Arranque

```bash
cp .env.example .env
# rellenar: POSTGRES_PASSWORD, EVOLUTION_API_KEY, JWT_SECRET, credenciales de R2,
# y SEED_ADMIN_EMAIL / SEED_ADMIN_PASSWORD para el primer login
#   openssl rand -hex 32

docker compose up -d postgres redis
docker compose up -d evolution-api
docker compose up -d backend web
```

**El esquema se aplica solo**, al arrancar el backend. No hay que correr `psql` a
mano: el runner compara el `embed.FS` de cada módulo contra `schema_migrations` y
aplica lo que falte, cada archivo en su propia transacción.

El primer login usa `SEED_ADMIN_EMAIL` / `SEED_ADMIN_PASSWORD`, que solo se
tienen en cuenta si la tabla `users` está vacía. Después de entrar, cambiá la
contraseña desde el panel y sacá esas dos líneas del `.env`.

## Dar de alta la línea de un vendedor

Evolution solo escucha en `127.0.0.1`, así que desde el servidor:

```bash
curl -X POST http://127.0.0.1:8080/instance/create \
  -H "apikey: $EVOLUTION_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"instanceName":"ana-ventas","qrcode":true,"integration":"WHATSAPP-BAILEYS"}'
```

Luego insertar la fila correspondiente en `instances` con el `owner_label` del
vendedor. **Sin esa fila el backend descarta los mensajes de esa línea**: es
deliberado, un error de aprovisionamiento debe verse en los logs y no crear
instancias fantasma en la base.

Que esto siga siendo manual es justamente lo que resuelve el próximo spec.

## Notas de operación

- La API de Evolution nunca debe exponerse a internet. Su `apikey` da control
  total sobre todas las líneas conectadas.
- El bucket de R2 es privado. El frontend pide `GET /api/mirror/media/{id}` y
  recibe una URL firmada de vida corta. Por ahí viajan pasaportes.
- `webhook_events` se purga solo: el módulo espejo borra a diario, por lotes, los
  eventos ya procesados que superan `EVENT_RETENTION_DAYS` (30 por defecto). Es la
  tabla que más crece con diferencia, porque guarda el payload completo de cada
  evento incluidos los ACK de entrega y lectura. Con la purga desactivada (`0`) el
  problema no es que consulte lento: es que llena el disco.
- La búsqueda dentro de los mensajes tiene un `statement_timeout` de 3 s propio.
  Un término muy común devuelve `SEARCH_TOO_BROAD` en vez de monopolizar una
  conexión. Es el único endpoint que no escala solo con el volumen de mensajes.
- Si Redis se pierde, los mensajes enviados desde el panel durante esa ventana se
  clasifican como `device` (escritos desde el teléfono). Se degrada la
  atribución, nunca se pierde el mensaje. `outbound_messages` permite reconciliar
  a mano.

## Desarrollo

```bash
cd server
go build ./... && go vet ./... && go test ./...
```

Los tests de integración del runner de migraciones se saltan salvo que haya una
base a mano:

```bash
docker run --rm -e POSTGRES_PASSWORD=test -p 5433:5432 postgres:16
TEST_DATABASE_URL=postgres://postgres:test@localhost:5433/postgres \
  go test ./internal/kernel/db/
```

```bash
cd web
npm install
npm run typecheck && npm run build
```
