# Bandeja profesional con shadcn Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reemplazar la interfaz provisional de `/mirror` por una bandeja profesional basada en shadcn, conservando todos los flujos existentes.

**Architecture:** La pantalla seguirá usando `MirrorScreen` como coordinador de URL, API y WebSocket. Los componentes de dominio recibirán datos y callbacks, mientras que las primitivas accesibles de shadcn vivirán en `src/components/ui`; el rediseño no agregará endpoints ni estado persistente nuevo.

**Tech Stack:** Next.js 16, React 19, TypeScript, Tailwind CSS 4, shadcn/ui, Radix UI, Lucide React, Vitest y Testing Library.

## Global Constraints

- Mantener las APIs, los hooks, la paginación y los eventos WebSocket actuales.
- Usar `#0B1220`, `#F5F7FB`, `#FFFFFF`, `#E2E8F0`, `#146EF5` y `#20B15A` como base cromática.
- Usar Manrope para títulos y navegación, e Inter para contenido y controles; ambas fuentes deben instalarse localmente.
- No agregar etiquetas, persistencia de notas, sincronización de fotos, endpoints ni acciones decorativas.
- Usar `avatarUrl` si existe e iniciales si no existe o la imagen falla.
- Mantener visible la procedencia de línea en lista, hilo y detalles.
- Evitar scroll general y overflow horizontal en 1366×768.
- Preservar cambios preexistentes del working tree; durante esta ejecución no se harán commits de archivos que ya estaban modificados.

---

### Task 1: Base shadcn, tokens y avatar de contacto

**Files:**
- Create: `web/components.json`
- Create: `web/postcss.config.mjs`
- Create: `web/src/lib/utils.ts`
- Create: `web/src/components/ui/button.tsx`
- Create: `web/src/components/ui/avatar.tsx`
- Create: `web/src/components/ui/badge.tsx`
- Create: `web/src/components/ui/input.tsx`
- Create: `web/src/components/ui/textarea.tsx`
- Create: `web/src/components/ui/separator.tsx`
- Create: `web/src/components/ui/skeleton.tsx`
- Create: `web/src/components/ui/alert.tsx`
- Create: `web/src/modules/mirror/ContactAvatar.tsx`
- Create: `web/src/modules/mirror/ContactAvatar.test.tsx`
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Modify: `web/tsconfig.json`
- Modify: `web/src/app/layout.tsx`
- Modify: `web/src/app/globals.css`

**Interfaces:**
- Produces: `ContactAvatar({ name, src, size?, className? })` y primitivas UI reutilizables.
- Consumes: nombre y `avatarUrl` opcional de `Conversation`.

- [ ] **Step 1: Write the failing avatar behavior tests**

```tsx
render(<ContactAvatar name="Carla Mendoza" />)
expect(screen.getByText("CM")).toBeInTheDocument()

render(<ContactAvatar name="Carla Mendoza" src="https://example.test/carla.jpg" />)
expect(screen.getByRole("img", { name: "Carla Mendoza" })).toHaveAttribute(
  "src",
  "https://example.test/carla.jpg",
)
```

- [ ] **Step 2: Run the test and verify RED**

Run: `npm test -- ContactAvatar.test.tsx`

Expected: FAIL because `ContactAvatar` does not exist.

- [ ] **Step 3: Install and initialize the design-system dependencies**

Run from `web`:

```powershell
npm install tailwindcss @tailwindcss/postcss shadcn class-variance-authority clsx tailwind-merge lucide-react tw-animate-css @fontsource-variable/inter @fontsource-variable/manrope
npx shadcn@latest init
npx shadcn@latest add button avatar badge input textarea separator skeleton alert
```

Configure the `@/* -> ./src/*` alias and the approved tokens in
`globals.css`. Import the local variable fonts from `layout.tsx` and expose
`--font-heading` and `--font-sans`.

Use this PostCSS boundary:

```js
export default {
  plugins: {
    "@tailwindcss/postcss": {},
  },
}
```

and add this exact TypeScript alias:

```json
"baseUrl": ".",
"paths": { "@/*": ["./src/*"] }
```

- [ ] **Step 4: Implement `ContactAvatar` minimally**

Use `Avatar`, `AvatarImage` and `AvatarFallback`; derive at most two uppercase
initials from non-empty name segments and use `?` for an empty name. Set the
image alt to the complete contact name.

- [ ] **Step 5: Verify GREEN and the build boundary**

Run:

```powershell
npm test -- ContactAvatar.test.tsx
npm run typecheck
```

Expected: both commands exit 0.

### Task 2: Shell, selector de líneas y bandeja

**Files:**
- Create: `web/src/components/ui/dropdown-menu.tsx`
- Create: `web/src/components/ui/dialog.tsx`
- Create: `web/src/components/ui/scroll-area.tsx`
- Create: `web/src/components/ui/tooltip.tsx`
- Modify: `web/src/kernel/shell/Shell.tsx`
- Modify: `web/src/modules/mirror/LineFilter.tsx`
- Modify: `web/src/modules/mirror/LineFilter.test.tsx`
- Modify: `web/src/modules/mirror/ConversationList.tsx`
- Create: `web/src/modules/mirror/ConversationList.test.tsx`
- Modify: `web/src/modules/mirror/MirrorScreen.tsx`
- Modify: `web/src/modules/mirror/LineDialog.tsx`

**Interfaces:**
- Consumes: `Line[]`, `Conversation[]`, callbacks de selección, conexión y desvinculación.
- Produces: selector shadcn accesible, filas compactas y búsqueda unificada.

- [ ] **Step 1: Write failing tests for operational behavior**

Add tests proving that:

```tsx
expect(screen.getByRole("button", { name: /todas las líneas/i })).toBeInTheDocument()
await user.click(screen.getByRole("button", { name: /todas las líneas/i }))
await user.click(screen.getByRole("menuitemradio", { name: /línea norte/i }))
expect(onSelect).toHaveBeenCalledWith("linea-1")
```

and that a conversation row exposes initials, unread count and line origin:

```tsx
expect(screen.getByText("CM")).toBeInTheDocument()
expect(screen.getByText("3")).toBeInTheDocument()
expect(screen.getByText("Línea Norte")).toBeInTheDocument()
```

Keep the existing destructive confirmation assertions.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `npm test -- LineFilter.test.tsx ConversationList.test.tsx`

Expected: FAIL because the dropdown selector and avatar row are absent.

- [ ] **Step 3: Generate interactive shadcn primitives**

Run from `web`:

```powershell
npx shadcn@latest add dropdown-menu dialog scroll-area tooltip
```

- [ ] **Step 4: Implement the shell and inbox composition**

Replace emoji controls with Lucide icons. Render a 228 px navy navigation rail,
the page header, the current-line dropdown and the primary connection button.
Convert the disconnect confirmation and QR flow to `Dialog` without changing
their callbacks.

Render conversation rows with `ContactAvatar`, a one-line preview, time, unread
badge and line origin. Use one search input: update `contactFilter` on change and
write `q` to the URL on Enter. Keep the existing `SearchResults` path when `q`
is present. Replace initial loading copy with stable `Skeleton` rows and render
the existing fetch error inside `Alert` with its real “Reintentar” callback.

- [ ] **Step 5: Verify GREEN**

Run: `npm test -- LineFilter.test.tsx ConversationList.test.tsx`

Expected: all focused tests pass.

### Task 3: Hilo, compositor y panel de detalles

**Files:**
- Create: `web/src/components/ui/sheet.tsx`
- Create: `web/src/modules/mirror/ContactDetails.tsx`
- Create: `web/src/modules/mirror/ContactDetails.test.tsx`
- Modify: `web/src/modules/mirror/Thread.tsx`
- Modify: `web/src/modules/mirror/MessageBubble.tsx`
- Modify: `web/src/modules/mirror/Composer.tsx`
- Modify: `web/src/modules/mirror/MirrorScreen.tsx`

**Interfaces:**
- Produces: `ContactDetails({ conversation, line, mode })`, donde `mode` es `"panel" | "sheet"`.
- Consumes: conversación abierta, línea correspondiente y nota local de demostración.

- [ ] **Step 1: Write failing detail-panel tests**

```tsx
render(<ContactDetails conversation={conversation} line={line} mode="panel" />)
expect(screen.getByText("Carla Mendoza")).toBeInTheDocument()
expect(screen.getByText("+54 9 11 3456-7890")).toBeInTheDocument()
expect(screen.getByText("Línea Norte")).toBeInTheDocument()
expect(screen.getByText(/nota de demostración/i)).toBeInTheDocument()
expect(screen.queryByText(/etiquetas/i)).not.toBeInTheDocument()
```

Also assert that no details content renders when `conversation` is null.

- [ ] **Step 2: Run the test and verify RED**

Run: `npm test -- ContactDetails.test.tsx`

Expected: FAIL because `ContactDetails` does not exist.

- [ ] **Step 3: Generate the responsive sheet**

Run from `web`: `npx shadcn@latest add sheet`

- [ ] **Step 4: Implement thread, messages, composer and details**

Use a fixed thread header with avatar, phone and line origin. Preserve message
direction, author, pending, failure, deletion and media behavior. Restyle the
composer with shadcn controls and Lucide icons while keeping Enter, Shift+Enter,
attachments and optimistic callbacks unchanged.

Render the details column above 1180 px and a `Sheet` trigger between 768 px and
1179 px. Its sections are contact, phone, current line and a visibly non-editable
note labeled “Nota de demostración”. Do not render labels or invented location.

- [ ] **Step 5: Verify GREEN**

Run:

```powershell
npm test -- ContactDetails.test.tsx
npm test
```

Expected: all tests pass.

### Task 4: Responsive closure and production verification

**Files:**
- Modify: `web/src/app/globals.css`
- Modify: any files from Tasks 1–3 only when visual inspection exposes a concrete defect.

**Interfaces:**
- Consumes: completed shell, inbox, thread, details and dialog components.
- Produces: deployable `/mirror` layout with no page-level overflow.

- [ ] **Step 1: Add a failing regression test for empty operational state**

Render `ConversationList` with an empty inbox and assert the UI gives the operator
a specific next step rather than only blank space:

```tsx
expect(screen.getByText("No hay conversaciones todavía.")).toBeInTheDocument()
expect(screen.getByText(/conectá una línea/i)).toBeInTheDocument()
```

- [ ] **Step 2: Run the test and verify RED**

Run: `npm test -- ConversationList.test.tsx`

Expected: FAIL because the current empty state lacks the actionable instruction.

- [ ] **Step 3: Implement the empty state and responsive constraints**

Add the actionable empty copy. Ensure `.shell`, the mirror workspace and each
scroll area use `min-width: 0` and `min-height: 0`; constrain horizontal overflow
to internal content only. Below 768 px, render the list when `c` is absent and
the thread with a “Volver a conversaciones” control when `c` is present; never
place both columns side by side.

- [ ] **Step 4: Run full automated verification**

Run from `web`:

```powershell
npm test
npm run typecheck
npm run build
```

Expected: every command exits 0 with no failed tests.

- [ ] **Step 5: Verify the running application visually**

Open `/mirror` and inspect 1440×900, 1366×768, 1024×768 and a mobile viewport.
Confirm focus visibility, independent scrolling, no full-page horizontal scroll,
closed dialogs rendering no layout space, and the composer remaining visible.

- [ ] **Step 6: Rebuild the frontend container if Docker serves the active UI**

Run from repository root: `docker compose up -d --build web`

Confirm the frontend and backend containers remain healthy and `/mirror` loads.
