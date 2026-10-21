# Avatares temporales y media privada Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mostrar avatares de WhatsApp bajo demanda sin persistirlos y dejar verificado el pipeline privado de adjuntos hacia R2.

**Architecture:** El backend resuelve la identidad de una conversación, consulta `POST /user/avatar` en Evolution y sirve una copia autenticada desde una caché acotada en memoria. El frontend descarga ese binario con el JWT y usa un `blob:` URL; el pipeline de adjuntos existente no cambia, sólo se documenta y verifica su activación por configuración.

**Tech Stack:** Go 1.26, `net/http`, pgx, React 19, Next.js 16.3, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-21-avatares-y-media-privada-design.md`

## Global Constraints

- Los avatares no se escriben en Postgres, R2 ni disco.
- TTL positivo: 6 horas; caché HTTP negativa: 15 minutos.
- Caché en memoria: 32 MiB totales; respuesta individual: 2 MiB máximo.
- Sólo se descargan URLs HTTPS alojadas bajo `whatsapp.net` y respuestas `image/*`.
- Evolution y R2 permanecen privados; ningún secreto viaja en query params ni se commitea.
- Los cambios existentes en `store/read.go` y `store/postgres/read.go` se preservan.

## Review Focus

- JID individual, LID y JID de grupo: todos deben llegar intactos a Evolution.
- Foto oculta, ausente o timeout: devolver fallback con caché negativa, no bloquear la UI.
- URL o redirección no HTTPS o ajena a WhatsApp: rechazarla sin iniciar una descarga SSRF.
- Imagen sin `Content-Type`, no imagen o mayor a 2 MiB: no cachear ni servir contenido parcial.
- Cambio rápido de conversación/desmontaje: revocar el `blob:` URL y no aplicar una respuesta tardía.

---

### Task 1: Contrato de avatar con Evolution Go

**Files:**
- Modify: `server/internal/modules/mirror/evolution/client.go`
- Test: `server/internal/modules/mirror/evolution/client_test.go`

**Interfaces:**
- Consumes: `Client.do(ctx, method, path, apiKey, in, out)` y `instanceToken(instance)`.
- Produces: `func (c *Client) AvatarURL(ctx context.Context, instance, jid string) (string, error)`.

- [ ] **Step 1: Escribir la prueba fallida**

  Agregar `TestAvatarURLUsesTheInstanceTokenAndReadsEvolutionURL`: el servidor de prueba debe recibir `POST /user/avatar`, la API key derivada y JSON literal `{"number":"...@s.whatsapp.net","preview":true}`; responder `{"data":{"URL":"https://pps.whatsapp.net/avatar.jpg"}}` y exigir esa URL como resultado. Agregar el caso `data.URL` vacío que debe devolver error.

- [ ] **Step 2: Verificar RED**

  Run: `go test ./internal/modules/mirror/evolution -run AvatarURL -count=1`
  Expected: FAIL porque `AvatarURL` todavía no existe.

- [ ] **Step 3: Implementar el contrato mínimo**

  En `client.go`, implementar la firma anterior con `preview: true`, token por instancia y lectura de `data.URL`; una URL vacía es error.

- [ ] **Step 4: Verificar GREEN**

  Run: `go test ./internal/modules/mirror/evolution -run AvatarURL -count=1`
  Expected: PASS.

### Task 2: Servicio temporal y endpoint autenticado

**Files:**
- Create: `server/internal/modules/mirror/avatar.go`
- Create: `server/internal/modules/mirror/avatar_test.go`
- Modify: `server/internal/modules/mirror/mirror.go`
- Modify: `server/internal/modules/mirror/store/postgres/read.go`
- Test: `server/internal/modules/mirror/store/postgres/read_integration_test.go`

**Interfaces:**
- Consumes: `store.OutboundStore.Target(ctx, conversationID) (store.SendTarget, error)` y `AvatarURL` de Task 1.
- Produces: `func NewAvatarService(targets avatarTargetStore, provider avatarProvider, httpClient *http.Client) *AvatarService`, `func (s *AvatarService) Load(ctx context.Context, conversationID string) (AvatarImage, error)` y handler `Module.serveAvatar`.

- [ ] **Step 1: Escribir pruebas fallidas del servicio**

  Probar con servidores HTTP reales, transporte HTTPS controlado y fakes estrechos que: una imagen HTTPS de WhatsApp se devuelve y la segunda carga no vuelve a consultar Evolution ni el origen; una URL HTTP o de otro host se rechaza antes de tocar el origen; una redirección desde WhatsApp hacia otro host se rechaza; una respuesta no `image/*` y una de 2 MiB + 1 byte se rechazan; un id inexistente conserva `store.ErrNotFound`; dos cargas concurrentes hacen una sola consulta; una ausencia se cachea quince minutos; al vencer seis horas se refresca la foto; al superar 32 MiB se desalojan las entradas más antiguas; y el target se resuelve aun cuando la imagen esté cacheada. Inyectar `now func() time.Time` sólo dentro del test/servicio para probar el vencimiento sin sleeps.

- [ ] **Step 2: Verificar RED del servicio**

  Run: `go test ./internal/modules/mirror -run Avatar -count=1`
  Expected: FAIL porque el servicio no existe.

- [ ] **Step 3: Implementar el servicio mínimo**

  Crear tipos privados `avatarTargetStore` y `avatarProvider`, `AvatarImage{Data []byte, ContentType string}`, caché mutex-protegida con TTL de seis horas y presupuesto de 32 MiB, caché negativa de quince minutos, agrupación de cargas concurrentes, validación de URL/MIME/tamaño y de cada redirect mediante `CheckRedirect`, y timeout de ocho segundos alrededor de Evolution y la descarga.

- [ ] **Step 4: Verificar GREEN del servicio**

  Run: `go test ./internal/modules/mirror -run Avatar -count=1`
  Expected: PASS.

- [ ] **Step 5: Escribir pruebas fallidas del handler y view model**

  En `avatar_test.go`, exigir 200 con bytes, `Content-Type` y `Cache-Control: private, max-age=21600`; exigir 404 con `private, max-age=900` para foto no disponible y conversación inexistente; exigir 500 para un fallo del store. En la prueba de integración de lectura, exigir que `AvatarURL` sea exactamente `/api/mirror/conversations/{id}/avatar`, aunque `profile_pic_url` esté vacío.

- [ ] **Step 6: Verificar RED del handler**

  Run: `go test ./internal/modules/mirror ./internal/modules/mirror/store/postgres -run 'Avatar|Conversation' -count=1`
  Expected: FAIL por ruta/handler/view model ausentes.

- [ ] **Step 7: Cablear handler, ruta y URL relativa**

  Reutilizar la instancia `postgres.Outbound` como `avatarTargetStore`, construir `AvatarService` en `New`, montar `GET /conversations/{id}/avatar`, escribir bytes sólo después de una carga válida y sobrescribir `InboxItem.AvatarURL` tras `scanInbox` con la ruta relativa. No tocar los campos `LastDirection`, `LastStatus` ni `LastSender` ya modificados.

- [ ] **Step 8: Verificar GREEN del backend**

  Run: `go test ./internal/modules/mirror ./internal/modules/mirror/evolution ./internal/modules/mirror/store/postgres -count=1`
  Expected: PASS.

### Task 3: Descarga autenticada en `ContactAvatar`

**Files:**
- Modify: `web/src/kernel/api/client.ts`
- Create: `web/src/kernel/api/client.test.ts`
- Modify: `web/src/modules/mirror/ContactAvatar.tsx`
- Test: `web/src/modules/mirror/ContactAvatar.test.tsx`

**Interfaces:**
- Consumes: rutas relativas `avatarUrl` de Task 2 y el JWT guardado por `token`.
- Produces: `apiBlob(path: string, init?: RequestInit): Promise<Blob>` y el comportamiento interno de `ContactAvatar` para rutas `/api/`.

- [ ] **Step 1: Escribir prueba fallida de `apiBlob`**

  Con `fetch` controlado, guardar un JWT literal, devolver una respuesta de imagen y exigir que `apiBlob` envíe `Authorization: Bearer ...` y devuelva el `Blob`; agregar un 401 con cuerpo de error y exigir `ApiError` con `isAuthError=true`.

- [ ] **Step 2: Verificar RED del cliente**

  Run: `npm test -- src/kernel/api/client.test.ts`
  Expected: FAIL porque `apiBlob` no existe.

- [ ] **Step 3: Implementar `apiBlob` sin duplicar autenticación**

  Extraer el `fetch` autenticado compartido por `api` y `apiBlob`; conservar el contrato JSON actual y hacer que sólo `apiBlob` lea `Response.blob()`.

- [ ] **Step 4: Verificar GREEN del cliente**

  Run: `npm test -- src/kernel/api/client.test.ts`
  Expected: PASS.

- [ ] **Step 5: Escribir pruebas fallidas del componente**

  Exigir que un `src="/api/mirror/conversations/c1/avatar"` termine mostrando el `blob:` URL creado desde la respuesta autenticada; que un 404 mantenga las iniciales; y que cambiar `src` o desmontar revoque el URL anterior e ignore una respuesta tardía.

- [ ] **Step 6: Verificar RED del componente**

  Run: `npm test -- src/modules/mirror/ContactAvatar.test.tsx`
  Expected: FAIL porque la ruta interna aún se entrega directamente al `img`.

- [ ] **Step 7: Implementar la carga temporal**

  Convertir `ContactAvatar` en Client Component explícito, cargar rutas internas con `apiBlob` dentro de un efecto cancelable, crear/revocar `URL.createObjectURL` y conservar soporte directo para un `src` externo legado. Un error deja `resolvedSrc` vacío y muestra las iniciales.

- [ ] **Step 8: Verificar GREEN del frontend enfocado**

  Run: `npm test -- src/kernel/api/client.test.ts src/modules/mirror/ContactAvatar.test.tsx`
  Expected: PASS.

### Task 4: Operación de R2 y verificación final

**Files:**
- Modify: `server/internal/modules/mirror/store/postgres/media.go`
- Test: `server/internal/modules/mirror/store/postgres/read_integration_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `Config.UseR2()`, las cuatro variables `R2_*`, `blobs.R2` y el pipeline `media.Service` ya existentes.
- Produces: recuperación de media trabada e instrucciones reproducibles para crear un bucket privado, un token limitado al bucket y activar R2 sin commitear secretos.

- [ ] **Step 1: Escribir la prueba fallida de recuperación**

  Insertar media en `processing` con `updated_at` de más de diez minutos y exigir que `NextPending` la recupere; una fila `processing` reciente no debe recuperarse y provocar dos uploads simultáneos.

- [ ] **Step 2: Verificar RED de recuperación**

  Run (desde `server/`, con `TEST_DATABASE_URL`): `go test ./internal/modules/mirror/store/postgres -run MediaProcessing -count=1`
  Expected: FAIL porque `NextPending` ignora todo estado `processing`.

- [ ] **Step 3: Implementar la recuperación mínima**

  Incluir en `NextPending` filas `processing` con `updated_at < now() - interval '10 minutes'`, manteniendo `attempts < 5` y `FOR UPDATE SKIP LOCKED`. No hace falta migración: `updated_at` y su trigger ya existen.

- [ ] **Step 4: Verificar GREEN de recuperación**

  Run (desde `server/`, con `TEST_DATABASE_URL`): `go test ./internal/modules/mirror/store/postgres -run MediaProcessing -count=1`
  Expected: PASS y la prueba no debe figurar como omitida.

- [ ] **Step 5: Redactar las fuentes binarias del webhook procesado**

  Crear una prueba de integración fallida para `Events.MarkProcessed` que inserte un payload con metadatos, `data.Message.base64` y `data.Message.mediaUrl`; comprobar que al marcarlo `processed` desaparecen sólo esos dos campos y el resto del payload se conserva. Implementar la redacción JSONB en `server/internal/modules/mirror/store/postgres/events.go`.

- [ ] **Step 6: Documentar la activación**

  Agregar una sección breve que indique crear bucket privado, crear credenciales S3 limitadas, completar juntas las cuatro variables y hacer una copia puntual de los objetos locales antes de recrear `backend`. Comprobar con una imagen nueva; un log de arranque no valida las credenciales. Aclarar que los adjuntos anteriores no se migran automáticamente.

- [ ] **Step 7: Formatear y ejecutar suites completas**

  Run: `gofmt -w internal/modules/mirror/avatar.go internal/modules/mirror/avatar_test.go internal/modules/mirror/evolution/client.go internal/modules/mirror/evolution/client_test.go internal/modules/mirror/mirror.go internal/modules/mirror/store/postgres/read.go internal/modules/mirror/store/postgres/read_integration_test.go`

  Run: `go test ./...`
  Expected: PASS.

  Run: `npm test`
  Expected: PASS.

  Run: `npm run typecheck`
  Expected: PASS.

  Run: `npm run build`
  Expected: PASS.

- [ ] **Step 8: Validar el despliegue local sin secretos**

  Rebuild de `backend` y `web`; abrir una conversación con foto visible y otra sin foto. Comprobar foto/iniciales, que el request del avatar lleva header `Authorization`, que el `src` final es `blob:` y que no aparece `token=`. Confirmar por inspección de entorno, mostrando sólo `configured/missing`, si R2 sigue pendiente.

- [ ] **Step 9: Revisar el diff final**

  Run: `git diff --check` y `git status --short`.
  Expected: sin errores de whitespace; los dos cambios previos del usuario siguen presentes y sin sobrescritura accidental.
