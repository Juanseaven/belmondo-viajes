# Editable Draft Operation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mostrar una ficha de operación desde el primer momento y persistir un borrador recién al guardar el primer dato, sin exigir destino ni fechas.

**Architecture:** PostgreSQL y el contrato Go representarán destino y fechas faltantes como valores nulos. Los paneles React compartirán formularios editables y `MirrorScreen` orquestará la creación diferida antes de guardar datos hijos. Los controles seguirán naciendo con el viaje, pero sólo recibirán vencimiento cuando exista su fecha base.

**Tech Stack:** Go 1.24, PostgreSQL/pgx, Next.js/React/TypeScript, Vitest Testing Library, Docker Compose.

**Spec:** `docs/superpowers/plans/2026-08-21-editable-draft-operation-design.md`

## Global Constraints

- Abrir una conversación no debe crear filas.
- El primer guardado debe crear la operación y conservar el dato ingresado.
- Destino, salida y regreso son independientes y opcionales.
- No se deben perder ni revertir cambios existentes del árbol de trabajo.

---

### Task 1: Contrato de viaje parcial

**Files:**
- Create: `server/internal/modules/operaciones/migrations/0002_trip_drafts.sql`
- Modify: `server/internal/modules/operaciones/api.go`
- Modify: `server/internal/modules/operaciones/store/store.go`
- Modify: `server/internal/modules/operaciones/store/postgres/trips.go`
- Modify: `server/internal/modules/operaciones/store/postgres/read.go`
- Modify: `server/internal/modules/operaciones/store/postgres/derivados.go`
- Test: `server/internal/modules/operaciones/api_test.go`
- Test: `server/internal/modules/operaciones/store/postgres/trips_integration_test.go`

**Interfaces:**
- Consumes: `POST /api/operaciones/viajes` con `contactId` y cero o más datos de viaje.
- Produces: `Trip` con `departAt` y `returnAt` opcionales; `destination` vacío cuando aún no fue cargado.

- [ ] Escribir pruebas HTTP y de PostgreSQL que creen un viaje sin destino ni fechas y verifiquen fechas nulas y controles sin vencimiento dependiente.
- [ ] Ejecutar las pruebas y comprobar que fallan por las restricciones actuales.
- [ ] Agregar la migración y adaptar tipos, parseo, escrituras, lecturas y cálculo derivado.
- [ ] Ejecutar las pruebas del módulo y comprobar que pasan.

### Task 2: Ficha nueva editable y creación diferida

**Files:**
- Modify: `web/src/modules/operaciones/NewTripPanel.tsx`
- Create: `web/src/modules/operaciones/NewTripPanel.test.tsx`
- Modify: `web/src/modules/operaciones/api.ts`
- Modify: `web/src/modules/operaciones/types.ts`
- Modify: `web/src/modules/mirror/MirrorScreen.tsx`
- Modify: `web/src/modules/mirror/MirrorScreen.live.test.tsx`
- Modify: `web/src/app/globals.css`

**Interfaces:**
- Consumes: callbacks de creación parcial, alta de pasajero y guardado de pago.
- Produces: ficha con pestañas `Resumen`, `Pasajeros`, `Pagos`, `Controles` y guardado diferido.

- [ ] Escribir pruebas del panel que verifiquen que renderizar no crea, que guardar destino crea una vez y que el primer pasajero se conserva.
- [ ] Ejecutar las pruebas y comprobar la falla esperada contra el alta bloqueante.
- [ ] Implementar la ficha, estados de guardado/error y orquestación de `MirrorScreen`.
- [ ] Ejecutar las pruebas focalizadas y comprobar que pasan.

### Task 3: Edición de borradores existentes

**Files:**
- Modify: `web/src/modules/operaciones/TripPanel.tsx`
- Modify: `web/src/modules/operaciones/TripPanel.test.tsx`
- Modify: `web/src/modules/operaciones/TripBanner.tsx`
- Modify: `web/src/modules/operaciones/TripChip.tsx`

**Interfaces:**
- Consumes: `onUpdateTrip` y viajes con campos opcionales.
- Produces: resumen editable y representaciones compactas seguras ante datos faltantes.

- [ ] Escribir pruebas para editar un destino faltante y renderizar fechas incompletas sin valores técnicos.
- [ ] Ejecutar las pruebas y comprobar la falla esperada.
- [ ] Implementar edición inline y textos `Sin completar`.
- [ ] Ejecutar las pruebas focalizadas y comprobar que pasan.

### Task 4: Verificación integrada

**Files:**
- Verify only.

**Interfaces:**
- Consumes: backend, frontend y migración terminados.
- Produces: evidencia fresca de pruebas, tipos, build y estado de servicios.

- [ ] Ejecutar `go test ./...` en `server`.
- [ ] Ejecutar la suite web y el chequeo de tipos.
- [ ] Ejecutar la integración PostgreSQL del módulo con la base de Docker.
- [ ] Revisar el diff y confirmar cada requisito del diseño.

