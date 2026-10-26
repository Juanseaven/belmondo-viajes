# Cloudflare Quick Tunnel Demo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publicar Belmondo Viajes temporalmente en una única URL de Cloudflare sin exponer servicios internos.

**Architecture:** Un override de Docker Compose agrega Nginx como gateway local para Next.js, la API, WebSockets y archivos. El frontend obtiene las URLs HTTP/WebSocket del origen actual y un script administra el Quick Tunnel y la URL pública requerida por el almacenamiento local.

**Tech Stack:** Docker Compose, Nginx Alpine, Cloudflare `cloudflared`, PowerShell 7/Windows PowerShell, Next.js 16, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-22-demo-cloudflare-tunnel-design.md`

## Global Constraints

- Sólo `demo-gateway` puede recibir tráfico del túnel.
- El puerto del gateway se publica en `127.0.0.1:8088`, nunca en todas las interfaces.
- PostgreSQL, Redis y Evolution no reciben rutas públicas.
- La demo conserva login JWT, firmas y vencimiento de `/files/*`.
- El arranque no escribe secretos ni la URL temporal en `.env`.
- Los cambios existentes del usuario se preservan.
- Los scripts/configuración se verifican con smoke tests reales; no se agregan unit tests que sólo reproduzcan texto declarativo.

## Review Focus

- Una página HTTPS debe abrir el socket con `wss://`, no `ws://`.
- Las llamadas `/api/*` deben permanecer same-origin y conservar el JWT.
- `/files/*` debe llegar al backend para que los enlaces firmados funcionen fuera de la PC.
- Un fallo antes de obtener la URL no debe dejar un contenedor de túnel huérfano.
- Detener la demo debe restaurar `PUBLIC_BASE_URL` desde `.env` sin apagar la base ni borrar volúmenes.

---

### Task 1: URLs same-origin del navegador

**Files:**
- Create: `web/src/kernel/api/urls.ts`
- Create: `web/src/kernel/api/urls.test.ts`
- Modify: `web/src/kernel/api/client.ts`
- Modify: `web/src/kernel/api/useSocket.ts`

**Interfaces:**
- Produces: `resolveApiBaseUrl(configured?: string): string` y `resolveWebSocketBaseUrl(configured: string | undefined, location: Pick<Location, "protocol" | "host">): string`.
- Consumes: `NEXT_PUBLIC_API_URL=\"same-origin\"` y `NEXT_PUBLIC_WS_URL=\"same-origin\"` del override de demo.

- [ ] **Step 1: Escribir las pruebas fallidas**

  Exigir que API devuelva `""` en modo `same-origin`; que WebSocket derive
  `wss://demo.trycloudflare.com` desde HTTPS y `ws://localhost:8088` desde HTTP;
  y que valores explícitos conserven el comportamiento actual.

- [ ] **Step 2: Verificar RED**

  Run: `npm test -- src/kernel/api/urls.test.ts`
  Expected: FAIL porque los resolvers no existen.

- [ ] **Step 3: Implementar y conectar los resolvers**

  Mantener `http://localhost:8080` y `ws://localhost:8080` como defaults fuera
  del modo de demo. Resolver el WebSocket dentro del efecto, donde `window` ya
  existe.

- [ ] **Step 4: Verificar GREEN**

  Run: `npm test -- src/kernel/api/urls.test.ts src/kernel/api/client.test.ts src/kernel/api/useSocket.test.tsx`
  Expected: PASS.

### Task 2: Gateway de demo y override de Compose

**Files:**
- Create: `infra/demo/nginx.conf`
- Create: `docker-compose.demo.yml`

**Interfaces:**
- Consumes: servicios `web`, `backend` y red `backbone` del Compose principal.
- Produces: `demo-gateway:8080`, publicado como `127.0.0.1:${DEMO_GATEWAY_PORT:-8088}`.

- [ ] **Step 1: Crear el gateway**

  Enrutar `/api/` y `/files/` a `backend:8080`, incluida la negociación de
  WebSocket; enrutar `/` a `web:3000`; permitir adjuntos de hasta 32 MiB.

- [ ] **Step 2: Crear el override**

  Construir `web` con ambos valores `same-origin`; configurar en `backend`
  `PUBLIC_BASE_URL` y `CORS_ALLOWED_ORIGINS` desde `DEMO_PUBLIC_URL`; agregar el
  gateway con dependencias y publicación loopback.

- [ ] **Step 3: Validar la configuración antes de arrancar**

  Run: `docker compose -f docker-compose.yml -f docker-compose.demo.yml config`
  Expected: exit 0; ningún puerto nuevo de Postgres, Redis o Evolution.

- [ ] **Step 4: Validar Nginx y el gateway local**

  Run: `docker compose -f docker-compose.yml -f docker-compose.demo.yml up -d --build demo-gateway`

  Run: `Invoke-WebRequest http://127.0.0.1:8088/login -UseBasicParsing`
  Expected: HTTP 200.

### Task 3: Inicio y detención reproducibles

**Files:**
- Create: `scripts/start-demo.ps1`
- Create: `scripts/stop-demo.ps1`
- Modify: `README.md`

**Interfaces:**
- Produces: contenedor efímero exacto `wm-demo-tunnel` y muestra la URL `https://*.trycloudflare.com`.
- Consumes: Docker activo, red Compose `whatsapp-mirror_backbone` y gateway saludable.

- [ ] **Step 1: Implementar `start-demo.ps1`**

  Validar Docker; rechazar un `wm-demo-tunnel` ya existente; levantar el gateway;
  iniciar `cloudflare/cloudflared` conectado a la red interna; esperar y validar
  la URL; recrear sólo `backend` con `DEMO_PUBLIC_URL`; limpiar el contenedor del
  túnel si falla el arranque.

- [ ] **Step 2: Implementar `stop-demo.ps1`**

  Eliminar sólo `wm-demo-tunnel`, detener `demo-gateway` y recrear sólo
  `backend` con el Compose principal para restaurar las URLs de `.env`.

- [ ] **Step 3: Documentar el uso**

  Incluir comandos de inicio/detención, vida efímera de la URL y requisito de
  mantener PC, Docker y el script activos. Aclarar que la autenticación es el
  login del CRM.

- [ ] **Step 4: Ejecutar la demo real**

  Run: `powershell -ExecutionPolicy Bypass -File scripts/start-demo.ps1`
  Expected: URL HTTPS válida, `/login` responde 200 desde Internet y el login,
  una llamada autenticada, WebSocket y un archivo firmado funcionan.

- [ ] **Step 5: Verificación completa**

  Run: `npm test` y `npm run build` desde `web/`; `git diff --check`; inspección
  de `docker compose ps` y `docker port wm-demo-tunnel`.
  Expected: suite y build en verde; el túnel no publica puertos Docker y los
  servicios internos siguen privados.

