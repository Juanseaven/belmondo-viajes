# Belmondo Viajes

Panel de administración de una agencia de viajes, organizado en módulos. Se
construye de a poco: cada herramienta que la agencia necesita entra como un
módulo nuevo, sin tocar los que ya están.

Hoy hay tres:

- **mirror**, el espejo de WhatsApp. Refleja en tiempo real todas las líneas de
  la agencia en una bandeja compartida, filtrable por número, con envío desde el
  panel.
- **operaciones**, los clientes y viajes de la agencia. La ficha de cada viaje
  vive al lado de la conversación del cliente, y la pantalla **Viajes** junta lo
  que hay que hacer hoy (Pendientes) y la consulta de todos los viajes. La
  pantalla **Clientes** tiene a quien compra, con o sin WhatsApp, con todos sus
  viajes; un cliente cargado a mano tiene sus viajes completos sin chat.
- **requisitos**, las reglas de entrada de cada país para cada nacionalidad.
  Salen solas de Wikipedia y Passport Index (Claude lee Wikipedia y cada dato
  lleva su cita); nadie las carga ni las aprueba. Operaciones las cruza con los
  documentos y visas de cada pasajero y dice qué cumple, qué falta y qué hay
  que verificar.

Documentos de diseño:

- Arquitectura modular: [docs/superpowers/specs/2026-08-04-arquitectura-modular-design.md](docs/superpowers/specs/2026-08-04-arquitectura-modular-design.md)
- Módulo mirror: [docs/superpowers/specs/2026-08-01-panel-espejo-whatsapp-design.md](docs/superpowers/specs/2026-08-01-panel-espejo-whatsapp-design.md)
- Módulo operaciones: [docs/superpowers/specs/2026-08-20-modulo-operaciones-design.md](docs/superpowers/specs/2026-08-20-modulo-operaciones-design.md)
- Pantalla de Viajes: [docs/superpowers/specs/2026-10-20-pantalla-viajes-design.md](docs/superpowers/specs/2026-10-20-pantalla-viajes-design.md)
- Pantalla de Clientes: [docs/superpowers/specs/2026-10-22-modulo-clientes-design.md](docs/superpowers/specs/2026-10-22-modulo-clientes-design.md)
- Pagos registrados y archivos del cliente: [docs/superpowers/specs/2026-10-22-pagos-y-archivos-design.md](docs/superpowers/specs/2026-10-22-pagos-y-archivos-design.md)
- Requisitos de viaje: [docs/superpowers/specs/2026-10-20-requisitos-de-viaje-design.md](docs/superpowers/specs/2026-10-20-requisitos-de-viaje-design.md)

## Estado

| | |
|---|---|
| Kernel | Auth (login, JWT, usuarios), migraciones por módulo, websockets, R2 |
| mirror — ingesta | Funciona: webhook → worker particionado → Postgres → R2 → websocket |
| mirror — lectura | Funciona: bandeja unificada o por línea, hilo paginado, búsqueda dentro de los mensajes, marcar revisado |
| mirror — escritura | Funciona: enviar texto y adjuntos, alta y desvinculación de línea con QR desde el panel |
| operaciones — ficha | Funciona: viaje, pasajeros, pago, cliente, estado y vendedor junto al chat; 31 controles con cierre derivado, por acción o manual |
| operaciones — Viajes | Funciona: Pendientes (atrasado, esta semana, salen pronto) y Todos los viajes con búsqueda, filtros y orden |
| operaciones — Clientes | Funciona: lista con búsqueda (también por teléfono y pasajero) y chips, página del cliente con viajes, chats y con quién viaja; alta a mano con aviso de repetidos; viajes sin chat en un panel aparte, con «Marcar hecho»; traer pasajeros de viajes anteriores; vincular chats |
| requisitos | Funciona: reglas por nacionalidad y país desde Wikipedia (con espera de 72 horas) y Passport Index; extracción con Claude citada y verificada; revisión semanal. Necesita `ANTHROPIC_API_KEY` y `REQUISITOS_CONTACTO` |
| operaciones — requisitos | Funciona: itinerario, documentos y visas por pasajero; requisitos por pasajero y país con estado cumple, falta o verificar; confirmaciones; «Requisitos migratorios» y el aviso al cliente salen de eso |
| operaciones — pagos y archivos | Funciona: pagos registrados en dólares o pesos que suben la barra hasta el total, la seña se da por cobrada sola y el total en pesos se calcula; pestaña Archivos con las fotos, videos y documentos de los chats del cliente, con favoritos compartidos y visor |
| Frontend | Login, shell con menú por módulos, bandeja del espejo, ficha del viaje y pantallas de Viajes y Clientes |

### API del módulo mirror

Todo bajo `/api/mirror`, con sesión. Los listados paginan por keyset: la
respuesta trae `nextCursor` y se devuelve tal cual en la petición siguiente.

```
GET    /lines                          líneas + sin-revisar de cada una
GET    /conversations                  ?instance= ?q= ?cursor= ?limit=
GET    /conversations/{id}
GET    /conversations/{id}/avatar     foto temporal autenticada
GET    /conversations/{id}/messages    ?before= | ?around=<messageId>
POST   /conversations/{id}/messages    JSON para texto, multipart con adjunto
POST   /conversations/{id}/read
GET    /search                         ?q= ?instance=   (mínimo 3 caracteres)
GET    /media/{id}                     URL firmada de vida corta

POST   /lines                          (admin) da de alta una línea
DELETE /lines/{id}                     (admin) la desvincula
GET    /lines/{id}/qr                  (admin) último QR conocido
```

Enviar exige una **`idempotencyKey` generada por el cliente**: `outbound_messages`
la tiene como `UNIQUE`, así que un doble clic o un reintento del navegador
devuelven el envío original en vez de mandar el mensaje dos veces.

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

**La regla que gobierna todo:** el kernel no conoce a ningún módulo. Los módulos
sí pueden depender entre sí en una sola dirección: pueden leer tablas que el
módulo dueño declare públicas y declarar claves foráneas hacia ellas, pero nunca
escribirlas. El grafo debe ser acíclico y las dependencias se pasan a mano al
construir. Si algo del kernel necesitara importar un módulo, esa pieza no era del
kernel.

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
docker compose up -d evolution-go
docker compose up -d backend web
```

### Compartir una demo temporal por Cloudflare

La demo usa una URL aleatoria `https://*.trycloudflare.com` y no requiere
cuenta de Cloudflare, dominio ni abrir puertos del router. Todo el stack sigue
corriendo en esta computadora; PostgreSQL, Redis y Evolution no se publican.

Con Docker Desktop activo:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/start-demo.ps1
```

El comando imprime la URL que hay que compartir. El visitante entra con un
usuario del CRM; la URL deja de funcionar si se apaga la computadora, Docker o
el contenedor del túnel. Para cerrarla y restaurar la configuración local:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/stop-demo.ps1
```

La URL cambia en cada inicio. Es una configuración para demostraciones, no un
despliegue de producción.

**El esquema se aplica solo**, al arrancar el backend. No hay que correr `psql` a
mano: el runner compara el `embed.FS` de cada módulo contra `schema_migrations` y
aplica lo que falte, cada archivo en su propia transacción.

El primer login usa `SEED_ADMIN_EMAIL` / `SEED_ADMIN_PASSWORD`, que solo se
tienen en cuenta si la tabla `users` está vacía. Después de entrar, cambiá la
contraseña desde el panel y sacá esas dos líneas del `.env`.

### Activar Cloudflare R2 para los adjuntos

R2 es opcional en desarrollo. Sin configuración, los adjuntos quedan en el
volumen Docker privado `blobs`; con R2, el backend sigue entregando únicamente
URLs firmadas y el bucket nunca necesita acceso público.

1. Crear un bucket R2 **privado** y un token S3 limitado a lectura/escritura de
   ese bucket.
2. Antes de cambiar el store, hacer una copia puntual del volumen `blobs` si se
   quieren conservar los adjuntos actuales. El backend no migra objetos entre
   stores automáticamente: al activar R2, las filas existentes siguen apuntando
   a claves que todavía sólo existen en el volumen local.
3. Completar juntas en `.env` `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`,
   `R2_SECRET_ACCESS_KEY` y `R2_BUCKET`. No commitear esos valores. Una
   configuración parcial es inválida y el backend debe rechazarla.
4. Recrear el backend con `docker compose up -d --build backend` y enviar una
   imagen nueva por WhatsApp. Abrirla desde el panel confirma tanto el upload
   como la firma; que el proceso arranque no demuestra que las credenciales
   puedan operar sobre el bucket.

La copia previa del volumen es una operación deliberada y separada porque los
objetos actuales pueden contener documentación sensible. No se habilita una
migración automática ni un bucket público para simplificarla.

## Dar de alta la línea de un vendedor

Desde el panel: **Conectar línea**, se escribe de quién es el número, y aparece el
QR para escanear desde *Dispositivos vinculados* de ese teléfono. Solo los
usuarios con rol `admin` lo ven.

El orden interno importa: **la fila en `instances` se crea antes que la instancia
en Evolution.** Al revés, si Evolution se creara y el `INSERT` fallara, quedaría
una instancia fantasma vinculada y recibiendo mensajes que el backend descarta
por no encontrar su fila — un fallo invisible. En este orden, el fallo se ve: una
línea que figura sin conectar y se puede reintentar o borrar.

Desvincular no borra la fila, la marca inactiva: las conversaciones y mensajes
cuelgan de ella con `ON DELETE CASCADE`, así que borrarla se llevaría puesto el
historial.

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
- La atribución de un saliente tiene dos fuentes: la marca en Redis que deja el
  envío, y `outbound_messages` como respaldo. Si Redis se reinicia o el webhook le
  gana la carrera a la marca, el autor se recupera igual de la base. Solo un
  mensaje escrito de verdad desde el teléfono queda sin autor.

## Desarrollo

```bash
cd server
go build ./... && go vet ./... && go test ./...
```

Los tests de integración (runner de migraciones y consultas del espejo) se saltan
salvo que haya una base a mano. Aplican las migraciones **reales** sobre un
esquema limpio, así que verifican el esquema y el SQL juntos:

```bash
createdb viajes_crm_test
TEST_DATABASE_URL='postgres://postgres:clave@127.0.0.1:5432/viajes_crm_test?sslmode=disable' \
  go test ./...
```

La URL apunta a una base cualquiera donde se pueda crear y borrar: cada paquete
de test se crea la suya (`viajes_crm_test_kernel_db`, `viajes_crm_test_mirror_postgres`)
y la borra al terminar. Eso es lo que permite que `go test ./...` corra los
paquetes en paralelo, que es su comportamiento por defecto — compartiendo una
sola base se pisan al limpiar el esquema y fallan de forma intermitente.

Con Postgres en Docker sobre Windows, cada test tarda unos 27 segundos en
sincronizar a disco y `operaciones` pasa los 10 minutos del límite de `go test`.
Una base descartable sin sincronización lo baja a un minuto:

```bash
docker run -d --rm --name crm-test-pg -e POSTGRES_PASSWORD=test -p 127.0.0.1:55432:5432   postgres:16-alpine -c fsync=off -c synchronous_commit=off -c full_page_writes=off
TEST_DATABASE_URL='postgres://postgres:test@127.0.0.1:55432/postgres?sslmode=disable' go test ./...
```

Con Docker Desktop en Windows, la base de test tiene que ir **sin fsync**. Cada
test recrea el esquema entero, y con fsync cada DDL espera al disco virtual:
15 segundos por test, y el paquete de operaciones supera los 10 minutos que
`go test` da antes de abortar. Sin fsync, toda la suite tarda medio minuto:

```bash
docker run -d --rm --name wm-testdb -p 55432:5432 -e POSTGRES_PASSWORD=test \
  postgres:16-alpine -c fsync=off -c synchronous_commit=off -c full_page_writes=off
TEST_DATABASE_URL='postgres://postgres:test@localhost:55432/postgres?sslmode=disable' \
  go test ./...
```

```bash
cd web
npm install
npm run typecheck && npm run build
```
