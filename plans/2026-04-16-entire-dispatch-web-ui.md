# Entire Dispatch — Web UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. For any visual/styling work, invoke the `frontend-design` skill per the project's CLAUDE.md convention.

**Goal:** Ship the entire.io web UI for dispatches: detail page at `/dispatches/:id`, per-repo index at `/gh/:org/:repo/dispatches`, per-org index at `/orgs/:org/dispatches`, and create form at `/dispatches/new`. All pages consume the backend API built in the companion backend plan.

**Architecture:** New frontend domain `frontend/src/domains/platform/dispatches/` following the existing `checkpoints` domain structure. Four pages (Detail, PerRepo, PerOrg, New) wired through TanStack Router. No personal feed, no DELETE, no creator display — dispatches are shared cache objects per the spec. Styling and polish delegated to `frontend-design` skill at implementation time.

**Tech Stack:** React 19, TanStack Router, TanStack Query (if used by the project — inspect existing `domains/platform/checkpoints/hooks`), existing entire.io design tokens and components, `vitest` + `@testing-library/react` for unit tests.

**Companion spec:** [`specs/2026-04-16-entire-dispatch-web-ui-design.md`](../specs/2026-04-16-entire-dispatch-web-ui-design.md). **Depends on the backend plan** for every API call. Cannot ship before the backend endpoints are live.

**Worktree:** Same worktree as the backend plan — a new git worktree branched off `analysis-chunk-merge` at `/Users/alisha/Projects/wt/entire.io/dispatch-backend` (or a separate `dispatch-web-ui` worktree if the backend is already merged and published; check coordination with backend engineer).

---

## File Structure

```
frontend/src/domains/platform/dispatches/
  index.ts
  api.ts                                      # fetch wrappers for /api/v1/...
  api.test.ts
  routeConfig.ts                              # TanStack route config
  breadcrumbs.ts
  breadcrumbs.test.ts
  types.ts                                    # DispatchResponse, CreateRequest, etc.
  components/
    DispatchDetail.tsx
    DispatchDetail.test.tsx
    DispatchSidebar.tsx
    DispatchSidebar.test.tsx
    DispatchCard.tsx
    DispatchCard.test.tsx
    DispatchFilterBar.tsx
    DispatchFilterBar.test.tsx
    DispatchForm.tsx
    DispatchForm.test.tsx
    DispatchPreviewPane.tsx
    DispatchPreviewPane.test.tsx
    VoicePicker.tsx
    VoicePicker.test.tsx
    OpenInCliModal.tsx
    OpenInCliModal.test.tsx
  hooks/
    useDispatch.ts                            # single dispatch with polling on status:generating
    useRepoDispatches.ts
    useOrgDispatches.ts
    useCreateDispatch.ts
  pages/
    DispatchDetailPage.tsx
    DispatchDetailPage.test.tsx
    DispatchPerRepoPage.tsx
    DispatchPerRepoPage.test.tsx
    DispatchPerOrgPage.tsx
    DispatchPerOrgPage.test.tsx
    DispatchNewPage.tsx
    DispatchNewPage.test.tsx

frontend/src/routes/
  _authenticated/
    dispatches.new.tsx                        # → DispatchNewPage
    dispatches.$dispatchId.tsx                # → DispatchDetailPage
    orgs.$org.dispatches.index.tsx            # → DispatchPerOrgPage
  _repo/
    gh/
      $org.$repo.dispatches.index.tsx         # → DispatchPerRepoPage
```

---

## Task 1: Scaffold the domain + types

**Files:**
- Create: `frontend/src/domains/platform/dispatches/index.ts`
- Create: `frontend/src/domains/platform/dispatches/types.ts`
- Create: `frontend/src/domains/platform/dispatches/routeConfig.ts`
- Create: `frontend/src/domains/platform/dispatches/breadcrumbs.ts`

- [ ] **Step 1.1: Write types matching the backend response shapes**

```ts
// frontend/src/domains/platform/dispatches/types.ts
export type DispatchStatus = "complete" | "generating" | "failed"

export interface DispatchWindow {
  normalized_since: string
  normalized_until: string
  first_checkpoint_created_at: string | null
  last_checkpoint_created_at: string | null
}

export interface DispatchBullet {
  checkpoint_id: string
  text: string
  source: "cloud_analysis" | "local_summary" | "commit_message"
  branch: string
  created_at: string
  labels: string[]
}

export interface DispatchSection {
  label: string
  bullets: DispatchBullet[]
}

export interface DispatchRepo {
  full_name: string
  sections: DispatchSection[]
}

export interface DispatchTotals {
  checkpoints: number
  used_checkpoint_count: number
  branches: number
  files_touched: number
}

export interface DispatchWarnings {
  access_denied_count: number
  pending_count: number
  failed_count: number
  unknown_count: number
  uncategorized_count: number
}

// Persisted response — only for generate:true, dry_run:false
export interface PersistedDispatchResponse {
  id: string
  status: DispatchStatus
  fingerprint_hash: string
  deduped: boolean
  web_url: string
  window: DispatchWindow
  covered_repos: string[]
  repos: DispatchRepo[]
  totals: DispatchTotals
  warnings: DispatchWarnings
  generated_text: string | null
}

// Bullets-only response (generate:false, dry_run:false)
export interface BulletsDispatchResponse {
  generate: false
  window: DispatchWindow
  repos: DispatchRepo[]
  totals: DispatchTotals
  warnings: DispatchWarnings
}

// Preview / dry-run response
export interface DryRunDispatchResponse {
  dry_run: true
  requested_generate: boolean
  window: DispatchWindow
  repos: DispatchRepo[]
  totals: DispatchTotals
  warnings: DispatchWarnings
}

export type CreateDispatchResponse = PersistedDispatchResponse | BulletsDispatchResponse | DryRunDispatchResponse

export interface CreateDispatchRequest {
  repo?: string
  org?: string
  since: string
  until: string
  branches: string[] | "all"
  generate: boolean
  voice?: string
  dry_run?: boolean
}
```

- [ ] **Step 1.2: Commit**

```bash
git add frontend/src/domains/platform/dispatches/types.ts frontend/src/domains/platform/dispatches/index.ts
git commit -m "dispatches: scaffold domain with response types"
```

---

## Task 2: API client

**Files:**
- Create: `frontend/src/domains/platform/dispatches/api.ts`
- Create: `frontend/src/domains/platform/dispatches/api.test.ts`

- [ ] **Step 2.1: Failing tests**

```ts
// frontend/src/domains/platform/dispatches/api.test.ts
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { getDispatch, createDispatch, listRepoDispatches, listOrgDispatches } from "./api"

describe("dispatch API client", () => {
  const fetchMock = vi.fn()
  beforeEach(() => { (global as any).fetch = fetchMock; fetchMock.mockReset() })
  afterEach(() => { vi.restoreAllMocks() })

  it("getDispatch issues a GET with the id", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ id: "dsp_1", status: "complete" }), { status: 200 }))
    const r = await getDispatch("dsp_1")
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/dispatches/dsp_1", expect.any(Object))
    expect(r.id).toBe("dsp_1")
  })

  it("getDispatch surfaces 404 as an error", async () => {
    fetchMock.mockResolvedValue(new Response("", { status: 404 }))
    await expect(getDispatch("dsp_missing")).rejects.toThrow(/not found|404/i)
  })

  it("createDispatch POSTs with the correct body for generate:true", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ id: "dsp_2", status: "complete" }), { status: 201 }))
    const body = { repo: "entireio/cli", since: "2026-04-09T00:00:00Z", until: "2026-04-16T00:00:00Z", branches: "all" as const, generate: true, voice: "neutral" }
    await createDispatch(body)
    const [, init] = fetchMock.mock.calls[0]
    expect(init.method).toBe("POST")
    const sent = JSON.parse(init.body as string)
    expect(sent).toMatchObject(body)
  })

  it("createDispatch handles the dry-run preview shape", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ dry_run: true, requested_generate: true, window: {}, repos: [], totals: {}, warnings: {} }), { status: 200 }))
    const r = await createDispatch({ repo: "x/y", since: "s", until: "u", branches: "all", generate: true, dry_run: true }) as any
    expect(r.dry_run).toBe(true)
    expect((r as any).id).toBeUndefined()
  })

  it("listRepoDispatches calls the per-repo endpoint with query params", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ items: [], next_cursor: null }), { status: 200 }))
    await listRepoDispatches({ org: "entireio", repo: "cli", since: "2026-04-09T00:00:00Z", voice: "neutral", limit: 20 })
    const [url] = fetchMock.mock.calls[0]
    expect(url).toContain("/api/v1/repos/entireio/cli/dispatches")
    expect(url).toContain("since=2026-04-09T00%3A00%3A00Z")
    expect(url).toContain("voice=neutral")
    expect(url).toContain("limit=20")
  })
})
```

- [ ] **Step 2.2: Fail + implement**

```ts
// frontend/src/domains/platform/dispatches/api.ts
import type { CreateDispatchRequest, CreateDispatchResponse, PersistedDispatchResponse } from "./types"

async function req<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, { credentials: "include", ...init, headers: { "Content-Type": "application/json", ...(init.headers ?? {}) } })
  if (res.status === 404) throw new Error("not found")
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return (await res.json()) as T
}

export async function getDispatch(id: string): Promise<PersistedDispatchResponse> {
  return req<PersistedDispatchResponse>(`/api/v1/dispatches/${encodeURIComponent(id)}`)
}

export async function createDispatch(body: CreateDispatchRequest): Promise<CreateDispatchResponse> {
  return req<CreateDispatchResponse>(`/api/v1/users/me/dispatches`, { method: "POST", body: JSON.stringify(body) })
}

export interface ListResponse { items: PersistedDispatchResponse[]; next_cursor: string | null }

export async function listRepoDispatches(args: { org: string; repo: string; since?: string; until?: string; voice?: string; cursor?: string; limit?: number }): Promise<ListResponse> {
  const qs = new URLSearchParams()
  if (args.since) qs.set("since", args.since)
  if (args.until) qs.set("until", args.until)
  if (args.voice) qs.set("voice", args.voice)
  if (args.cursor) qs.set("cursor", args.cursor)
  if (args.limit) qs.set("limit", String(args.limit))
  return req<ListResponse>(`/api/v1/repos/${encodeURIComponent(args.org)}/${encodeURIComponent(args.repo)}/dispatches?${qs.toString()}`)
}

export async function listOrgDispatches(args: { org: string; since?: string; until?: string; voice?: string; cursor?: string; limit?: number }): Promise<ListResponse> {
  const qs = new URLSearchParams()
  if (args.since) qs.set("since", args.since)
  if (args.until) qs.set("until", args.until)
  if (args.voice) qs.set("voice", args.voice)
  if (args.cursor) qs.set("cursor", args.cursor)
  if (args.limit) qs.set("limit", String(args.limit))
  return req<ListResponse>(`/api/v1/orgs/${encodeURIComponent(args.org)}/dispatches?${qs.toString()}`)
}
```

- [ ] **Step 2.3: Pass + commit**

```bash
pnpm --filter frontend test src/domains/platform/dispatches/api.test.ts
git add frontend/src/domains/platform/dispatches/api.ts frontend/src/domains/platform/dispatches/api.test.ts
git commit -m "dispatches: api client for detail/list/create"
```

---

## Task 3: `useDispatch` hook with polling

**Files:**
- Create: `frontend/src/domains/platform/dispatches/hooks/useDispatch.ts`
- Create: `frontend/src/domains/platform/dispatches/hooks/useDispatch.test.tsx`

- [ ] **Step 3.1: Failing test — hook resolves complete dispatch**

```tsx
// useDispatch.test.tsx
import { renderHook, waitFor } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { useDispatch } from "./useDispatch"

const client = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
const wrap = (qc = client()) => ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider client={qc}>{children}</QueryClientProvider>
)

describe("useDispatch", () => {
  beforeEach(() => { (global as any).fetch = vi.fn() })

  it("returns the dispatch on a single poll when status is complete", async () => {
    (global as any).fetch.mockResolvedValue(new Response(JSON.stringify({ id: "dsp_a", status: "complete" }), { status: 200 }))
    const { result } = renderHook(() => useDispatch("dsp_a"), { wrapper: wrap() })
    await waitFor(() => expect(result.current.data?.status).toBe("complete"))
  })

  it("polls until status transitions to complete", async () => {
    let call = 0
    ;(global as any).fetch.mockImplementation(async () => {
      call++
      const status = call < 3 ? "generating" : "complete"
      return new Response(JSON.stringify({ id: "dsp_b", status }), { status: 200 })
    })
    const { result } = renderHook(() => useDispatch("dsp_b", { pollIntervalMs: 10 }), { wrapper: wrap() })
    await waitFor(() => expect(result.current.data?.status).toBe("complete"), { timeout: 1000 })
  })
})
```

- [ ] **Step 3.2: Implement**

```ts
// useDispatch.ts
import { useQuery } from "@tanstack/react-query"
import { getDispatch } from "../api"

export function useDispatch(id: string, opts?: { pollIntervalMs?: number }) {
  return useQuery({
    queryKey: ["dispatch", id],
    queryFn: () => getDispatch(id),
    refetchInterval: (q) => q.state.data?.status === "generating" ? (opts?.pollIntervalMs ?? 2000) : false,
  })
}
```

- [ ] **Step 3.3: Pass + commit**

---

## Task 4: `useRepoDispatches` and `useOrgDispatches`

**Files:**
- Create: `frontend/src/domains/platform/dispatches/hooks/useRepoDispatches.ts`
- Create: `frontend/src/domains/platform/dispatches/hooks/useOrgDispatches.ts`
- Tests accompany.

- [ ] **Step 4.1: Implement as TanStack infinite queries; failing tests + implement + commit**

```ts
// useRepoDispatches.ts
import { useInfiniteQuery } from "@tanstack/react-query"
import { listRepoDispatches } from "../api"

export function useRepoDispatches(args: { org: string; repo: string; since?: string; until?: string; voice?: string }) {
  return useInfiniteQuery({
    queryKey: ["dispatches", "repo", args],
    queryFn: ({ pageParam }) => listRepoDispatches({ ...args, cursor: pageParam as string | undefined, limit: 20 }),
    initialPageParam: undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  })
}
```

Mirror for `useOrgDispatches`. Commit: `dispatches: listing hooks`.

---

## Task 5: `useCreateDispatch` mutation (handles all three response shapes)

**Files:**
- Create: `frontend/src/domains/platform/dispatches/hooks/useCreateDispatch.ts`
- Create: `frontend/src/domains/platform/dispatches/hooks/useCreateDispatch.test.tsx`

- [ ] **Step 5.1: Test all three response branches**

```tsx
// useCreateDispatch.test.tsx — assert persisted vs bullets-only vs dry-run branches
// generate:true  → returns `id`
// generate:false → returns { generate: false, ... } and consumer code sees no `id`
// dry_run:true   → returns { dry_run: true, requested_generate, ... }
```

- [ ] **Step 5.2: Implement**

```ts
// useCreateDispatch.ts
import { useMutation } from "@tanstack/react-query"
import { createDispatch } from "../api"
import type { CreateDispatchRequest, CreateDispatchResponse } from "../types"

export function useCreateDispatch() {
  return useMutation<CreateDispatchResponse, Error, CreateDispatchRequest>({
    mutationFn: createDispatch,
  })
}

// Type guards for consumers
import type { PersistedDispatchResponse, BulletsDispatchResponse, DryRunDispatchResponse } from "../types"

export function isPersisted(r: CreateDispatchResponse): r is PersistedDispatchResponse {
  return "id" in r
}
export function isDryRun(r: CreateDispatchResponse): r is DryRunDispatchResponse {
  return "dry_run" in r && r.dry_run === true
}
export function isBulletsOnly(r: CreateDispatchResponse): r is BulletsDispatchResponse {
  return "generate" in r && (r as any).generate === false
}
```

- [ ] **Step 5.3: Commit**

---

## Task 6: Markdown renderer + `DispatchDetail` component

**Files:**
- Create: `frontend/src/domains/platform/dispatches/components/DispatchDetail.tsx`
- Create: `frontend/src/domains/platform/dispatches/components/DispatchDetail.test.tsx`

- [ ] **Step 6.1: Failing test — renders bullets + optional generated_text**

```tsx
// DispatchDetail.test.tsx
it("renders bullets grouped by section", () => {
  const d = { repos: [{ full_name: "x/y", sections: [{ label: "CI", bullets: [{ checkpoint_id: "1", text: "fix", source: "cloud_analysis", branch: "main", created_at: "", labels: [] }] }] }] }
  render(<DispatchDetail dispatch={d as any} />)
  expect(screen.getByText("CI")).toBeInTheDocument()
  expect(screen.getByText("fix")).toBeInTheDocument()
})

it("renders generated_text when present", () => {
  render(<DispatchDetail dispatch={{ ...base, generated_text: "Beep boop" } as any} />)
  expect(screen.getByText(/Beep boop/)).toBeInTheDocument()
})
```

- [ ] **Step 6.2: Implement**

Reuse whatever markdown renderer already ships in the checkpoints domain (check `frontend/src/domains/platform/checkpoints/components/` — likely a `Markdown.tsx` or similar). Compose the component to render `generated_text` (if present) OR fall back to a structured bullet list from `repos[].sections[].bullets`.

- [ ] **Step 6.3: Commit**

---

## Task 7: `DispatchSidebar` component

**Files:**
- Create: `frontend/src/domains/platform/dispatches/components/DispatchSidebar.tsx`
- Create: `frontend/src/domains/platform/dispatches/components/DispatchSidebar.test.tsx`

- [ ] **Step 7.1: Implement + test**

Sidebar sections (per spec):
- **Scope**: repo badges (from `covered_repos`), window (normalized_since → normalized_until), branches, voice name
- **Created**: `generated_at` (use `window.last_checkpoint_created_at` as a proxy for now)
- **Actions**: `Copy markdown`, `Open in CLI` (opens `OpenInCliModal`), `Copy share link`

Tests assert each button exists and `Copy markdown` copies the raw markdown payload via the clipboard API (mock `navigator.clipboard.writeText`).

Commit: `dispatches: sidebar with scope + actions`.

---

## Task 8: `DispatchCard` component

**Files:**
- Create: `frontend/src/domains/platform/dispatches/components/DispatchCard.tsx`
- Create: `frontend/src/domains/platform/dispatches/components/DispatchCard.test.tsx`

- [ ] **Step 8.1: Implement + test**

Card shape (per spec):
- Title (first `<h1>`/`<h2>` in generated text; fallback to "Dispatch for `<covered_repos[0]>` — `<date range>`")
- Body preview (~180 chars of rendered text)
- Footer row: repo badges (if multi-repo), checkpoint count, voice name (from `voice_name` if present)
- Date column: relative time + window
- Click → `/dispatches/:id`

Tests: title-fallback logic, click-navigation.

Commit: `dispatches: listing card`.

---

## Task 9: `DispatchFilterBar` component

**Files:**
- Create: `frontend/src/domains/platform/dispatches/components/DispatchFilterBar.tsx`
- Create: `frontend/src/domains/platform/dispatches/components/DispatchFilterBar.test.tsx`

- [ ] **Step 9.1: Implement with controlled inputs**

Fields: date range preset + custom, voice select ("All voices" / neutral / marvin), search input.

Emit changes via a single `onChange({since, until, voice, q})` callback.

Commit: `dispatches: filter bar`.

---

## Task 10: `VoicePicker` component

**Files:**
- Create: `frontend/src/domains/platform/dispatches/components/VoicePicker.tsx`
- Create: `frontend/src/domains/platform/dispatches/components/VoicePicker.test.tsx`

- [ ] **Step 10.1: Implement the card grid**

Four tiles (per spec): Neutral, Marvin, Custom description, Upload .md. "Custom" reveals a textarea; "Upload" uses a file input. State: `{kind: "preset"|"literal"|"file", value: string}` → emits a normalized string value suitable for the API's `voice` field.

Commit: `dispatches: voice picker`.

---

## Task 11: `DispatchForm` component

**Files:**
- Create: `frontend/src/domains/platform/dispatches/components/DispatchForm.tsx`
- Create: `frontend/src/domains/platform/dispatches/components/DispatchForm.test.tsx`

- [ ] **Step 11.1: Implement the create form**

Fields per spec:
- Scope: repo/org toggle + text input
- Time window: preset pills + custom date pair
- Branches: Default / All / Select (multi-select)
- Format: Markdown / Text / JSON (affects preview rendering; response payload is identical otherwise)
- Generate: Bullets only / Generate with voice
- Voice: `VoicePicker` (hidden when Bullets only)

Two buttons: "Generate dispatch" (primary — `createDispatch` with `dry_run: false`), "Preview (dry-run)" (secondary — `dry_run: true`).

On `Generate` success with `isPersisted(r)` → navigate to `/dispatches/:id`. On success with `isBulletsOnly(r)` → show the response inline on the Create page. On success with `isDryRun(r)` → render preview inline, never navigate.

Tests for each branch's post-submit behavior.

Commit: `dispatches: create form wiring createDispatch branches`.

---

## Task 12: `DispatchPreviewPane` component

**Files:**
- Create: `frontend/src/domains/platform/dispatches/components/DispatchPreviewPane.tsx`
- Create: `frontend/src/domains/platform/dispatches/components/DispatchPreviewPane.test.tsx`

- [ ] **Step 12.1: Implement**

Shows live "what this will cover" stats computed from the form state (no API call) plus the equivalent CLI command as a monospace block with a Copy button. Updates as the form changes.

Commit: `dispatches: live preview pane`.

---

## Task 13: `OpenInCliModal` component

**Files:**
- Create: `frontend/src/domains/platform/dispatches/components/OpenInCliModal.tsx`
- Create: `frontend/src/domains/platform/dispatches/components/OpenInCliModal.test.tsx`

- [ ] **Step 13.1: Implement**

Modal with a single monospace block showing the equivalent `entire dispatch …` command (derived from the dispatch's scope + voice). Copy button.

Commit: `dispatches: open-in-cli modal`.

---

## Task 14: `DispatchDetailPage`

**Files:**
- Create: `frontend/src/domains/platform/dispatches/pages/DispatchDetailPage.tsx`
- Create: `frontend/src/domains/platform/dispatches/pages/DispatchDetailPage.test.tsx`

- [ ] **Step 14.1: Compose `useDispatch` + `DispatchDetail` + `DispatchSidebar`**

States:
- `isLoading` → skeleton layout
- `isError` (including 404) → standard 404 layout
- `data.status === "generating"` → loading skeleton + "Generating, usually takes 5–15s" message, polling every 2s
- `data.status === "failed"` → error card with `data.error_message ?? "Generation failed"` + "Retry" button (Retry: re-submits the Create form with the same inputs — which, because of the fingerprint, will re-reserve if the failed row was swept or produce a new row otherwise)
- `data.status === "complete"` → render DispatchDetail + DispatchSidebar

- [ ] **Step 14.2: Tests + commit**

---

## Task 15: `DispatchPerRepoPage`

**Files:**
- Create: `frontend/src/domains/platform/dispatches/pages/DispatchPerRepoPage.tsx`
- Create: `frontend/src/domains/platform/dispatches/pages/DispatchPerRepoPage.test.tsx`

- [ ] **Step 15.1: Implement**

Header: "Dispatches for `:org/:repo`" + `DispatchFilterBar` + "+ New dispatch" button (routes to `/dispatches/new` with repo pre-filled via query string).

Body: `useRepoDispatches` → map `items` to `DispatchCard`s. Infinite scroll / "Load more" at the bottom.

Empty state: "No dispatches yet for this repo. [Create one here](/dispatches/new)."

- [ ] **Step 15.2: Tests + commit**

---

## Task 16: `DispatchPerOrgPage`

Same pattern as Per-Repo but using `useOrgDispatches`. Commit when done.

---

## Task 17: `DispatchNewPage`

**Files:**
- Create: `frontend/src/domains/platform/dispatches/pages/DispatchNewPage.tsx`
- Create: `frontend/src/domains/platform/dispatches/pages/DispatchNewPage.test.tsx`

- [ ] **Step 17.1: Compose form + preview**

Two-column layout: left = `DispatchForm`, right = `DispatchPreviewPane`. Handle submit per Task 11. Optional: accept `?repo=` query param to pre-fill the form.

- [ ] **Step 17.2: Tests + commit**

---

## Task 18: Route wire-up

**Files:**
- Create: `frontend/src/routes/_authenticated/dispatches.new.tsx`
- Create: `frontend/src/routes/_authenticated/dispatches.$dispatchId.tsx`
- Create: `frontend/src/routes/_authenticated/orgs.$org.dispatches.index.tsx`
- Create: `frontend/src/routes/_repo/gh/$org.$repo.dispatches.index.tsx`

- [ ] **Step 18.1: Each route is a thin shim that exports the page component**

Follow the existing pattern in `routes/_repo/gh/$org.$repo.checkpoints.index.tsx`.

```tsx
// routes/_authenticated/dispatches.$dispatchId.tsx
import { createFileRoute } from "@tanstack/react-router"
import { DispatchDetailPage } from "../../domains/platform/dispatches/pages/DispatchDetailPage"

export const Route = createFileRoute("/_authenticated/dispatches/$dispatchId")({
  component: DispatchDetailPage,
})
```

- [ ] **Step 18.2: Verify route types regenerate**

```bash
pnpm --filter frontend exec tsr generate
```

- [ ] **Step 18.3: Commit**

```bash
git add frontend/src/routes/
git commit -m "dispatches: wire routes for detail, per-repo, per-org, new"
```

---

## Task 19: Repo sidebar navigation — add "Dispatches" entry

**Files:**
- Modify: the existing repo sidebar component (grep for `Checkpoints` link to find it)

- [ ] **Step 19.1: Add "Dispatches" link alongside Checkpoints / Trails / Runners**

Points to `/gh/:org/:repo/dispatches`.

- [ ] **Step 19.2: Test + commit**

---

## Task 20: Top-level nav — add "Dispatches" entry (optional; check mockup)

If the topbar/nav component already has a concept of top-level pages for Repos / Search / Overview, add a "Dispatches" entry that navigates to the viewer's org index (or a menu if they have multiple orgs). Follow existing patterns. Commit.

---

## Task 21: Styling polish

- [ ] **Step 21.1: Invoke the `frontend-design` skill for production-grade polish**

Per the project's CLAUDE.md: "For Frontend changes always use brainstorm companion; When updating or building the UI / frontend use the frontend-design skill". Hand off the four pages (Detail, PerRepo, PerOrg, New) + the card/sidebar/form components to the `frontend-design` skill for visual polish, aesthetic refinement, and ensuring the design avoids generic AI aesthetics.

This step produces CSS/styled-component changes — commit separately as `dispatches: visual polish`.

---

## Task 22: E2E (optional, if Playwright/Cypress is set up)

- [ ] **Step 22.1: Write a dispatch happy-path E2E test**

Seed backend state via a test helper. Navigate to `/dispatches/new`, fill the form, submit with `Generate`, expect to land on `/dispatches/:id`, expect the generated text to be present. Follow any existing E2E pattern in `frontend/e2e/`.

Commit: `dispatches: e2e happy path`.

---

## Task 23: Self-review + CI check

- [ ] **Step 23.1: Run all tests**

```bash
pnpm --filter frontend test
```

- [ ] **Step 23.2: Type check + lint**

```bash
pnpm --filter frontend exec tsc --noEmit
pnpm --filter frontend exec eslint src/
```

- [ ] **Step 23.3: Spec-coverage walk-through**

Skim the spec's Pages, API surface, Access control, and Testing sections. For each requirement, point to a task that implements it. Fill gaps with a fix-up commit.

---

## Execution Handoff

Plan complete and saved to `plans/2026-04-16-entire-dispatch-web-ui.md`.

Dependency graph:
- Tasks 1–2 (types, api) first
- Tasks 3–5 (hooks) depend on api
- Tasks 6–13 (components) depend on types + any needed hooks; mostly parallelizable
- Tasks 14–17 (pages) depend on components + hooks
- Task 18 (routes) depends on pages
- Tasks 19–20 (nav) and 21 (styling) after pages exist
- Task 22 depends on backend being deployed

Total tasks: 23. Estimated ~1–1.5 weeks for one frontend engineer; parallelizable with a partner splitting components.
