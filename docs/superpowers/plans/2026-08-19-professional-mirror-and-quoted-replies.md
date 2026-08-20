# Professional Mirror And Quoted Replies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Profesionalizar la bandeja real y permitir responder mensajes concretos con la cita nativa de WhatsApp, incluidos los envíos con adjuntos.

**Architecture:** El cliente identifica el mensaje citado por el UUID interno. El backend valida que ese UUID pertenezca a la conversación, resuelve `messageId` y `participant` de WhatsApp y recién entonces llama a Evolution. La lectura del hilo resuelve una vista resumida de la cita mediante un `LEFT JOIN`, de modo que la UI puede mostrarla y saltar al mensaje original sin consultas por burbuja.

**Tech Stack:** Go 1.24, PostgreSQL/pgx, Evolution Go 0.7.2, Next.js 16, React 19, TypeScript, Vitest y Testing Library.

**Spec:** Alcance aprobado explícitamente por el usuario en la conversación actual; no se crea un documento de diseño por pedido del usuario.

## Global Constraints

- Usar únicamente conversaciones, líneas y contactos reales; no copiar datos operativos ficticios del mock.
- La cita debe funcionar para texto y adjuntos.
- El backend debe rechazar una cita que no pertenezca a la conversación seleccionada.
- No agregar consultas N+1 al hilo.
- Preservar todos los cambios locales preexistentes y no realizar commits automáticos sobre el árbol de trabajo acumulado.

---

### Task 1: Contrato seguro de cita saliente

**Files:**
- Modify: `server/internal/modules/mirror/store/outbound.go`
- Modify: `server/internal/modules/mirror/store/postgres/outbound.go`
- Modify: `server/internal/modules/mirror/send.go`
- Modify: `server/internal/modules/mirror/send_test.go`
- Modify: `server/internal/modules/mirror/api_write.go`
- Modify: `server/internal/modules/mirror/api_test.go`

**Interfaces:**
- Consumes: `SendRequest.ConversationID` y `SendRequest.ReplyToMessageID`.
- Produces: `OutboundStore.QuoteTarget(ctx, conversationID, messageID) (QuoteTarget, error)` y una cita validada para Evolution.

- [ ] **Step 1: Escribir pruebas fallidas del servicio**

Agregar casos que demuestren que un envío con `ReplyToMessageID` consulta la cita de la misma conversación, la pasa a Evolution y aborta antes del envío cuando el mensaje no existe o pertenece a otra conversación.

- [ ] **Step 2: Ejecutar la prueba y verificar RED**

Run: `go test ./internal/modules/mirror -run 'TestEnvioCitado|TestCitaAjena' -count=1`

Expected: FAIL porque `ReplyToMessageID`, `QuoteTarget` y el argumento citado todavía no existen.

- [ ] **Step 3: Implementar resolución y validación mínima**

Crear `store.QuoteTarget` con `WAMessageID` y `Participant`; resolverlo en PostgreSQL filtrando simultáneamente por `messages.id` y `messages.conversation_id`. En `SendService.Send`, resolver la cita antes de crear el outbound y pasarla tanto a texto como a media.

- [ ] **Step 4: Escribir y ejecutar pruebas fallidas del handler**

Agregar casos JSON y multipart que prueben el campo `replyToMessageId`, y un caso de mensaje inexistente que devuelva 404 sin despachar.

Run: `go test ./internal/modules/mirror -run 'Test.*Reply|Test.*Cita' -count=1`

Expected: FAIL antes de ampliar los parsers JSON/multipart.

- [ ] **Step 5: Implementar el contrato HTTP y verificar GREEN**

Aceptar `replyToMessageId` en JSON y multipart, normalizar espacios y mapear `store.ErrNotFound` a 404.

Run: `go test ./internal/modules/mirror -count=1`

Expected: PASS.

### Task 2: Payload citado hacia Evolution

**Files:**
- Modify: `server/internal/modules/mirror/evolution/client.go`
- Modify: `server/internal/modules/mirror/evolution/client_test.go`

**Interfaces:**
- Consumes: `*evolution.Quote{MessageID, Participant}`.
- Produces: cuerpos `/send/text` y `/send/media` con `quoted.messageId` y `quoted.participant` solo cuando hay cita.

- [ ] **Step 1: Escribir pruebas de contrato HTTP fallidas**

Extender el servidor HTTP de prueba para decodificar los cuerpos reales y afirmar el objeto literal `quoted` en texto y media, además de comprobar que se omite en un envío normal.

- [ ] **Step 2: Ejecutar y verificar RED**

Run: `go test ./internal/modules/mirror/evolution -run 'TestSend.*Quoted' -count=1`

Expected: FAIL porque los métodos actuales no reciben ni serializan la cita.

- [ ] **Step 3: Implementar el payload mínimo y verificar GREEN**

Agregar `Quote`, ampliar las firmas de `SendText` y `SendMedia` y adjuntar el objeto solo si `MessageID` no está vacío.

Run: `go test ./internal/modules/mirror/evolution -count=1`

Expected: PASS.

### Task 3: Vista de la cita en la lectura del hilo

**Files:**
- Modify: `server/internal/modules/mirror/store/read.go`
- Modify: `server/internal/modules/mirror/store/postgres/thread.go`
- Modify: `server/internal/modules/mirror/store/postgres/read_integration_test.go`

**Interfaces:**
- Consumes: `messages.quoted_wa_message_id`.
- Produces: `ThreadMessage.Quote *QuotedThreadMessage` con UUID interno, dirección, tipo, cuerpo y autor resumido.

- [ ] **Step 1: Escribir una integración fallida**

Insertar dos mensajes en una conversación, hacer que el segundo cite el `wa_message_id` del primero y afirmar que `Thread` devuelve el UUID interno y el resumen del original. Incluir una cita no disponible para comprobar que no desaparece el mensaje actual.

- [ ] **Step 2: Ejecutar y verificar RED**

Run: `go test ./internal/modules/mirror/store/postgres -run TestThread.*Quote -count=1`

Expected: FAIL porque `ThreadMessage.Quote` no existe.

- [ ] **Step 3: Implementar un único LEFT JOIN y verificar GREEN**

Unir el mensaje citado por `instance_id + wa_message_id`, resolver el autor con el usuario del panel o `sender_name`, y escanear el resumen sin consultas adicionales.

Run: `go test ./internal/modules/mirror/store/postgres -count=1`

Expected: PASS.

### Task 4: Interacción de responder y saltar al original

**Files:**
- Modify: `web/src/modules/mirror/api.ts`
- Modify: `web/src/modules/mirror/MessageBubble.tsx`
- Modify: `web/src/modules/mirror/MessageBubble.test.tsx`
- Modify: `web/src/modules/mirror/Composer.tsx`
- Create: `web/src/modules/mirror/Composer.test.tsx`
- Modify: `web/src/modules/mirror/Thread.tsx`
- Create: `web/src/modules/mirror/Thread.test.tsx`

**Interfaces:**
- Consumes: `Message.quote`, `replyToMessageId` y `mirrorApi.messages(..., {around})`.
- Produces: botón accesible “Responder”, barra de cita cancelable, payload de envío citado y salto/carga del original.

- [ ] **Step 1: Escribir pruebas fallidas de burbuja y compositor**

Probar que una burbuja expone “Responder”, pinta la cita con autor/resumen y notifica el UUID al hacer clic. Probar que el compositor muestra/cancela la selección y manda `replyToMessageId` para texto y archivo.

- [ ] **Step 2: Ejecutar y verificar RED**

Run: `npm test -- MessageBubble.test.tsx Composer.test.tsx`

Expected: FAIL porque las props y estados todavía no existen.

- [ ] **Step 3: Implementar selección y envío de cita**

Mantener `replyingTo` en `Thread`, pasarlo a `Composer`, limpiarlo al cancelar o después de un envío aceptado y conservarlo si el envío falla para permitir reintentar.

- [ ] **Step 4: Escribir la prueba fallida de salto**

Probar que hacer clic en una cita ya cargada desplaza/resalta el original y que una cita fuera de la ventana solicita `around=<uuid>` antes de enfocarla.

- [ ] **Step 5: Implementar el salto y verificar GREEN**

Reutilizar el mecanismo de foco de `useThread`, sin crear un segundo cargador de mensajes.

Run: `npm test -- MessageBubble.test.tsx Composer.test.tsx Thread.test.tsx`

Expected: PASS.

### Task 5: Pase visual profesional con datos reales

**Files:**
- Modify: `web/src/modules/mirror/Thread.tsx`
- Modify: `web/src/modules/mirror/ConversationList.tsx`
- Modify: `web/src/modules/mirror/ContactDetails.tsx`
- Modify: `web/src/modules/mirror/ContactDetails.test.tsx`
- Modify: `web/src/modules/mirror/MirrorScreen.live.test.tsx`
- Modify: `web/src/app/globals.css`

**Interfaces:**
- Consumes: `Conversation`, `Line` y mensajes reales existentes.
- Produces: encabezado compacto, separadores diarios, estados legibles, panel de detalles estructurado y layout responsivo.

- [ ] **Step 1: Escribir pruebas visual-semánticas fallidas**

Probar las secciones “Contacto”, “Atención” y “Conversación”, valores no disponibles, separadores diarios y acciones accesibles sin afirmar clases privadas.

- [ ] **Step 2: Ejecutar y verificar RED**

Run: `npm test -- ContactDetails.test.tsx MirrorScreen.live.test.tsx Thread.test.tsx`

Expected: FAIL porque las secciones y separadores todavía no existen.

- [ ] **Step 3: Implementar componentes y estilos**

Agrupar identidad/estado en el encabezado, mejorar la densidad de la bandeja, añadir separadores por día, estructurar detalles con datos reales y completar estados hover/focus/responsive de respuestas citadas.

- [ ] **Step 4: Ejecutar pruebas focalizadas y refactorizar**

Run: `npm test -- ContactDetails.test.tsx MirrorScreen.live.test.tsx Thread.test.tsx MessageBubble.test.tsx Composer.test.tsx`

Expected: PASS sin warnings de React.

### Task 6: Verificación integral y despliegue local

**Files:**
- Verify only: all changed server and web files.

**Interfaces:**
- Consumes: implementación completa.
- Produces: evidencia fresca de tests, tipos, vet, builds y servicios saludables.

- [ ] **Step 1: Verificar backend completo**

Run: `go test ./... -count=1 && go vet ./... && go build ./...`

Expected: exit 0 en los tres comandos.

- [ ] **Step 2: Verificar frontend completo**

Run: `npm test && npm run typecheck && npm run build`

Expected: exit 0 y cero pruebas fallidas.

- [ ] **Step 3: Reconstruir los dos servicios modificados**

Run: `docker compose up -d --build backend web`

Expected: contenedores `wm-backend` y `wm-web` recreados sin afectar Postgres, Redis ni la sesión de Evolution.

- [ ] **Step 4: Verificar salud y rutas**

Run: `docker compose ps; Invoke-WebRequest http://localhost:3000/mirror; Invoke-WebRequest http://localhost:8090/health`

Expected: servicios healthy y respuestas HTTP exitosas.

