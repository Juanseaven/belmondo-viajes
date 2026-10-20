# Módulo operaciones Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Terminar el módulo real de viajes dentro de `/mirror`, desde las escrituras y lecturas PostgreSQL hasta la API, tiempo real y UI sin datos demo.

**Architecture:** `operaciones` mantiene su dominio, store PostgreSQL y módulo HTTP propios; solo lee las tablas públicas de `mirror` y emite por el hub del kernel. El frontend compone datos de `mirror` y `operaciones` por `contactId`, conserva el expediente seleccionado en el hilo y resincroniza ante eventos websocket.

**Tech Stack:** Go 1.24, net/http, pgx/PostgreSQL, WebSocket del kernel, Next.js 16, React 19, TypeScript, Vitest y Testing Library.

**Spec:** `docs/superpowers/specs/2026-08-20-modulo-operaciones-design.md`

## Global Constraints

- El módulo `mirror` no recibe columnas ni migraciones nuevas.
- El alta de viaje conserva solo contacto, destino, salida y regreso como entrada obligatoria.
- Los controles derivados se recalculan después de toda escritura de datos relevante y pueden reabrirse.
- Los controles por acción requieren evidencia de un mensaje del mismo cliente.
- La interfaz usa “viaje”, no “operación”, y no incorpora servicios ni documentos en esta entrega.
- Todo endpoint vive bajo `/api/operaciones/` y exige sesión.

---

### Task 1: Escrituras de pasajeros, pagos y viaje

**Files:**
- Modify: `server/internal/modules/operaciones/store/store.go`
- Create: `server/internal/modules/operaciones/store/postgres/passengers.go`
- Create: `server/internal/modules/operaciones/store/postgres/payments.go`
- Modify: `server/internal/modules/operaciones/store/postgres/trips.go`
- Test: `server/internal/modules/operaciones/store/postgres/passengers_integration_test.go`
- Test: `server/internal/modules/operaciones/store/postgres/payments_integration_test.go`

**Interfaces:**
- Produces: `AddPassenger`, `UpdatePassenger`, `DeletePassenger`, `PutPayment`, `UpdateTrip` y los tipos `Passenger`, `Payment`, `TripPatch`.

- [ ] **Step 1: Write failing integration tests** for passenger add/update/delete, single-holder conflicts, derived close/reopen, document criticality, payment upsert, and due-date recalculation after a trip date change.
- [ ] **Step 2: Run the focused package tests** with `TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable` and confirm failures are missing-method/behavior failures.
- [ ] **Step 3: Implement the minimal transaction-backed methods**; every passenger/payment mutation calls `refreshDerived`, and passenger document mutations also recompute `control-vto-doc` criticality.
- [ ] **Step 4: Run the focused tests and all operaciones store tests** and confirm green.

### Task 2: Expediente y bandeja

**Files:**
- Modify: `server/internal/modules/operaciones/store/store.go`
- Create: `server/internal/modules/operaciones/store/postgres/read.go`
- Test: `server/internal/modules/operaciones/store/postgres/read_integration_test.go`

**Interfaces:**
- Produces: `TripsByContact(ctx, contactID) ([]TripSummary, error)`, `TripDetail(ctx, tripID) (TripDetail, error)` and `Inbox(ctx, contactIDs) (map[string]InboxTrip, error)`.

- [ ] **Step 1: Write failing integration tests** proving newest-open selection, closed-trip fallback, critical-first/due-first control selection, full passenger/payment/control/evidence hydration, and preservation of requested contacts without trips.
- [ ] **Step 2: Run tests and confirm the reads are absent.**
- [ ] **Step 3: Implement set-based PostgreSQL reads** with one inbox query and deterministic ordering; hydrate the full detail without N+1 queries.
- [ ] **Step 4: Run the store package tests and confirm green.**

### Task 3: Module, HTTP API, events and architecture rule

**Files:**
- Create: `server/internal/modules/operaciones/operaciones.go`
- Create: `server/internal/modules/operaciones/api.go`
- Test: `server/internal/modules/operaciones/api_test.go`
- Modify: `server/cmd/api/main.go`
- Modify: `server/internal/modules/module.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 1 and Task 2 store methods.
- Produces: the REST contract from the spec plus websocket events `trip.created`, `trip.updated`, `passenger.changed`, `payment.changed`, and `control.changed` with `{tripId, contactIds}` payloads.

- [ ] **Step 1: Write failing handler tests** for route registration, JSON/date/decimal decoding, validation, not-found/conflict mapping, authenticated user attribution, successful response shapes, and one websocket broadcast per committed write.
- [ ] **Step 2: Run the focused handler tests and confirm failures.**
- [ ] **Step 3: Implement the embedded-migration module and thin handlers**, register it after `mirror.New(k)`, and update the documented one-way module dependency rule.
- [ ] **Step 4: Run `go test ./...`, `go vet ./...`, and `go build ./...`.**

### Task 4: Frontend client and live state

**Files:**
- Create: `web/src/modules/operaciones/api.ts`
- Create: `web/src/modules/operaciones/useTrips.ts`
- Test: `web/src/modules/operaciones/useTrips.test.tsx`
- Modify: `web/src/modules/mirror/MirrorScreen.tsx`
- Test: `web/src/modules/mirror/MirrorScreen.live.test.tsx`

**Interfaces:**
- Produces: typed `operacionesApi`, inbox enrichment by `contactId`, selected trip detail, mutations with REST revalidation, and websocket refresh for the affected contact/trip.

- [ ] **Step 1: Write failing component/hook tests** for inbox enrichment, newest-open selection, creating the first trip, switching trips, websocket refresh, and action controls only preloading the composer.
- [ ] **Step 2: Run `npm test -- useTrips.test.tsx MirrorScreen.live.test.tsx` and confirm failures.**
- [ ] **Step 3: Implement the typed API and state hook**, then compose it into the existing real screen without changing mirror's API.
- [ ] **Step 4: Run the focused frontend tests and confirm green.**

### Task 5: Real banner, panel and row chip

**Files:**
- Create: `web/src/modules/operaciones/types.ts`
- Create: `web/src/modules/operaciones/TripBanner.tsx`
- Create: `web/src/modules/operaciones/TripPanel.tsx`
- Create: `web/src/modules/operaciones/TripChip.tsx`
- Test: `web/src/modules/operaciones/TripPanel.test.tsx`
- Test: `web/src/modules/operaciones/TripChip.test.tsx`
- Modify: `web/src/app/globals.css`

**Interfaces:**
- Consumes: Task 4's API types and mutation callbacks.
- Produces: banner with active trip summary, four tabs (`Resumen`, `Pasajeros`, `Pagos`, `Controles`), explicit empty-state passenger CTA, critical-first “Lo que sigue”, and a chip naming the overdue control.

- [ ] **Step 1: Write failing component tests** for critical-first ordering, collapsed remainder, passenger empty state, explicit overdue label, payment summary, manual close/reopen, and composer preload for actions.
- [ ] **Step 2: Run focused tests and confirm failures.**
- [ ] **Step 3: Implement accessible components using existing tokens and responsive shell constraints.**
- [ ] **Step 4: Run component tests, typecheck and build.**

### Task 6: Remove demo and verify end to end

**Files:**
- Delete: `web/src/modules/mirror/demoData.ts`
- Delete: `web/src/modules/mirror/demoOperations.ts`
- Delete: `web/src/modules/mirror/DemoMirrorScreen.tsx`
- Delete: `web/src/modules/mirror/MirrorScreen.demo.test.tsx`
- Modify: remaining imports/tests that mention `demo=1` or operation demo types.

**Interfaces:**
- Produces: `/mirror` has one real data path and no query-param demo branch.

- [ ] **Step 1: Add/adjust a failing real-screen regression test** proving `demo=1` no longer changes data flow.
- [ ] **Step 2: Run the regression test and confirm it fails while the demo branch exists.**
- [ ] **Step 3: Remove demo files, imports and branch; migrate reusable presentation code into `operaciones`.**
- [ ] **Step 4: Run `go build ./...`, `go vet ./...`, `go test ./...`, `npm test`, `npm run typecheck`, and `npm run build`; inspect `/mirror` at desktop and mobile widths for page-level horizontal overflow.**
