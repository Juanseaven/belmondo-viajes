# Live Mirror Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the appearance and inbox behavior of `/mirror?demo=1` into `/mirror`, backed only by real WhatsApp data and actions.

**Architecture:** Keep the current live controller (`LiveMirrorScreen`) and replace its page composition with the three-column demo layout. Add a denormalized `last_message_direction` field to conversations so the “Sin responder” tab is backed by Postgres and updates atomically during message ingestion. The demo remains an isolated reference during this vertical and is removed only after operations have a real backend.

**Tech Stack:** Go 1.23, pgx/PostgreSQL migrations, Next.js 16.3, React 19, TypeScript, Vitest, Testing Library, WebSocket.

**Spec:** Approved in chat on 2026-08-18; the user explicitly requested no separate design document.

## Global Constraints

- `/mirror` must never import, display, or fall back to demo contacts, phone numbers, conversations, operations, controls, or profiles.
- `/mirror?demo=1` remains unchanged as a temporary visual reference.
- Do not change the Evolution Go integration unless an observed event contract requires it; consult `docs/evolution-go/` first if that happens.
- Preserve URL state for `instance`, `c`, `m`, and `q`.
- Preserve optimistic idempotent sends, attachment sends, keyset pagination, mark-read behavior, and WebSocket reconciliation.
- Do not commit during this execution: the worktree contains pre-existing user changes in files this work must modify.

---

### Task 1: Persist and query the real “Sin responder” state

**Files:**
- Create: `server/internal/modules/mirror/migrations/0006_last_message_direction.sql`
- Modify: `server/internal/modules/mirror/store/read.go`
- Modify: `server/internal/modules/mirror/store/postgres/messages.go`
- Modify: `server/internal/modules/mirror/store/postgres/read.go`
- Modify: `server/internal/modules/mirror/api.go`
- Modify: `server/internal/modules/mirror/api_test.go`
- Modify: `server/internal/modules/mirror/store/postgres/messages_test.go`
- Modify: `server/internal/modules/mirror/store/postgres/read_integration_test.go`

**Interfaces:**
- Produces: `InboxItem.NeedsReply bool` serialized as `needsReply`.
- Produces: `InboxQuery.NeedsReply bool`.
- Produces: `GET /api/mirror/conversations?status=unanswered`.

- [ ] **Step 1: Write failing store and API tests**

Add an API assertion that `status=unanswered` sets `read.lastInbox.NeedsReply`, an integration test with one inbound-last and one outbound-last conversation, and a persistence test proving an older message cannot overwrite the latest direction.

```go
func TestFiltroSinResponderLlegaAlStore(t *testing.T) {
    read := &fakeRead{}
    m := newTestModule(read)
    do(m.listConversations, "/?status=unanswered", nil)
    if !read.lastInbox.NeedsReply { t.Fatal("se esperaba NeedsReply") }
}
```

- [ ] **Step 2: Run the focused tests and confirm failure**

Run: `go test ./internal/modules/mirror/... -run 'SinResponder|Direccion'`

Expected: compile failures because `NeedsReply` and `last_message_direction` do not exist.

- [ ] **Step 3: Add the schema and atomic projection**

The migration must add the nullable enum column, backfill it from the newest message, and add partial indexes for global and per-line unanswered queries.

```sql
ALTER TABLE conversations ADD COLUMN last_message_direction message_direction;

UPDATE conversations c
   SET last_message_direction = latest.direction
  FROM (
    SELECT DISTINCT ON (conversation_id) conversation_id, direction
      FROM messages
     ORDER BY conversation_id, wa_timestamp DESC, id DESC
  ) latest
 WHERE latest.conversation_id = c.id;
```

Update `bumpConv` so `last_message_direction` changes in the same timestamp guard as preview and type. Extend `InboxItem`, `InboxQuery`, `inboxColumns`, `scanInbox`, and `Inbox` with `NeedsReply`; when true append `c.last_message_direction = 'inbound'` to the SQL predicate. Parse only `status=unanswered` in the handler.

- [ ] **Step 4: Run backend tests**

Run: `go test ./internal/modules/mirror/...`

Expected: PASS; database integration tests may skip when `TEST_DATABASE_URL` is absent.

---

### Task 2: Carry the unanswered filter through the web client

**Files:**
- Modify: `web/src/modules/mirror/api.ts`
- Modify: `web/src/modules/mirror/useInbox.ts`
- Create: `web/src/modules/mirror/useInbox.test.tsx`

**Interfaces:**
- Consumes: `GET /api/mirror/conversations?status=unanswered` and `needsReply` from Task 1.
- Produces: `useInbox(instance, search, unanswered)`.

- [ ] **Step 1: Write failing hook tests**

Mock `mirrorApi.conversations`, render a small hook harness, and assert that selecting unanswered passes `{ status: "unanswered" }`, while the default omits it. Assert pagination repeats the same filter.

```tsx
render(<Harness unanswered />);
await waitFor(() => expect(mirrorApi.conversations).toHaveBeenCalledWith({
  instance: "", q: "", status: "unanswered",
}));
```

- [ ] **Step 2: Run the focused web test and confirm failure**

Run: `npm test -- useInbox.test.tsx`

Expected: FAIL because the API and hook do not accept `status`/`unanswered`.

- [ ] **Step 3: Extend the client contract**

Add `needsReply: boolean` to `Conversation`, allow `status?: "unanswered"` in `mirrorApi.conversations`, and thread `unanswered` through both `reload` and `loadMore`.

- [ ] **Step 4: Run the focused web test**

Run: `npm test -- useInbox.test.tsx`

Expected: PASS.

---

### Task 3: Move the demo inbox composition into the live screen

**Files:**
- Modify: `web/src/modules/mirror/MirrorScreen.tsx`
- Modify: `web/src/modules/mirror/Thread.tsx`
- Modify: `web/src/modules/mirror/ConversationList.tsx`
- Modify: `web/src/app/globals.css`
- Create: `web/src/modules/mirror/MirrorScreen.live.test.tsx`

**Interfaces:**
- Consumes: `useInbox(instance, search, unanswered)` from Task 2.
- Preserves: `Thread`/`Composer` real message loading and sending, `LineDialog`, `SearchResults`, URL state, and WebSocket handlers.
- Produces: the demo-style three-column composition for `/mirror`.

- [ ] **Step 1: Write the live-screen regression test**

Mock real API data with unmistakably non-demo names, render without `demo=1`, and assert the two tabs, line filter, real conversation, thread, and contact panel appear. Assert `Ana Pérez`, `#10482`, and `Nota de demostración` are absent.

```tsx
expect(screen.getByRole("tablist", { name: "Filtrar conversaciones" })).toBeTruthy();
expect(screen.getByText("Contacto real")).toBeTruthy();
expect(screen.queryByText("Ana Pérez")).toBeNull();
```

Click “Sin responder” and assert the inbox hook/API is called with the real filter. Open a conversation and assert `markRead` and URL navigation remain active.

- [ ] **Step 2: Run the live-screen test and confirm failure**

Run: `npm test -- MirrorScreen.live.test.tsx`

Expected: FAIL because the live screen still uses the old workspace header and has no inbox tabs.

- [ ] **Step 3: Replace only the live page composition**

Port the structural classes from `DemoMirrorScreen` into `LiveMirrorScreen`: line controls at the top of the inbox, `Todas`/`Sin responder` tabs, compact search row, thread in the center, and collapsible details on the right. Do not import `demoData`, `demoOperations`, `OperationBanner`, or `OperationPanel`.

Use the real values:

```ts
type InboxTab = "all" | "unanswered";
const inbox = useInbox(instance, query ? "" : searchValue, tab === "unanswered");
```

Keep search semantics: typing filters contacts; submitting the query opens existing message search results. Keep the admin-only real `LineDialog` action.

- [ ] **Step 4: Adapt live thread and row presentation**

Add the panel reopen control to `Thread` via explicit props (`detailsHidden`, `onShowDetails`) without adding operation concepts. Keep real avatar, phone, line status, message pagination, and `Composer`. In `ConversationList`, show real line and unread state only; do not synthesize commercial state chips.

- [ ] **Step 5: Run component tests**

Run: `npm test -- MirrorScreen.live.test.tsx ConversationList.test.tsx MirrorScreen.demo.test.tsx`

Expected: PASS, including the unchanged demo route.

---

### Task 4: Make the real details panel truthful and resilient

**Files:**
- Modify: `web/src/modules/mirror/ContactDetails.tsx`
- Modify: `web/src/modules/mirror/ContactDetails.test.tsx`
- Modify: `web/src/modules/mirror/MirrorScreen.live.test.tsx`
- Modify: `web/src/app/globals.css`

**Interfaces:**
- Consumes: real `Conversation` and `Line` only.
- Produces: a collapsible desktop panel and existing mobile sheet with no placeholder commercial content.

- [ ] **Step 1: Write failing truthfulness tests**

Assert the panel renders only the real contact name, type, phone, and active line; assert no “demostración”, fake note, email, operation, or control copy appears.

- [ ] **Step 2: Run the focused test and confirm failure**

Run: `npm test -- ContactDetails.test.tsx MirrorScreen.live.test.tsx`

Expected: FAIL on the current “Nota de demostración” block.

- [ ] **Step 3: Remove placeholder content and finish responsive behavior**

Delete the demo note from `ContactDetails`. Add a close callback for the desktop panel while keeping the sheet mode unchanged. Ensure the live grid switches between two and three columns without horizontal page scroll, and that the mobile back/details controls remain reachable.

- [ ] **Step 4: Run focused tests**

Run: `npm test -- ContactDetails.test.tsx MirrorScreen.live.test.tsx`

Expected: PASS.

---

### Task 5: Full verification

**Files:**
- Verify only; fix failures in the smallest owning file.

**Interfaces:**
- Produces: a buildable, tested real inbox with no demo-data leakage.

- [ ] **Step 1: Format backend files**

Run: `gofmt -w internal/modules/mirror/api.go internal/modules/mirror/store/read.go internal/modules/mirror/store/postgres/messages.go internal/modules/mirror/store/postgres/read.go internal/modules/mirror/api_test.go internal/modules/mirror/store/postgres/messages_test.go internal/modules/mirror/store/postgres/read_integration_test.go`

- [ ] **Step 2: Verify backend**

Run: `go test ./... && go vet ./... && go build ./...`

Expected: all commands exit 0.

- [ ] **Step 3: Verify frontend**

Run: `npm test && npm run typecheck && npm run build`

Expected: all commands exit 0.

- [ ] **Step 4: Inspect both routes**

Open `/mirror` with the running real stack and verify real lines, conversations, unread counts, unanswered filtering, reading, sending, attachments, details collapse, and WebSocket updates. Open `/mirror?demo=1` and verify it remains isolated and unchanged.

- [ ] **Step 5: Review the diff for fixture leakage**

Run: `rg -n "demoData|DEMO_|demoOperations|OperationBanner|OperationPanel" web/src/modules/mirror/MirrorScreen.tsx web/src/modules/mirror/Thread.tsx web/src/modules/mirror/ContactDetails.tsx`

Expected: no demo imports or fixture references in the live implementation; the only allowed `DemoMirrorScreen` reference is the explicit `demo=1` branch.
