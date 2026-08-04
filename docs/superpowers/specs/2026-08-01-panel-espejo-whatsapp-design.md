# Panel espejo de WhatsApp multi-instancia — Diseño

Fecha: 2026-08-01
Estado: implementado en parte; **premisas de producto revocadas**

> **Leer con esta advertencia.** Este documento se conserva sin editar como
> registro de por qué el módulo `mirror` está construido como está. Sus
> decisiones técnicas siguen vigentes —el particionado por conversación, la
> idempotencia por `dedupe_key`, el rendezvous en Redis, el pipeline de media—
> pero **dos premisas de producto se levantaron** el 2026-08-04
> ([spec de arquitectura modular](2026-08-04-arquitectura-modular-design.md)):
>
> 1. **"Los vendedores no son usuarios."** Ahora sí lo son. `users` es el
>    directorio del personal de la agencia.
> 2. **"Panel exclusivo del administrador, secreto para los vendedores."** Ahora
>    es una bandeja compartida: todo el personal entra, ve todas las líneas y
>    puede escribir por cualquiera.
>
> La distinción `origin = 'panel' | 'device'` sobrevive intacta, pero cambia de
> significado: ya no responde "¿fue la dirección o el vendedor?" sino "¿se
> escribió desde la app o desde el teléfono?".
>
> Este espejo tampoco es ya el sistema entero: es el primer módulo de
> **viajes_crm**.

## Problema

Una agencia de viajes tiene varios vendedores, cada uno con su propia línea de
WhatsApp y su propio teléfono. La dirección no tiene visibilidad de esas
conversaciones: no puede auditar qué se le prometió a un cliente, no puede
recuperar el pasaporte que el cliente envió a un vendedor que ya no está, y no
puede intervenir cuando un chat se está enfriando.

## Qué construimos

Un panel web **exclusivo para el administrador** que refleja en tiempo real todas
las líneas de WhatsApp de la agencia, con capacidad de escritura.

No es un CRM colaborativo ni una bandeja compartida. Los vendedores **no son
usuarios del sistema**: siguen trabajando desde la app nativa de WhatsApp en su
teléfono y no saben que el panel existe. El sistema es un observador pasivo con
un canal de intervención.

### Consecuencias de diseño

1. **Un vendedor ≈ una `instance`.** La tabla `users` contiene solo
   administradores del panel. El nombre del vendedor vive en
   `instances.owner_label`, no en `users`.
2. **No hay asignación de conversaciones.** No existe `assigned_user_id`.
3. **La tabla `conversations` sobrevive** pero como *thread materializado* por
   `(instance_id, contact_id)`: mantiene `last_message_at`, `unread_count` y un
   preview para que la bandeja ordene y pagine sin escanear `messages`.
4. **El filtro por línea es de primera clase.** `messages.instance_id` está
   desnormalizado (aunque sea derivable vía `conversations`) porque el filtro por
   línea es la consulta central del producto y no debe pagar un JOIN.

## Auditoría de origen (requisito derivado)

Un mensaje saliente puede tener dos autores y el payload del webhook no los
distingue: ambos llegan con `key.fromMe: true`.

- El **vendedor** escribió desde su teléfono.
- El **administrador** escribió desde el panel.

Para una herramienta de auditoría, confundirlos la invalida.

**Solución.** Cuando el panel envía, Evolution devuelve `key.id` en la respuesta
201. El backend escribe ese id en Redis (`SETEX panel:sent:<id> 300`) *antes* de
que el webhook pueda llegar. Al procesar un evento con `fromMe: true`:

- id presente en Redis → `origin = 'panel'`, se registra `sent_by_user_id`.
- id ausente → `origin = 'device'`: el vendedor respondió desde su móvil.

El TTL de 5 minutos cubre holgadamente la latencia del webhook. Si Redis se
pierde, el peor caso es que un mensaje del panel se clasifique como `device`:
degradación silenciosa, nunca corrupción. Los envíos del panel también se
persisten en `outbound_messages` para reconciliación posterior.

## Arquitectura de ingesta

Evolution API reintenta los webhooks fallidos hasta 10 veces con backoff
exponencial. Eso define el contrato: **el handler debe ser rápido e idempotente**.

```
Evolution ──POST /webhooks/evolution──> handler HTTP
                                          │  valida apikey
                                          │  INSERT webhook_events (dedupe_key UNIQUE)
                                          │  responde 200
                                          ▼
                              canal particionado (shard N)
                                          ▼
                              worker N  ──> normaliza ──> store
                                          ├─> media job ──> R2
                                          └─> ws.Hub ──> navegador del admin
```

### Workers particionados

Un pool plano de goroutines desordena los mensajes de un mismo chat: dos eventos
de la misma conversación pueden procesarse en paralelo y persistirse invertidos.

Se particiona por `hash(instance_id + remote_jid) % N`. Todos los eventos de una
conversación caen siempre en la misma goroutine — orden garantizado dentro del
chat — mientras que conversaciones distintas siguen procesándose en paralelo. Es
el mismo principio que las particiones de Kafka.

### Contrapresión

Si el canal de un shard está lleno, el handler responde `503`. No descarta.
Evolution reintentará con backoff, así que la contrapresión es segura y el
sistema degrada en latencia, no en pérdida de datos.

### Idempotencia

`webhook_events.dedupe_key` (`UNIQUE`) combina instancia, evento e id de mensaje.
Un reintento choca contra el índice y se descarta antes de tocar el pool. La
tabla también sirve como registro forense del payload crudo.

## Pipeline de media

El webhook no trae el archivo: trae una URL de `mmg.whatsapp.net` cifrada más un
`mediaKey`. No es descargable directamente.

El worker de media llama a
`POST /chat/getBase64FromMediaMessage/{instance}`, obtiene el binario descifrado
y lo sube a R2 bajo una clave que controlamos:

```
{instance_name}/{yyyy-mm}/{wa_message_id}.{ext}
```

Se descarta la alternativa de activar `S3_ENABLED` en Evolution (que subiría a R2
por su cuenta) porque acopla el almacenamiento al ciclo de vida de un contenedor
desechable y nos quita el control del naming, la retención y los thumbnails.

El frontend nunca recibe una URL pública de R2: pide
`GET /api/media/{id}` y el backend responde con una URL firmada de vida corta.
Los pasaportes no deben quedar accesibles por URL adivinable ni indexable.

`media_assets.storage_status` (`pending` → `processing` → `stored` | `failed`) más
un contador de intentos permite reintentar descargas fallidas sin bloquear la
ingesta del mensaje: el mensaje aparece en el panel de inmediato, el archivo
llega después.

## Websockets

Un `Hub` central mantiene el registro de clientes conectados y hace fan-out de los
eventos normalizados. Cada cliente tiene su propio canal con buffer; si se llena
(navegador lento o congelado), se cierra esa conexión en lugar de bloquear al
worker. El frontend se re-sincroniza al reconectar vía REST.

La suscripción se filtra por `instance_id` en el servidor: un cliente que está
mirando una línea no recibe el tráfico de las demás.

## Red y despliegue

Single-tenant: un `docker-compose` por agencia, base de datos aislada, sin
`tenant_id` en ninguna tabla.

Red interna `backbone` sin exposición pública salvo el frontend y la API:

- `postgres` y `redis`: sin puertos publicados.
- `evolution-api`: publicado solo en `127.0.0.1:8080` para leer el QR de
  vinculación desde un túnel SSH. Nunca abierto a internet.
- `backend`: publicado para el frontend.
- El webhook viaja por DNS interno de Docker: `http://backend:8080/webhooks/evolution`.

Un único Postgres aloja dos bases separadas: `evolution` (propiedad de Evolution
API, gestionada por su Prisma) y `mirror` (nuestra). Nunca se mezclan esquemas ni
se leen las tablas de Evolution directamente: el contrato es el webhook.

## Testing

- `internal/webhook`: parseo de payloads reales de cada tipo de mensaje
  (texto, imagen, audio ogg, documento) contra fixtures JSON.
- `internal/worker`: verificar que dos eventos del mismo `remote_jid` caen en el
  mismo shard y que la contrapresión devuelve 503 en vez de bloquear.
- `internal/media`: fake de R2 vía interfaz `Uploader`.
- Test de integración con Postgres real sobre la idempotencia de `dedupe_key`.

## Fuera de alcance

Round-robin, respuestas automáticas, plantillas, métricas de vendedor, campañas
masivas, multi-tenant.
