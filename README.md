# Panel espejo de WhatsApp

Panel web de auditoría para agencias de viajes. Refleja en tiempo real todas las
líneas de WhatsApp de la agencia en una sola bandeja, filtrable por línea, con
capacidad de intervención del administrador.

**No es un CRM colaborativo.** Los vendedores no son usuarios del sistema: siguen
trabajando desde la app nativa de WhatsApp en su teléfono. Cada vendedor es una
*instancia* conectada. Solo el administrador entra al panel.

El diseño completo está en
[docs/superpowers/specs/2026-08-01-panel-espejo-whatsapp-design.md](docs/superpowers/specs/2026-08-01-panel-espejo-whatsapp-design.md).

## Estructura

```
docker-compose.yml           Stack completo, single-tenant
infra/postgres/init/         Creación de la base de Evolution en el primer arranque
server/                      Backend Go
  cmd/api/                   Entrypoint: arranque y apagado ordenado
  internal/
    config/                  Carga y validación del entorno (falla al arrancar)
    domain/                  Vocabulario del sistema. No importa nada del proyecto
    webhook/                 Ingesta: parseo de Evolution + handler HTTP
    worker/                  Pool particionado genérico (orden por conversación)
    ingest/                  Lógica de negocio que corre dentro del pool
    evolution/               Cliente saliente hacia Evolution API
    media/                   Descarga de adjuntos + subida a R2
    ws/                      Hub de websockets con fan-out
    store/                   Puertos de persistencia
      postgres/              Implementación con pgx
      rediscache/            Rendezvous de envíos del panel
    httpapi/                 Router y middlewares
  migrations/                Esquema SQL
web/                         Frontend Next.js
```

La dependencia va siempre hacia dentro: `domain` no importa a nadie, `store`
define interfaces que `postgres` implementa, y la lógica de negocio depende de
las interfaces, no de pgx.

## Arranque

```bash
cp .env.example .env
# rellenar: POSTGRES_PASSWORD, EVOLUTION_API_KEY, JWT_SECRET, credenciales de R2
#   openssl rand -hex 32

docker compose up -d postgres redis
docker compose up -d evolution-api
docker compose up -d backend web
```

Aplicar el esquema:

```bash
docker compose exec -T postgres psql -U mirror -d mirror < server/migrations/0001_init.sql
```

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

El QR llega por el webhook `qrcode.updated` y se muestra en el panel.

## Notas de operación

- La API de Evolution nunca debe exponerse a internet. Su `apikey` da control
  total sobre todas las líneas conectadas.
- El bucket de R2 es privado. El frontend pide `GET /api/media/{id}` y recibe una
  URL firmada de vida corta. Por ahí viajan pasaportes.
- `webhook_events` crece rápido. Purgar pasados 30-90 días.
- Si Redis se pierde, los mensajes enviados desde el panel durante esa ventana se
  clasifican como `device` (escritos por el vendedor). Se degrada la atribución,
  nunca se pierde el mensaje. `outbound_messages` permite reconciliar a mano.

## Desarrollo

```bash
cd server
go build ./...
go vet ./...
go test ./...
```
