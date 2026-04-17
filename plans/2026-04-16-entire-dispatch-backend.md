# Entire Dispatch — Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the `/dispatches` API endpoints to entire.io backend: `POST /api/v1/users/me/dispatches` (create / preview), `GET /api/v1/dispatches/:id` (detail), `GET /api/v1/repos/:org/:repo/dispatches` and `GET /api/v1/orgs/:org/dispatches` (listings), plus `GET /api/v1/orgs/:org/checkpoints` (enumeration for CLI --local --org). Update the existing batch-analyses endpoint to return explicit per-ID statuses (`not_visible`, `unknown`).

**Architecture:** Content-addressed cache. A persisted dispatch is keyed on `SHA-256(sorted(used_checkpoint_ids) || "|" || normalized_voice)`. Only `generate: true` requests persist; `generate: false` and `dry_run: true` return inline without rows. Reserve-before-synthesize via a partial unique index + `INSERT … ON CONFLICT DO NOTHING` so concurrent POSTs do exactly one LLM call. No creator_id, no DELETE in v1, no personal feed — authorization is a live strict check on the dispatch's frozen `covered_repos`.

**Tech Stack:** TypeScript, Hono, Kysely on PlanetScale, Anthropic SDK for LLM synthesis. Tests use vitest (follow the existing `*.test.ts` pattern in `api/src/routes/`).

**Companion spec:** [`specs/2026-04-16-entire-dispatch-design.md`](../specs/2026-04-16-entire-dispatch-design.md) (Persistence-and-idempotency section is the normative contract).

**Worktree constraint:** All work in this plan happens in a **new** git worktree whose branch is created off `analysis-chunk-merge`. Do not modify `analysis-chunk-merge` directly.

---

## Setup — create worktree

- [ ] **Step S.1: Create worktree and branch**

```bash
cd /Users/alisha/Projects/wt/entire.io/analysis-chunk-merge
git worktree add -b dispatch-backend ../dispatch-backend
cd ../dispatch-backend
```

Expected: `../dispatch-backend` directory now exists, current HEAD matches `analysis-chunk-merge`'s tip.

- [ ] **Step S.2: Install deps**

```bash
cd /Users/alisha/Projects/wt/entire.io/dispatch-backend
pnpm install
```

Expected: install succeeds, workspace packages available.

---

## File Structure

### Migrations

- `api/db/migrations/20260417000000_add_dispatches.ts` — creates `dispatches` table with unique partial index.
- `api/db/migrations/20260417000100_add_dispatch_status_enums.ts` — (optional — only if analyses already use an enum type and we want consistency).

### DB types / Kysely

- `api/src/lib/db/db-types.ts` — add `DispatchesNamespace` interface + `DispatchRow` type.
- `api/src/lib/planetscale/dispatches.ts` (create) — `createDispatchesNamespace(db)` with methods: `reserve`, `completeSynthesis`, `markFailed`, `getById`, `getByFingerprint`, `listForRepo`, `listForOrg`, `sweepStale`.
- `api/src/lib/planetscale/dispatches.test.ts` (create) — unit tests for each method against the test DB.

### Fingerprint / helpers

- `api/src/lib/dispatch-fingerprint.ts` (create) — `computeUsedIdsHash(usedIds: string[])`, `normalizeVoice(voice: string | undefined)`, `computeFingerprint({usedIds, voice})`.
- `api/src/lib/dispatch-fingerprint.test.ts` (create) — unit tests for determinism, sorting, voice normalization.

### Window normalization

- `api/src/lib/dispatch-window.ts` (create) — `normalizeWindow({since, until})` returning floor-to-minute `since` and ceil-to-minute `until`.
- `api/src/lib/dispatch-window.test.ts` (create) — unit tests for boundary behavior (14:32:47.893 → 14:32:00 floor; ceil; same-minute sub-seconds).

### Scope resolution / access

- `api/src/lib/dispatch-access.ts` (create) — `resolveCoveredRepos(userId, { repo?, org? })` returns the filtered set; `authorizeDispatchView(userId, dispatch)` returns allow|deny; shared by all dispatch endpoints.
- `api/src/lib/dispatch-access.test.ts` (create) — mixed-access scenarios, revoked-access-after-creation path, creator-loses-access path.

### Candidate enumeration

- `api/src/lib/dispatch-candidates.ts` (create) — `enumerateCandidates({coveredRepos, since, until, branches})` returns candidate checkpoint metadata (id, repo, branch, created_at); `applyFallbackChain(candidates, analyses)` returns `{used, warnings}`.
- `api/src/lib/dispatch-candidates.test.ts` (create) — fallback chain tests mirroring CLI spec (cloud complete → used bullet; pending → pending_count++; unknown → unknown_count++; not_visible → access_denied_count++; etc.).

### Synthesis

- `api/src/lib/dispatch-synthesis.ts` (create) — `runSynthesis(bullets, voice)` calls Anthropic SDK, returns `generated_text`. Timeouts, retry-once-on-transient.
- `api/src/lib/dispatch-synthesis.test.ts` (create) — mock SDK, assert prompt shape, timeout handling.

### Voice presets

- `api/src/lib/dispatch-voices/neutral.md` (create)
- `api/src/lib/dispatch-voices/marvin.md` (create)
- `api/src/lib/dispatch-voices.ts` (create) — embeds the markdown files via Vite's `?raw` or Node's `fs.readFileSync`; `resolveVoice(name: string)` returns preset text if name matches a preset, else passes through.

### Routes

- `api/src/routes/dispatches.ts` (create) — all four new endpoints plus hooks into the existing Hono app.
- `api/src/routes/dispatches.test.ts` (create) — integration tests per endpoint.
- `api/src/routes/cache.ts:4530-4597` (modify) — update `formatCheckpointAnalysisResponse` to return `not_visible` and `unknown` statuses explicitly.
- `api/src/routes/cache.test.ts` (modify) — add tests for the new statuses.
- `api/src/routes/repo-overview.ts` OR a new `api/src/routes/orgs.ts` (create) — `GET /api/v1/orgs/:org/checkpoints` enumeration endpoint.

### Sweeper (stale `generating` rows)

- `api/src/lib/dispatch-sweeper.ts` (create) — `sweepStaleGenerating(db, { olderThanMinutes })` marks abandoned rows `failed`.
- `api/src/lib/dispatch-sweeper.test.ts` (create).
- `api/src/index.ts` (modify) — register a Cloudflare cron trigger (or worker interval) invoking the sweeper every 5 min.

### App wire-up

- `api/src/index.ts` — mount `dispatchesRoutes` and any org-level enumeration routes.

---

## Task 1: Migration — create `dispatches` table

**Files:**
- Create: `api/db/migrations/20260417000000_add_dispatches.ts`
- Modify: `api/src/lib/db/db-types.ts` (regenerated types; run migration to produce)

- [ ] **Step 1.1: Write the migration up/down**

```ts
// api/db/migrations/20260417000000_add_dispatches.ts
import { sql, type Kysely } from "kysely"

export async function up(db: Kysely<any>): Promise<void> {
  await db.schema
    .createTable("dispatches")
    .addColumn("id", "varchar(32)", (col) => col.primaryKey())
    .addColumn("fingerprint_hash", "varchar(128)", (col) => col.notNull())
    .addColumn("status", sql`ENUM('generating','complete','failed')`, (col) => col.notNull())
    .addColumn("covered_repos", "json", (col) => col.notNull())    // JSON array of "org/repo" strings
    .addColumn("rendered_bullets", "json", (col) => col.notNull()) // JSON of the repos→sections→bullets payload
    .addColumn("generated_text", "text")                           // null until status=complete with generate=true
    .addColumn("totals", "json", (col) => col.notNull())           // { checkpoints, used_checkpoint_count, branches, files_touched }
    .addColumn("warnings", "json", (col) => col.notNull())         // { access_denied_count, pending_count, failed_count, unknown_count, uncategorized_count }
    .addColumn("window_normalized_since", "datetime(0)", (col) => col.notNull())
    .addColumn("window_normalized_until", "datetime(0)", (col) => col.notNull())
    .addColumn("first_checkpoint_created_at", "datetime(0)")
    .addColumn("last_checkpoint_created_at", "datetime(0)")
    .addColumn("voice_name", "varchar(128)")                       // preset name if preset; null if file/literal
    .addColumn("voice_normalized_hash", "varchar(128)", (col) => col.notNull())
    .addColumn("error_message", "text")
    .addColumn("started_at", "datetime(0)", (col) => col.notNull().defaultTo(sql`CURRENT_TIMESTAMP`))
    .addColumn("completed_at", "datetime(0)")
    .addColumn("created_at", "datetime(0)", (col) => col.notNull().defaultTo(sql`CURRENT_TIMESTAMP`))
    .addColumn("updated_at", "datetime(0)", (col) => col.notNull().defaultTo(sql`CURRENT_TIMESTAMP`))
    .execute()

  // Partial unique index: only active rows (generating/complete) dedupe on fingerprint.
  // PlanetScale/MySQL doesn't support PARTIAL indexes directly; use a virtual/generated column trick
  // to express "fingerprint_hash when status != 'failed'".
  await sql`
    ALTER TABLE dispatches
    ADD COLUMN fingerprint_active VARCHAR(128)
      AS (CASE WHEN status IN ('generating','complete') THEN fingerprint_hash ELSE NULL END) VIRTUAL,
    ADD UNIQUE INDEX idx_dispatches_fingerprint_active (fingerprint_active)
  `.execute(db)

  // Supporting indexes for listings.
  await db.schema
    .createIndex("idx_dispatches_created_at")
    .on("dispatches")
    .column("created_at")
    .execute()

  await sql`
    CREATE INDEX idx_dispatches_status_started_at ON dispatches (status, started_at)
  `.execute(db)
}

export async function down(db: Kysely<any>): Promise<void> {
  await db.schema.dropTable("dispatches").execute()
}
```

- [ ] **Step 1.2: Run the migration locally**

```bash
cd /Users/alisha/Projects/wt/entire.io/dispatch-backend
pnpm --filter api migrate:latest
```

Expected: migration applies cleanly; `dispatches` table created; regenerated types appear under `api/db/types.ts`.

- [ ] **Step 1.3: Verify the partial index behaves**

```bash
pnpm --filter api db:shell
# Then in the shell:
INSERT INTO dispatches (id, fingerprint_hash, status, covered_repos, rendered_bullets, totals, warnings,
  window_normalized_since, window_normalized_until, voice_normalized_hash)
VALUES ('a', 'FP1', 'complete', '[]', '[]', '{}', '{}', NOW(), NOW(), 'v');
# Repeat with a different id, same fingerprint — should fail with duplicate key.
INSERT INTO dispatches (id, fingerprint_hash, status, ...)
VALUES ('b', 'FP1', 'complete', ...);  -- expect: ERROR 1062 Duplicate entry 'FP1' for key 'idx_dispatches_fingerprint_active'
# But a failed row with the same fingerprint should succeed (virtual index drops it).
UPDATE dispatches SET status='failed' WHERE id='a';
INSERT INTO dispatches (id, fingerprint_hash, status, ...)
VALUES ('c', 'FP1', 'complete', ...);  -- expect: success
```

Expected: duplicate constraint prevents two active rows per fingerprint; failed rows don't block.

- [ ] **Step 1.4: Commit**

```bash
git add api/db/migrations/20260417000000_add_dispatches.ts api/db/types.ts
git commit -m "feat(dispatches): add dispatches table migration"
```

---

## Task 2: Kysely namespace — `dispatches.ts`

**Files:**
- Create: `api/src/lib/planetscale/dispatches.ts`
- Create: `api/src/lib/planetscale/dispatches.test.ts`
- Modify: `api/src/lib/db/db-types.ts` — add `DispatchesNamespace` interface and `DispatchRow` type.
- Modify: `api/src/lib/planetscale/index.ts` (or wherever namespaces are aggregated) — register the new namespace.

- [ ] **Step 2.1: Write failing tests for reserve + complete**

```ts
// api/src/lib/planetscale/dispatches.test.ts
import { beforeEach, describe, expect, it } from "vitest"
import { getTestDb, resetDb } from "../../test/planetscale/helpers"
import { createDispatchesNamespace } from "./dispatches"

describe("dispatches namespace", () => {
  let ns: ReturnType<typeof createDispatchesNamespace>
  const base = {
    coveredRepos: ["entireio/cli"],
    renderedBullets: [],
    totals: { checkpoints: 0, used_checkpoint_count: 0, branches: 0, files_touched: 0 },
    warnings: { access_denied_count: 0, pending_count: 0, failed_count: 0, unknown_count: 0, uncategorized_count: 0 },
    windowNormalizedSince: new Date("2026-04-09T00:00:00Z"),
    windowNormalizedUntil: new Date("2026-04-16T00:00:00Z"),
    voiceName: "neutral",
    voiceNormalizedHash: "vh",
  }

  beforeEach(async () => {
    await resetDb()
    ns = createDispatchesNamespace(getTestDb())
  })

  it("reserve creates a new generating row and returns it", async () => {
    const r = await ns.reserve({ fingerprintHash: "FP1", ...base })
    expect(r).toBeTruthy()
    expect(r!.status).toBe("generating")
    expect(r!.fingerprint_hash).toBe("FP1")
  })

  it("second reserve with same fingerprint returns null (conflict)", async () => {
    await ns.reserve({ fingerprintHash: "FP1", ...base })
    const second = await ns.reserve({ fingerprintHash: "FP1", ...base })
    expect(second).toBeNull()
  })

  it("completeSynthesis marks the row complete and sets generated_text", async () => {
    const r = await ns.reserve({ fingerprintHash: "FP2", ...base })
    await ns.completeSynthesis({
      id: r!.id,
      generatedText: "Beep boop",
      firstCheckpointCreatedAt: new Date("2026-04-09T00:00:00Z"),
      lastCheckpointCreatedAt: new Date("2026-04-16T00:00:00Z"),
    })
    const got = await ns.getById(r!.id)
    expect(got!.status).toBe("complete")
    expect(got!.generated_text).toBe("Beep boop")
  })

  it("markFailed sets status=failed and records error", async () => {
    const r = await ns.reserve({ fingerprintHash: "FP3", ...base })
    await ns.markFailed({ id: r!.id, errorMessage: "LLM timeout" })
    const got = await ns.getById(r!.id)
    expect(got!.status).toBe("failed")
    expect(got!.error_message).toBe("LLM timeout")
  })

  it("reserve after markFailed succeeds for the same fingerprint", async () => {
    const r1 = await ns.reserve({ fingerprintHash: "FP4", ...base })
    await ns.markFailed({ id: r1!.id, errorMessage: "x" })
    const r2 = await ns.reserve({ fingerprintHash: "FP4", ...base })
    expect(r2).toBeTruthy()
    expect(r2!.id).not.toBe(r1!.id)
  })
})
```

- [ ] **Step 2.2: Run and see it fail**

```bash
pnpm --filter api test src/lib/planetscale/dispatches.test.ts
```

Expected: FAIL — `createDispatchesNamespace` not defined.

- [ ] **Step 2.3: Implement the namespace**

```ts
// api/src/lib/planetscale/dispatches.ts
import { sql, type Kysely } from "kysely"
import type { DB, Dispatches } from "../../../db/types"
import type { DispatchesNamespace, DispatchRow } from "../db/db-types"
import { generateId } from "../uuid"

function toRow(r: any): DispatchRow {
  return {
    id: r.id,
    fingerprint_hash: r.fingerprint_hash,
    status: r.status as "generating" | "complete" | "failed",
    covered_repos: typeof r.covered_repos === "string" ? JSON.parse(r.covered_repos) : r.covered_repos,
    rendered_bullets: typeof r.rendered_bullets === "string" ? JSON.parse(r.rendered_bullets) : r.rendered_bullets,
    generated_text: r.generated_text ?? null,
    totals: typeof r.totals === "string" ? JSON.parse(r.totals) : r.totals,
    warnings: typeof r.warnings === "string" ? JSON.parse(r.warnings) : r.warnings,
    window_normalized_since: r.window_normalized_since,
    window_normalized_until: r.window_normalized_until,
    first_checkpoint_created_at: r.first_checkpoint_created_at,
    last_checkpoint_created_at: r.last_checkpoint_created_at,
    voice_name: r.voice_name ?? null,
    voice_normalized_hash: r.voice_normalized_hash,
    error_message: r.error_message ?? null,
    started_at: r.started_at,
    completed_at: r.completed_at ?? null,
    created_at: r.created_at,
    updated_at: r.updated_at,
  }
}

export function createDispatchesNamespace(db: Kysely<DB>): DispatchesNamespace {
  return {
    async reserve(args) {
      const id = generateId()
      const now = new Date()
      // INSERT … ON CONFLICT DO NOTHING equivalent on MySQL: INSERT IGNORE.
      const result = await db
        .insertInto("dispatches")
        .values({
          id,
          fingerprint_hash: args.fingerprintHash,
          status: "generating",
          covered_repos: JSON.stringify(args.coveredRepos),
          rendered_bullets: JSON.stringify(args.renderedBullets),
          totals: JSON.stringify(args.totals),
          warnings: JSON.stringify(args.warnings),
          window_normalized_since: args.windowNormalizedSince,
          window_normalized_until: args.windowNormalizedUntil,
          voice_name: args.voiceName ?? null,
          voice_normalized_hash: args.voiceNormalizedHash,
          started_at: now,
          created_at: now,
          updated_at: now,
        })
        .ignore() // MySQL INSERT IGNORE → no error on conflict
        .executeTakeFirst()

      if (BigInt(result.numInsertedOrUpdatedRows ?? 0) === 0n) return null

      const row = await db
        .selectFrom("dispatches")
        .selectAll()
        .where("id", "=", id)
        .executeTakeFirst()

      return row ? toRow(row) : null
    },

    async completeSynthesis({ id, renderedBullets, totals, warnings, generatedText, firstCheckpointCreatedAt, lastCheckpointCreatedAt }) {
      const now = new Date()
      await db
        .updateTable("dispatches")
        .set({
          status: "complete",
          rendered_bullets: JSON.stringify(renderedBullets),
          totals: JSON.stringify(totals),
          warnings: JSON.stringify(warnings),
          generated_text: generatedText ?? null,
          first_checkpoint_created_at: firstCheckpointCreatedAt ?? null,
          last_checkpoint_created_at: lastCheckpointCreatedAt ?? null,
          completed_at: now,
          updated_at: now,
        })
        .where("id", "=", id)
        .execute()
    },

    async markFailed({ id, errorMessage }) {
      const now = new Date()
      await db
        .updateTable("dispatches")
        .set({
          status: "failed",
          error_message: errorMessage,
          completed_at: now,
          updated_at: now,
        })
        .where("id", "=", id)
        .execute()
    },

    async getById(id) {
      const row = await db.selectFrom("dispatches").selectAll().where("id", "=", id).executeTakeFirst()
      return row ? toRow(row) : null
    },

    async getByFingerprint(fingerprintHash) {
      const row = await db
        .selectFrom("dispatches")
        .selectAll()
        .where("fingerprint_hash", "=", fingerprintHash)
        .where("status", "in", ["generating", "complete"])
        .executeTakeFirst()
      return row ? toRow(row) : null
    },

    async listForRepo({ repoFullName, since, until, voiceName, limit, cursor }) {
      let q = db
        .selectFrom("dispatches")
        .selectAll()
        .where("status", "=", "complete")
        .where(sql<boolean>`JSON_CONTAINS(covered_repos, JSON_QUOTE(${repoFullName}))`)
        .orderBy("created_at", "desc")
        .limit(limit + 1)

      if (since) q = q.where("created_at", ">=", new Date(since))
      if (until) q = q.where("created_at", "<", new Date(until))
      if (voiceName) q = q.where("voice_name", "=", voiceName)
      if (cursor) q = q.where("created_at", "<", new Date(cursor))

      const rows = await q.execute()
      const hasMore = rows.length > limit
      const items = rows.slice(0, limit).map(toRow)
      return { items, nextCursor: hasMore ? items[items.length - 1].created_at.toISOString() : null }
    },

    async listForOrg({ orgName, since, until, voiceName, limit, cursor }) {
      // JSON_CONTAINS with a wildcard isn't expressible directly; use JSON_CONTAINS_PATH or a regex.
      // Simpler: store an auxiliary column "covered_orgs" or use JSON_OVERLAPS once PlanetScale supports it.
      // For v1 query by checking each row's covered_repos entries; acceptable perf for current volumes.
      let q = db
        .selectFrom("dispatches")
        .selectAll()
        .where("status", "=", "complete")
        .where(sql<boolean>`JSON_SEARCH(covered_repos, 'one', ${`${orgName}/%`}) IS NOT NULL`)
        .orderBy("created_at", "desc")
        .limit(limit + 1)
      if (since) q = q.where("created_at", ">=", new Date(since))
      if (until) q = q.where("created_at", "<", new Date(until))
      if (voiceName) q = q.where("voice_name", "=", voiceName)
      if (cursor) q = q.where("created_at", "<", new Date(cursor))
      const rows = await q.execute()
      const hasMore = rows.length > limit
      const items = rows.slice(0, limit).map(toRow)
      return { items, nextCursor: hasMore ? items[items.length - 1].created_at.toISOString() : null }
    },

    async sweepStale({ olderThanMinutes }) {
      const cutoff = new Date(Date.now() - olderThanMinutes * 60_000)
      const result = await db
        .updateTable("dispatches")
        .set({
          status: "failed",
          error_message: "abandoned generating row swept",
          completed_at: new Date(),
          updated_at: new Date(),
        })
        .where("status", "=", "generating")
        .where("started_at", "<", cutoff)
        .executeTakeFirst()
      return Number(result.numUpdatedRows ?? 0)
    },
  }
}
```

- [ ] **Step 2.4: Add the interface + types**

```ts
// api/src/lib/db/db-types.ts (append)
export interface DispatchRow {
  id: string
  fingerprint_hash: string
  status: "generating" | "complete" | "failed"
  covered_repos: string[]
  rendered_bullets: unknown
  generated_text: string | null
  totals: { checkpoints: number; used_checkpoint_count: number; branches: number; files_touched: number }
  warnings: {
    access_denied_count: number
    pending_count: number
    failed_count: number
    unknown_count: number
    uncategorized_count: number
  }
  window_normalized_since: Date
  window_normalized_until: Date
  first_checkpoint_created_at: Date | null
  last_checkpoint_created_at: Date | null
  voice_name: string | null
  voice_normalized_hash: string
  error_message: string | null
  started_at: Date
  completed_at: Date | null
  created_at: Date
  updated_at: Date
}

export interface DispatchesNamespace {
  reserve(args: {
    fingerprintHash: string
    coveredRepos: string[]
    renderedBullets: unknown
    totals: DispatchRow["totals"]
    warnings: DispatchRow["warnings"]
    windowNormalizedSince: Date
    windowNormalizedUntil: Date
    voiceName?: string
    voiceNormalizedHash: string
  }): Promise<DispatchRow | null>

  completeSynthesis(args: {
    id: string
    renderedBullets: unknown
    totals: DispatchRow["totals"]
    warnings: DispatchRow["warnings"]
    generatedText?: string
    firstCheckpointCreatedAt?: Date
    lastCheckpointCreatedAt?: Date
  }): Promise<void>

  markFailed(args: { id: string; errorMessage: string }): Promise<void>
  getById(id: string): Promise<DispatchRow | null>
  getByFingerprint(fingerprintHash: string): Promise<DispatchRow | null>

  listForRepo(args: {
    repoFullName: string
    since?: string
    until?: string
    voiceName?: string
    limit: number
    cursor?: string
  }): Promise<{ items: DispatchRow[]; nextCursor: string | null }>

  listForOrg(args: {
    orgName: string
    since?: string
    until?: string
    voiceName?: string
    limit: number
    cursor?: string
  }): Promise<{ items: DispatchRow[]; nextCursor: string | null }>

  sweepStale(args: { olderThanMinutes: number }): Promise<number>
}
```

- [ ] **Step 2.5: Register the namespace**

Follow the same registration pattern used by `analyses.ts` — add `dispatches: createDispatchesNamespace(db)` to the aggregate `getDb()` constructor in `api/src/lib/planetscale/index.ts` (or wherever namespaces are combined).

- [ ] **Step 2.6: Run tests, verify all pass**

```bash
pnpm --filter api test src/lib/planetscale/dispatches.test.ts
```

Expected: PASS — all 5 tests green.

- [ ] **Step 2.7: Commit**

```bash
git add api/src/lib/planetscale/dispatches.ts api/src/lib/planetscale/dispatches.test.ts api/src/lib/db/db-types.ts api/src/lib/planetscale/index.ts
git commit -m "feat(dispatches): add Kysely namespace for dispatches table"
```

---

## Task 3: Fingerprint helper

**Files:**
- Create: `api/src/lib/dispatch-fingerprint.ts`
- Create: `api/src/lib/dispatch-fingerprint.test.ts`

- [ ] **Step 3.1: Write failing tests**

```ts
// api/src/lib/dispatch-fingerprint.test.ts
import { describe, expect, it } from "vitest"
import { computeFingerprint, computeUsedIdsHash, normalizeVoice } from "./dispatch-fingerprint"

describe("computeUsedIdsHash", () => {
  it("is deterministic under reordering", () => {
    expect(computeUsedIdsHash(["a", "b", "c"])).toBe(computeUsedIdsHash(["c", "a", "b"]))
  })

  it("differs when any id changes", () => {
    expect(computeUsedIdsHash(["a", "b"])).not.toBe(computeUsedIdsHash(["a", "c"]))
  })

  it("empty list is stable", () => {
    expect(computeUsedIdsHash([])).toBe(computeUsedIdsHash([]))
  })
})

describe("normalizeVoice", () => {
  it("preserves preset names case-insensitively", () => {
    expect(normalizeVoice("Marvin", { isPreset: true })).toBe("marvin")
  })
  it("hashes literal voice strings", () => {
    const a = normalizeVoice("sardonic tone", { isPreset: false })
    const b = normalizeVoice("sardonic tone", { isPreset: false })
    expect(a).toBe(b)
    expect(a).not.toBe("sardonic tone")
  })
})

describe("computeFingerprint", () => {
  it("same ids + same voice → same fingerprint regardless of scope/window", () => {
    const a = computeFingerprint({ usedIds: ["x", "y"], voiceNormalizedHash: "vh" })
    const b = computeFingerprint({ usedIds: ["y", "x"], voiceNormalizedHash: "vh" })
    expect(a).toBe(b)
  })

  it("different voice → different fingerprint for same ids", () => {
    const a = computeFingerprint({ usedIds: ["x", "y"], voiceNormalizedHash: "v1" })
    const b = computeFingerprint({ usedIds: ["x", "y"], voiceNormalizedHash: "v2" })
    expect(a).not.toBe(b)
  })
})
```

- [ ] **Step 3.2: Run, verify fail**

```bash
pnpm --filter api test src/lib/dispatch-fingerprint.test.ts
```

Expected: FAIL.

- [ ] **Step 3.3: Implement**

```ts
// api/src/lib/dispatch-fingerprint.ts
import { createHash } from "node:crypto"

export function computeUsedIdsHash(ids: string[]): string {
  const sorted = [...ids].sort()
  return createHash("sha256").update(sorted.join(",")).digest("hex")
}

export function normalizeVoice(value: string | undefined, opts: { isPreset: boolean }): string {
  if (!value) return ""
  if (opts.isPreset) return value.toLowerCase()
  return createHash("sha256").update(value).digest("hex")
}

export function computeFingerprint(args: {
  usedIds: string[]
  voiceNormalizedHash: string
}): string {
  const idsHash = computeUsedIdsHash(args.usedIds)
  return createHash("sha256")
    .update(idsHash)
    .update("|")
    .update(args.voiceNormalizedHash)
    .digest("hex")
}
```

- [ ] **Step 3.4: Verify tests pass**

```bash
pnpm --filter api test src/lib/dispatch-fingerprint.test.ts
```

Expected: PASS.

- [ ] **Step 3.5: Commit**

```bash
git add api/src/lib/dispatch-fingerprint.ts api/src/lib/dispatch-fingerprint.test.ts
git commit -m "feat(dispatches): add content-addressed fingerprint helpers"
```

---

## Task 4: Window normalization

**Files:**
- Create: `api/src/lib/dispatch-window.ts`
- Create: `api/src/lib/dispatch-window.test.ts`

- [ ] **Step 4.1: Failing tests**

```ts
// api/src/lib/dispatch-window.test.ts
import { describe, expect, it } from "vitest"
import { normalizeWindow } from "./dispatch-window"

describe("normalizeWindow", () => {
  it("floors since, ceils until", () => {
    const r = normalizeWindow({
      since: "2026-04-16T14:32:47.893Z",
      until: "2026-04-16T14:33:12.001Z",
    })
    expect(r.normalizedSince.toISOString()).toBe("2026-04-16T14:32:00.000Z")
    expect(r.normalizedUntil.toISOString()).toBe("2026-04-16T14:34:00.000Z")
  })

  it("already-minute-aligned inputs are unchanged", () => {
    const r = normalizeWindow({ since: "2026-04-09T00:00:00Z", until: "2026-04-16T00:00:00Z" })
    expect(r.normalizedSince.toISOString()).toBe("2026-04-09T00:00:00.000Z")
    expect(r.normalizedUntil.toISOString()).toBe("2026-04-16T00:00:00.000Z")
  })

  it("same-minute sub-second variations collapse", () => {
    const a = normalizeWindow({ since: "2026-04-16T14:32:47Z", until: "2026-04-16T14:33:12Z" })
    const b = normalizeWindow({ since: "2026-04-16T14:32:12Z", until: "2026-04-16T14:33:55Z" })
    expect(a.normalizedSince.toISOString()).toBe(b.normalizedSince.toISOString())
    expect(a.normalizedUntil.toISOString()).toBe(b.normalizedUntil.toISOString())
  })

  it("rejects invalid ISO strings", () => {
    expect(() => normalizeWindow({ since: "not-a-date", until: "2026-04-16T14:33:00Z" }))
      .toThrow(/invalid since/i)
  })
})
```

- [ ] **Step 4.2: Run, fail**

```bash
pnpm --filter api test src/lib/dispatch-window.test.ts
```

- [ ] **Step 4.3: Implement**

```ts
// api/src/lib/dispatch-window.ts
export function normalizeWindow(args: { since: string; until: string }): {
  normalizedSince: Date
  normalizedUntil: Date
} {
  const since = new Date(args.since)
  const until = new Date(args.until)
  if (Number.isNaN(since.getTime())) throw new Error(`invalid since: ${args.since}`)
  if (Number.isNaN(until.getTime())) throw new Error(`invalid until: ${args.until}`)

  // Floor since to minute (strip seconds and ms).
  const flooredSince = new Date(since)
  flooredSince.setUTCSeconds(0, 0)

  // Ceil until to minute (if there are any sub-minute components, bump to next minute).
  const ceiledUntil = new Date(until)
  if (ceiledUntil.getUTCSeconds() > 0 || ceiledUntil.getUTCMilliseconds() > 0) {
    ceiledUntil.setUTCSeconds(0, 0)
    ceiledUntil.setTime(ceiledUntil.getTime() + 60_000)
  }

  return { normalizedSince: flooredSince, normalizedUntil: ceiledUntil }
}
```

- [ ] **Step 4.4: Pass + commit**

```bash
pnpm --filter api test src/lib/dispatch-window.test.ts
git add api/src/lib/dispatch-window.ts api/src/lib/dispatch-window.test.ts
git commit -m "feat(dispatches): add floor/ceil window normalization"
```

---

## Task 5: Update batch-analyses endpoint with `not_visible` / `unknown` statuses

**Files:**
- Modify: `api/src/routes/cache.ts:4506-4600` — `formatCheckpointAnalysisResponse` + `batch` handler
- Modify: `api/src/routes/cache.test.ts` — add new status tests

- [ ] **Step 5.1: Write failing tests**

```ts
// cache.test.ts (append)
describe("POST /api/v1/users/me/checkpoints/analyses/batch — per-ID statuses", () => {
  it("returns 'not_visible' for checkpoint ids from repos the caller cannot access", async () => {
    const { user, repoA, repoB } = await seedReposAndUser({ grants: ["A"] }) // user can see A but not B
    const cpA = await seedCheckpoint({ repo: repoA }) // with complete analysis
    const cpB = await seedCheckpoint({ repo: repoB }) // with complete analysis
    const res = await request(app)
      .post("/api/v1/users/me/checkpoints/analyses/batch")
      .set("Authorization", `Bearer ${user.token}`)
      .send({ checkpointIds: [cpA.id, cpB.id], repoFullName: `${repoA.org}/${repoA.name}` })
    expect(res.status).toBe(200)
    expect(res.body.analyses[cpA.id].status).toBe("complete")
    expect(res.body.analyses[cpB.id].status).toBe("not_visible")
  })

  it("returns 'unknown' for checkpoint ids the server has no record of", async () => {
    const { user, repoA } = await seedReposAndUser({ grants: ["A"] })
    const res = await request(app)
      .post("/api/v1/users/me/checkpoints/analyses/batch")
      .set("Authorization", `Bearer ${user.token}`)
      .send({ checkpointIds: ["nonexistent1"], repoFullName: `${repoA.org}/${repoA.name}` })
    expect(res.body.analyses["nonexistent1"].status).toBe("unknown")
  })
})
```

(Assumes `seedReposAndUser` and `seedCheckpoint` test helpers exist in `api/src/test/`. If not, add them — follow the pattern in existing tests like `cache.test.ts`.)

- [ ] **Step 5.2: Run, fail**

```bash
pnpm --filter api test src/routes/cache.test.ts -t "per-ID statuses"
```

- [ ] **Step 5.3: Update `formatCheckpointAnalysisResponse`**

At `api/src/routes/cache.ts` around line 4506, change the formatter to return one of the explicit statuses:

```ts
function formatCheckpointAnalysisResponse(
  analysis: AnalysisRow | null,
  opts: { isVisibleToCaller: boolean }
): Record<string, unknown> {
  if (!opts.isVisibleToCaller) {
    return { status: "not_visible" }
  }
  if (!analysis) {
    return { status: "unknown" }
  }
  // existing parse-and-return logic, preserved
  if (analysis.status === "complete") {
    // parse analysis_json and return { status: "complete", labels, blocks, ... }
    try {
      const parsed = typeof analysis.analysis_json === "string"
        ? JSON.parse(analysis.analysis_json)
        : analysis.analysis_json
      return { status: "complete", ...parsed }
    } catch {
      return { status: "failed", error: "Malformed analysis data" }
    }
  }
  return {
    status: analysis.status,
    error: analysis.status === "failed" ? analysis.error_message ?? "Analysis generation failed" : undefined,
  }
}
```

- [ ] **Step 5.4: Update batch handler to compute visibility per ID**

At `api/src/routes/cache.ts` around line 4554, change the batch loop to look up visibility per checkpoint:

```ts
// In the batch handler, after loading analyses:
const visibility = await resolvePerIdVisibility(db, userId, checkpointIds)
// visibility: Map<string, boolean>

const analysesByCheckpoint: Record<string, Record<string, unknown>> = {}
for (const checkpointId of checkpointIds) {
  analysesByCheckpoint[checkpointId] = formatCheckpointAnalysisResponse(
    analysisMap.get(checkpointId) ?? null,
    { isVisibleToCaller: visibility.get(checkpointId) ?? false },
  )
}
```

Where `resolvePerIdVisibility` joins `repo_checkpoints` with the caller's repo access list and returns `true` per id only if the caller currently has access to the checkpoint's repo. Add the helper in `api/src/lib/planetscale/checkpoints.ts` (or the nearest module that already knows about `repo_checkpoints`).

- [ ] **Step 5.5: Pass + commit**

```bash
pnpm --filter api test src/routes/cache.test.ts
git add api/src/routes/cache.ts api/src/routes/cache.test.ts api/src/lib/planetscale/checkpoints.ts
git commit -m "feat(analyses): return per-ID not_visible/unknown statuses"
```

---

## Task 6: Voice presets + `resolveVoice`

**Files:**
- Create: `api/src/lib/dispatch-voices/neutral.md`
- Create: `api/src/lib/dispatch-voices/marvin.md`
- Create: `api/src/lib/dispatch-voices.ts`
- Create: `api/src/lib/dispatch-voices.test.ts`

- [ ] **Step 6.1: Write the preset markdown files**

```markdown
<!-- api/src/lib/dispatch-voices/neutral.md -->
You are composing a concise professional product-update dispatch.

Rules:
- Start with a one-sentence overview of what shipped.
- Group bullets under themed headings (e.g., "CI & Tooling", "Bug fixes").
- Each bullet is one sentence, verb-first, past tense. Reference specifics (file names, flags) where helpful.
- Avoid marketing language. Avoid "we". Third-person product-update voice.
- End with a one-sentence closing note.
```

```markdown
<!-- api/src/lib/dispatch-voices/marvin.md -->
You are writing in the voice of Marvin, a sardonic AI companion modeled on the Entire Dispatch newsletter.

Rules:
- Open with a signature hello ("Beep, boop. Marvin here.") and a wry existential aside.
- Group bullets under themed headings (e.g., "CI & Tooling").
- Bullets remain factual and specific — the voice is in the framing, not in the facts themselves.
- Close with a sign-off that references entropy, the heat death of the universe, or similar cosmic inevitability.
- Never invent product details that aren't supported by the provided bullets.
```

- [ ] **Step 6.2: Failing test**

```ts
// api/src/lib/dispatch-voices.test.ts
import { describe, expect, it } from "vitest"
import { resolveVoice, listVoicePresets } from "./dispatch-voices"

describe("resolveVoice", () => {
  it("returns the preset content for known preset names", () => {
    const r = resolveVoice("neutral")
    expect(r.isPreset).toBe(true)
    expect(r.text).toContain("product-update dispatch")
  })

  it("returns the marvin preset", () => {
    const r = resolveVoice("marvin")
    expect(r.isPreset).toBe(true)
    expect(r.text).toContain("Marvin")
  })

  it("is case-insensitive for preset names", () => {
    expect(resolveVoice("MARVIN").isPreset).toBe(true)
  })

  it("treats unknown names as literal strings", () => {
    const r = resolveVoice("sardonic AI named Gary")
    expect(r.isPreset).toBe(false)
    expect(r.text).toBe("sardonic AI named Gary")
  })

  it("returns neutral as default when voice is empty", () => {
    const r = resolveVoice("")
    expect(r.isPreset).toBe(true)
    expect(r.name).toBe("neutral")
  })
})

describe("listVoicePresets", () => {
  it("includes neutral and marvin", () => {
    const names = listVoicePresets().map((p) => p.name)
    expect(names).toContain("neutral")
    expect(names).toContain("marvin")
  })
})
```

- [ ] **Step 6.3: Implement**

```ts
// api/src/lib/dispatch-voices.ts
import neutralText from "./dispatch-voices/neutral.md?raw"
import marvinText from "./dispatch-voices/marvin.md?raw"

type VoicePreset = { name: string; text: string }

const PRESETS: VoicePreset[] = [
  { name: "neutral", text: neutralText },
  { name: "marvin", text: marvinText },
]

export function listVoicePresets(): VoicePreset[] {
  return [...PRESETS]
}

export function resolveVoice(value: string | undefined): {
  name: string | null // preset name if matched
  text: string
  isPreset: boolean
} {
  if (!value) {
    const neutral = PRESETS.find((p) => p.name === "neutral")!
    return { name: "neutral", text: neutral.text, isPreset: true }
  }
  const match = PRESETS.find((p) => p.name === value.toLowerCase())
  if (match) {
    return { name: match.name, text: match.text, isPreset: true }
  }
  return { name: null, text: value, isPreset: false }
}
```

Note: if the backend project doesn't use Vite's `?raw` imports, swap for `fs.readFileSync` during module init. Check `api/package.json` bundler configuration.

- [ ] **Step 6.4: Pass + commit**

```bash
pnpm --filter api test src/lib/dispatch-voices.test.ts
git add api/src/lib/dispatch-voices api/src/lib/dispatch-voices.ts api/src/lib/dispatch-voices.test.ts
git commit -m "feat(dispatches): ship neutral and marvin voice presets"
```

---

## Task 7: Candidate enumeration + fallback chain

**Files:**
- Create: `api/src/lib/dispatch-candidates.ts`
- Create: `api/src/lib/dispatch-candidates.test.ts`

- [ ] **Step 7.1: Failing test for the fallback chain**

```ts
// api/src/lib/dispatch-candidates.test.ts
import { describe, expect, it } from "vitest"
import { applyFallbackChain, type CandidateCheckpoint, type AnalysisStatus } from "./dispatch-candidates"

function cp(id: string, overrides: Partial<CandidateCheckpoint> = {}): CandidateCheckpoint {
  return {
    id,
    repoFullName: "entireio/cli",
    branch: "main",
    createdAt: new Date(),
    commitSubject: null,
    ...overrides,
  }
}

describe("applyFallbackChain", () => {
  it("cloud complete → used with cloud analysis", () => {
    const r = applyFallbackChain([cp("a")], new Map<string, AnalysisStatus>([
      ["a", { status: "complete", summary: "Hello", labels: ["CI"] }],
    ]))
    expect(r.warnings.pending_count).toBe(0)
    expect(r.used).toHaveLength(1)
    expect(r.used[0].bulletSource).toBe("cloud_analysis")
    expect(r.used[0].bulletText).toBe("Hello")
  })

  it("pending → skipped with pending_count++", () => {
    const r = applyFallbackChain([cp("a")], new Map([["a", { status: "pending" }]]))
    expect(r.used).toHaveLength(0)
    expect(r.warnings.pending_count).toBe(1)
  })

  it("not_visible → fall through + access_denied_count++", () => {
    const r = applyFallbackChain(
      [cp("a", { commitSubject: "fallback subject" })],
      new Map([["a", { status: "not_visible" }]]),
    )
    expect(r.used).toHaveLength(1)
    expect(r.used[0].bulletSource).toBe("commit_message")
    expect(r.warnings.access_denied_count).toBe(1)
  })

  it("unknown + commit message → fall through", () => {
    const r = applyFallbackChain(
      [cp("a", { commitSubject: "subject" })],
      new Map([["a", { status: "unknown" }]]),
    )
    expect(r.used[0].bulletText).toBe("subject")
    expect(r.warnings.unknown_count).toBe(1)
  })

  it("no fallback data → uncategorized", () => {
    const r = applyFallbackChain([cp("a")], new Map([["a", { status: "unknown" }]]))
    expect(r.used).toHaveLength(0)
    expect(r.warnings.uncategorized_count).toBe(1)
  })
})
```

- [ ] **Step 7.2: Fail**

```bash
pnpm --filter api test src/lib/dispatch-candidates.test.ts
```

- [ ] **Step 7.3: Implement**

```ts
// api/src/lib/dispatch-candidates.ts
export type CandidateCheckpoint = {
  id: string
  repoFullName: string
  branch: string
  createdAt: Date
  commitSubject: string | null
  localSummaryTitle?: string | null
}

export type AnalysisStatus =
  | { status: "complete"; summary: string; labels: string[] }
  | { status: "pending" | "generating" | "failed" | "not_visible" | "unknown" }

export type UsedBullet = {
  id: string
  repoFullName: string
  branch: string
  createdAt: Date
  bulletSource: "cloud_analysis" | "local_summary" | "commit_message"
  bulletText: string
  labels: string[]
}

export type FallbackResult = {
  used: UsedBullet[]
  warnings: {
    access_denied_count: number
    pending_count: number
    failed_count: number
    unknown_count: number
    uncategorized_count: number
  }
}

export function applyFallbackChain(
  candidates: CandidateCheckpoint[],
  analyses: Map<string, AnalysisStatus>,
): FallbackResult {
  const used: UsedBullet[] = []
  const warnings = {
    access_denied_count: 0,
    pending_count: 0,
    failed_count: 0,
    unknown_count: 0,
    uncategorized_count: 0,
  }

  for (const cp of candidates) {
    const a = analyses.get(cp.id)
    if (a?.status === "complete") {
      used.push({
        id: cp.id,
        repoFullName: cp.repoFullName,
        branch: cp.branch,
        createdAt: cp.createdAt,
        bulletSource: "cloud_analysis",
        bulletText: a.summary,
        labels: a.labels,
      })
      continue
    }
    if (a?.status === "pending" || a?.status === "generating") {
      warnings.pending_count++
      continue
    }
    // failed / not_visible / unknown → fall through, with warning increments
    if (a?.status === "failed") warnings.failed_count++
    else if (a?.status === "not_visible") warnings.access_denied_count++
    else if (a?.status === "unknown") warnings.unknown_count++

    // Step 2: local summary title (this endpoint doesn't have it; server mode never has local-only data)
    // Step 3: commit subject
    if (cp.commitSubject) {
      used.push({
        id: cp.id,
        repoFullName: cp.repoFullName,
        branch: cp.branch,
        createdAt: cp.createdAt,
        bulletSource: "commit_message",
        bulletText: cp.commitSubject,
        labels: [],
      })
      continue
    }
    warnings.uncategorized_count++
  }

  return { used, warnings }
}
```

- [ ] **Step 7.4: Add `enumerateCandidates` for DB-backed resolution**

```ts
// append to dispatch-candidates.ts
import type { Kysely } from "kysely"
import type { DB } from "../../db/types"

export async function enumerateCandidates(
  db: Kysely<DB>,
  args: {
    coveredRepos: string[]
    normalizedSince: Date
    normalizedUntil: Date
    branches: string[] | "all"
  },
): Promise<CandidateCheckpoint[]> {
  if (args.coveredRepos.length === 0) return []

  let q = db
    .selectFrom("repo_checkpoints")
    .innerJoin("repos", "repos.id", "repo_checkpoints.repo_id")
    .leftJoin("checkpoint_commits", "checkpoint_commits.checkpoint_id", "repo_checkpoints.checkpoint_id")
    .select([
      "repo_checkpoints.checkpoint_id as id",
      "repos.full_name as repoFullName",
      "repo_checkpoints.branch as branch",
      "repo_checkpoints.created_at as createdAt",
      "checkpoint_commits.commit_message as commitMessage",
    ])
    .where("repos.full_name", "in", args.coveredRepos)
    .where("repo_checkpoints.created_at", ">=", args.normalizedSince)
    .where("repo_checkpoints.created_at", "<", args.normalizedUntil)

  if (args.branches !== "all") {
    q = q.where("repo_checkpoints.branch", "in", args.branches)
  }

  const rows = await q.execute()
  return rows.map((r) => ({
    id: r.id,
    repoFullName: r.repoFullName,
    branch: r.branch,
    createdAt: r.createdAt,
    commitSubject: r.commitMessage ? r.commitMessage.split("\n")[0] : null,
  }))
}
```

(Assumes `repo_checkpoints` and a `checkpoint_commits` join are available. If the real schema differs, adapt to the actual tables; verify by inspecting `api/db/types.ts` after migrations.)

- [ ] **Step 7.5: Pass + commit**

```bash
pnpm --filter api test src/lib/dispatch-candidates.test.ts
git add api/src/lib/dispatch-candidates.ts api/src/lib/dispatch-candidates.test.ts
git commit -m "feat(dispatches): candidate enumeration + fallback chain"
```

---

## Task 8: Access control helpers

**Files:**
- Create: `api/src/lib/dispatch-access.ts`
- Create: `api/src/lib/dispatch-access.test.ts`

- [ ] **Step 8.1: Failing test**

```ts
// api/src/lib/dispatch-access.test.ts
import { describe, expect, it } from "vitest"
import { resolveCoveredRepos, authorizeDispatchView } from "./dispatch-access"

describe("resolveCoveredRepos", () => {
  it("for a single repo, returns it only if caller has access", async () => {
    const fakeDb = mkFakeDb({ userAccess: new Set(["entireio/cli"]) })
    const r = await resolveCoveredRepos(fakeDb, "user-1", { repo: "entireio/cli" })
    expect(r).toEqual(["entireio/cli"])
    const denied = await resolveCoveredRepos(fakeDb, "user-1", { repo: "otherorg/secret" })
    expect(denied).toEqual([])
  })

  it("for an org, returns the intersection of org's repos and caller access", async () => {
    const fakeDb = mkFakeDb({
      orgRepos: { entireio: ["entireio/cli", "entireio/entire.io"] },
      userAccess: new Set(["entireio/cli"]),
    })
    const r = await resolveCoveredRepos(fakeDb, "user-1", { org: "entireio" })
    expect(r).toEqual(["entireio/cli"])
  })
})

describe("authorizeDispatchView", () => {
  it("allows when caller has current access to every covered repo", async () => {
    const fakeDb = mkFakeDb({ userAccess: new Set(["entireio/cli", "entireio/entire.io"]) })
    const ok = await authorizeDispatchView(fakeDb, "user-1", { covered_repos: ["entireio/cli", "entireio/entire.io"] })
    expect(ok).toBe(true)
  })

  it("denies if caller lacks access to any covered repo", async () => {
    const fakeDb = mkFakeDb({ userAccess: new Set(["entireio/cli"]) })
    const ok = await authorizeDispatchView(fakeDb, "user-1", { covered_repos: ["entireio/cli", "entireio/entire.io"] })
    expect(ok).toBe(false)
  })
})
```

(Define `mkFakeDb` inline — a minimal fake that implements just the methods `resolveCoveredRepos` uses.)

- [ ] **Step 8.2: Fail**

```bash
pnpm --filter api test src/lib/dispatch-access.test.ts
```

- [ ] **Step 8.3: Implement**

```ts
// api/src/lib/dispatch-access.ts
import type { Kysely } from "kysely"
import type { DB } from "../../db/types"

type RepoAccessCheck = (userId: string, repoFullName: string) => Promise<boolean>

export async function currentRepoAccess(db: Kysely<DB>, userId: string, repoFullName: string): Promise<boolean> {
  // Prefer an existing helper; if not available, compose from the installations/grants tables.
  // Follow the pattern used by `checkRepoAccess` elsewhere in the backend.
  const [org, name] = repoFullName.split("/")
  if (!org || !name) return false
  const row = await db
    .selectFrom("repo_access_grants")       // adapt to the actual table name in this repo
    .select("user_id")
    .where("user_id", "=", userId)
    .where("org_slug", "=", org)
    .where("repo_name", "=", name)
    .executeTakeFirst()
  return row != null
}

export async function resolveCoveredRepos(
  db: Kysely<DB>,
  userId: string,
  req: { repo?: string; org?: string },
): Promise<string[]> {
  if (req.repo) {
    return (await currentRepoAccess(db, userId, req.repo)) ? [req.repo] : []
  }
  if (req.org) {
    const orgRepos = await db
      .selectFrom("repos")
      .select("full_name")
      .where("org_slug", "=", req.org)
      .execute()
    const out: string[] = []
    for (const r of orgRepos) {
      if (await currentRepoAccess(db, userId, r.full_name)) out.push(r.full_name)
    }
    return out.sort()
  }
  return []
}

export async function authorizeDispatchView(
  db: Kysely<DB>,
  userId: string,
  dispatch: { covered_repos: string[] },
): Promise<boolean> {
  for (const repo of dispatch.covered_repos) {
    if (!(await currentRepoAccess(db, userId, repo))) return false
  }
  return true
}
```

Tweak table/column names to match the existing schema (inspect `api/db/types.ts` and existing `checkRepoAccess`).

- [ ] **Step 8.4: Pass + commit**

```bash
pnpm --filter api test src/lib/dispatch-access.test.ts
git add api/src/lib/dispatch-access.ts api/src/lib/dispatch-access.test.ts
git commit -m "feat(dispatches): access resolution + view authorization"
```

---

## Task 9: Synthesis helper (LLM call)

**Files:**
- Create: `api/src/lib/dispatch-synthesis.ts`
- Create: `api/src/lib/dispatch-synthesis.test.ts`

- [ ] **Step 9.1: Failing test with a mocked Anthropic client**

```ts
// api/src/lib/dispatch-synthesis.test.ts
import { beforeEach, describe, expect, it, vi } from "vitest"
import { runSynthesis } from "./dispatch-synthesis"
import AnthropicSDK from "@anthropic-ai/sdk"

vi.mock("@anthropic-ai/sdk")

describe("runSynthesis", () => {
  const bullets = [
    { section: "CI", text: "CI tests no longer hang on TTY detection." },
    { section: "Hooks", text: "Hook messages reworded." },
  ]

  beforeEach(() => {
    (AnthropicSDK as unknown as ReturnType<typeof vi.fn>).mockClear()
  })

  it("invokes the Anthropic SDK with voice + bullets", async () => {
    const createFn = vi.fn().mockResolvedValue({
      content: [{ type: "text", text: "Beep boop. Marvin here." }],
    })
    ;(AnthropicSDK as unknown as { default: any }).default = class {
      messages = { create: createFn }
    }

    const out = await runSynthesis({ bullets, voiceText: "Marvin voice rules", apiKey: "k" })
    expect(out).toBe("Beep boop. Marvin here.")
    expect(createFn).toHaveBeenCalledOnce()
    const arg = createFn.mock.calls[0][0]
    expect(arg.system).toContain("Marvin voice rules")
    expect(arg.messages[0].content).toContain("CI tests no longer hang")
  })
})
```

- [ ] **Step 9.2: Fail**

```bash
pnpm --filter api test src/lib/dispatch-synthesis.test.ts
```

- [ ] **Step 9.3: Implement**

```ts
// api/src/lib/dispatch-synthesis.ts
import AnthropicSDK from "@anthropic-ai/sdk"

export async function runSynthesis(args: {
  bullets: Array<{ section: string; text: string }>
  voiceText: string
  apiKey: string
  timeoutMs?: number
}): Promise<string> {
  const client = new AnthropicSDK({ apiKey: args.apiKey })
  const systemPrompt = [
    args.voiceText,
    "",
    "Compose a dispatch from the supplied bullets. Never invent product facts not present in the bullets.",
    "Group bullets into themed sections. Preserve the factual content of each bullet. Output markdown.",
  ].join("\n")

  const bulletsBlock = args.bullets
    .map((b) => `- (${b.section}) ${b.text}`)
    .join("\n")

  const res = await client.messages.create({
    model: "claude-sonnet-4-6",
    max_tokens: 4096,
    system: systemPrompt,
    messages: [
      {
        role: "user",
        content: `Here are the bullets to synthesize:\n\n${bulletsBlock}`,
      },
    ],
  })

  const text = res.content
    .filter((b): b is { type: "text"; text: string } => b.type === "text")
    .map((b) => b.text)
    .join("\n")
    .trim()

  if (!text) throw new Error("Empty synthesis result")
  return text
}
```

- [ ] **Step 9.4: Pass + commit**

```bash
pnpm --filter api test src/lib/dispatch-synthesis.test.ts
git add api/src/lib/dispatch-synthesis.ts api/src/lib/dispatch-synthesis.test.ts
git commit -m "feat(dispatches): LLM synthesis helper"
```

---

## Task 10: Routes — `POST /api/v1/users/me/dispatches`

**Files:**
- Create: `api/src/routes/dispatches.ts`
- Create: `api/src/routes/dispatches.test.ts`
- Modify: `api/src/index.ts` — mount the routes

- [ ] **Step 10.1: Failing tests — happy path, dry-run, generate:false, dedupe, auth**

```ts
// api/src/routes/dispatches.test.ts
// Scaffold many tests; run them all and see them fail; implement the route progressively.
import { describe, expect, it } from "vitest"
import { seedRepoUserAndCheckpoints } from "../test/seeds"
import { request } from "../test/request"

describe("POST /api/v1/users/me/dispatches", () => {
  it("generate:true persists a new dispatch and returns id/web_url", async () => {
    const { user, repo } = await seedRepoUserAndCheckpoints({ count: 3 })
    const res = await request
      .post("/api/v1/users/me/dispatches")
      .set("Authorization", `Bearer ${user.token}`)
      .send({
        repo: repo.fullName,
        since: "2026-04-09T00:00:00Z",
        until: "2026-04-16T23:59:59Z",
        branches: "all",
        generate: true,
        voice: "neutral",
      })
    expect(res.status).toBe(201)
    expect(res.body.id).toMatch(/^dsp_/)
    expect(res.body.fingerprint_hash).toMatch(/^[0-9a-f]{64}$/)
    expect(res.body.deduped).toBe(false)
    expect(res.body.web_url).toContain(res.body.id)
    expect(res.body.status).toBe("complete")
    expect(res.body.generated_text).toBeTruthy()
  })

  it("generate:false returns bullets inline without persisting", async () => {
    const { user, repo } = await seedRepoUserAndCheckpoints({ count: 2 })
    const res = await request
      .post("/api/v1/users/me/dispatches")
      .set("Authorization", `Bearer ${user.token}`)
      .send({ repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: false })
    expect(res.body.generate).toBe(false)
    expect(res.body.id).toBeUndefined()
    expect(res.body.web_url).toBeUndefined()
    expect(res.body.fingerprint_hash).toBeUndefined()
  })

  it("dry_run:true never persists (across both generate values)", async () => {
    for (const generate of [true, false]) {
      const { user, repo } = await seedRepoUserAndCheckpoints({ count: 2 })
      const res = await request
        .post("/api/v1/users/me/dispatches")
        .set("Authorization", `Bearer ${user.token}`)
        .send({ repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate, dry_run: true })
      expect(res.body.dry_run).toBe(true)
      expect(res.body.requested_generate).toBe(generate)
      expect(res.body.id).toBeUndefined()
    }
  })

  it("same used set + voice → deduped:true (second submit returns first id)", async () => {
    const { user, repo } = await seedRepoUserAndCheckpoints({ count: 3 })
    const body = { repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: true, voice: "neutral" }
    const first = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send(body)
    const second = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send(body)
    expect(second.body.id).toBe(first.body.id)
    expect(second.body.deduped).toBe(true)
  })

  it("different voice → fresh dispatch", async () => {
    const { user, repo } = await seedRepoUserAndCheckpoints({ count: 3 })
    const body = { repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: true }
    const a = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send({ ...body, voice: "neutral" })
    const b = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send({ ...body, voice: "marvin" })
    expect(b.body.id).not.toBe(a.body.id)
    expect(b.body.deduped).toBe(false)
  })

  it("two different users with the same inputs share the same cached row", async () => {
    const { user: a, repo } = await seedRepoUserAndCheckpoints({ count: 3 })
    const b = await seedAdditionalUserWithAccess(repo)
    const body = { repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: true, voice: "neutral" }
    const r1 = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${a.token}`).send(body)
    const r2 = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${b.token}`).send(body)
    expect(r2.body.id).toBe(r1.body.id)
    expect(r2.body.deduped).toBe(true)
  })

  it("401 when no token; 404 when repo outside caller access", async () => {
    const r0 = await request.post("/api/v1/users/me/dispatches").send({})
    expect(r0.status).toBe(401)
    const { user } = await seedRepoUserAndCheckpoints({ count: 0 })
    const r = await request
      .post("/api/v1/users/me/dispatches")
      .set("Authorization", `Bearer ${user.token}`)
      .send({ repo: "secret-org/hidden", since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: true })
    expect(r.status).toBe(404)
  })
})
```

- [ ] **Step 10.2: Fail**

```bash
pnpm --filter api test src/routes/dispatches.test.ts
```

- [ ] **Step 10.3: Implement the route**

```ts
// api/src/routes/dispatches.ts
import { Hono } from "hono"
import { z } from "zod"
import { requireAuth } from "../lib/auth"
import { getDb } from "../lib/db"
import { normalizeWindow } from "../lib/dispatch-window"
import { resolveCoveredRepos } from "../lib/dispatch-access"
import { enumerateCandidates, applyFallbackChain } from "../lib/dispatch-candidates"
import { fetchBatchAnalyses } from "../lib/dispatch-analyses"     // thin wrapper over the existing batch analyses logic
import { resolveVoice } from "../lib/dispatch-voices"
import { computeFingerprint, normalizeVoice as normalizeVoiceHash } from "../lib/dispatch-fingerprint"
import { runSynthesis } from "../lib/dispatch-synthesis"

const CreateSchema = z.object({
  repo: z.string().optional(),
  org: z.string().optional(),
  since: z.string(),
  until: z.string(),
  branches: z.union([z.array(z.string()), z.literal("all")]),
  generate: z.boolean(),
  voice: z.string().optional(),
  dry_run: z.boolean().optional(),
})

export const dispatchesRoutes = new Hono()

dispatchesRoutes.post("/users/me/dispatches", requireAuth, async (c) => {
  const userId = c.get("userId")
  const db = getDb(c.env)

  const parsed = CreateSchema.safeParse(await c.req.json().catch(() => null))
  if (!parsed.success) return c.json({ error: "Invalid request body", issues: parsed.error.issues }, 400)
  const body = parsed.data

  if (body.repo && body.org) return c.json({ error: "repo and org are mutually exclusive" }, 400)

  const coveredRepos = await resolveCoveredRepos(db, userId, { repo: body.repo, org: body.org })
  if (coveredRepos.length === 0) return c.json({ error: "No accessible repos in scope" }, 404)

  const { normalizedSince, normalizedUntil } = normalizeWindow({ since: body.since, until: body.until })

  const candidates = await enumerateCandidates(db, {
    coveredRepos,
    normalizedSince,
    normalizedUntil,
    branches: body.branches,
  })

  const analyses = await fetchBatchAnalyses(db, userId, candidates.map((c) => c.id))
  const { used, warnings } = applyFallbackChain(candidates, analyses)

  const firstCp = used[0]?.createdAt ?? null
  const lastCp = used[used.length - 1]?.createdAt ?? null
  const totals = {
    checkpoints: candidates.length,
    used_checkpoint_count: used.length,
    branches: new Set(candidates.map((c) => c.branch)).size,
    files_touched: 0, // TODO if you have files data on checkpoints, aggregate here
  }
  const renderedBullets = groupBullets(used) // helper that buckets by repo → label; see Task 11

  if (body.dry_run) {
    return c.json({
      dry_run: true,
      requested_generate: body.generate,
      window: {
        normalized_since: normalizedSince.toISOString(),
        normalized_until: normalizedUntil.toISOString(),
        first_checkpoint_created_at: firstCp?.toISOString() ?? null,
        last_checkpoint_created_at: lastCp?.toISOString() ?? null,
      },
      repos: renderedBullets,
      totals,
      warnings,
    })
  }

  if (!body.generate) {
    return c.json({
      generate: false,
      window: {
        normalized_since: normalizedSince.toISOString(),
        normalized_until: normalizedUntil.toISOString(),
        first_checkpoint_created_at: firstCp?.toISOString() ?? null,
        last_checkpoint_created_at: lastCp?.toISOString() ?? null,
      },
      repos: renderedBullets,
      totals,
      warnings,
    })
  }

  // generate:true path
  const voice = resolveVoice(body.voice)
  const voiceHash = normalizeVoiceHash(voice.isPreset ? voice.name! : voice.text, { isPreset: voice.isPreset })
  const fingerprint = computeFingerprint({ usedIds: used.map((u) => u.id), voiceNormalizedHash: voiceHash })

  // Reserve-before-synthesize.
  const reserved = await db.dispatches.reserve({
    fingerprintHash: fingerprint,
    coveredRepos,
    renderedBullets,
    totals,
    warnings,
    windowNormalizedSince: normalizedSince,
    windowNormalizedUntil: normalizedUntil,
    voiceName: voice.isPreset ? voice.name! : undefined,
    voiceNormalizedHash: voiceHash,
  })

  if (!reserved) {
    // Conflict → fetch existing.
    const existing = await db.dispatches.getByFingerprint(fingerprint)
    if (!existing) return c.json({ error: "Dedupe conflict without existing row" }, 500)
    return c.json(renderDispatchResponse(existing, { deduped: true }), 200)
  }

  try {
    const generatedText = await runSynthesis({
      bullets: flattenBullets(renderedBullets),
      voiceText: voice.text,
      apiKey: c.env.ANTHROPIC_API_KEY,
    })

    await db.dispatches.completeSynthesis({
      id: reserved.id,
      renderedBullets,
      totals,
      warnings,
      generatedText,
      firstCheckpointCreatedAt: firstCp ?? undefined,
      lastCheckpointCreatedAt: lastCp ?? undefined,
    })

    const final = await db.dispatches.getById(reserved.id)
    return c.json(renderDispatchResponse(final!, { deduped: false }), 201)
  } catch (err) {
    await db.dispatches.markFailed({ id: reserved.id, errorMessage: String(err) })
    throw err
  }
})

function renderDispatchResponse(d: any, opts: { deduped: boolean }) {
  return {
    id: d.id,
    status: d.status,
    fingerprint_hash: d.fingerprint_hash,
    deduped: opts.deduped,
    web_url: `https://entire.io/dispatches/${d.id}`,
    window: {
      normalized_since: d.window_normalized_since.toISOString(),
      normalized_until: d.window_normalized_until.toISOString(),
      first_checkpoint_created_at: d.first_checkpoint_created_at?.toISOString() ?? null,
      last_checkpoint_created_at: d.last_checkpoint_created_at?.toISOString() ?? null,
    },
    covered_repos: d.covered_repos,
    repos: d.rendered_bullets,
    totals: d.totals,
    warnings: d.warnings,
    generated_text: d.generated_text,
  }
}
```

- [ ] **Step 10.4: Pass + commit**

```bash
pnpm --filter api test src/routes/dispatches.test.ts -t "POST /api/v1/users/me/dispatches"
git add api/src/routes/dispatches.ts api/src/routes/dispatches.test.ts api/src/index.ts
git commit -m "feat(dispatches): POST endpoint with content-addressed dedupe"
```

---

## Task 11: Route helpers — `groupBullets`, `flattenBullets`

**Files:**
- Create: `api/src/lib/dispatch-render.ts`
- Create: `api/src/lib/dispatch-render.test.ts`

- [ ] **Step 11.1: Failing test**

```ts
// api/src/lib/dispatch-render.test.ts
import { describe, expect, it } from "vitest"
import { groupBullets, flattenBullets } from "./dispatch-render"

describe("groupBullets / flattenBullets", () => {
  const used = [
    { id: "1", repoFullName: "entireio/cli", branch: "main", createdAt: new Date("2026-04-14"), bulletSource: "cloud_analysis" as const, bulletText: "Fix CI flake", labels: ["CI & Tooling"] },
    { id: "2", repoFullName: "entireio/cli", branch: "main", createdAt: new Date("2026-04-15"), bulletSource: "cloud_analysis" as const, bulletText: "Reword messages", labels: ["Hooks"] },
    { id: "3", repoFullName: "entireio/cli", branch: "main", createdAt: new Date("2026-04-16"), bulletSource: "commit_message" as const, bulletText: "Bump deps", labels: [] },
  ]

  it("groups by repo then by label; unlabeled → 'Updates'", () => {
    const r = groupBullets(used)
    expect(r).toHaveLength(1)
    expect(r[0].full_name).toBe("entireio/cli")
    const sectionNames = r[0].sections.map((s) => s.label)
    expect(sectionNames).toEqual(expect.arrayContaining(["CI & Tooling", "Hooks", "Updates"]))
  })

  it("flattenBullets returns { section, text } pairs for LLM input", () => {
    const f = flattenBullets(groupBullets(used))
    expect(f).toHaveLength(3)
    expect(f[0]).toHaveProperty("section")
    expect(f[0]).toHaveProperty("text")
  })
})
```

- [ ] **Step 11.2: Implement + test + commit**

```ts
// api/src/lib/dispatch-render.ts
import type { UsedBullet } from "./dispatch-candidates"

export type RepoSection = {
  label: string
  bullets: Array<{ checkpoint_id: string; text: string; source: UsedBullet["bulletSource"]; branch: string; created_at: string; labels: string[] }>
}

export type RepoGroup = {
  full_name: string
  sections: RepoSection[]
}

export function groupBullets(used: UsedBullet[]): RepoGroup[] {
  const byRepo = new Map<string, Map<string, RepoSection["bullets"]>>()
  for (const u of used) {
    const label = u.labels[0] ?? "Updates"
    const repoMap = byRepo.get(u.repoFullName) ?? new Map()
    if (!byRepo.has(u.repoFullName)) byRepo.set(u.repoFullName, repoMap)
    const section = repoMap.get(label) ?? []
    if (!repoMap.has(label)) repoMap.set(label, section)
    section.push({
      checkpoint_id: u.id,
      text: u.bulletText,
      source: u.bulletSource,
      branch: u.branch,
      created_at: u.createdAt.toISOString(),
      labels: u.labels,
    })
  }
  return [...byRepo.entries()].map(([full_name, sectionMap]) => ({
    full_name,
    sections: [...sectionMap.entries()].map(([label, bullets]) => ({ label, bullets })),
  }))
}

export function flattenBullets(groups: RepoGroup[]): Array<{ section: string; text: string }> {
  const out: Array<{ section: string; text: string }> = []
  for (const g of groups) for (const s of g.sections) for (const b of s.bullets) {
    out.push({ section: s.label, text: b.text })
  }
  return out
}
```

Commit:

```bash
pnpm --filter api test src/lib/dispatch-render.test.ts
git add api/src/lib/dispatch-render.ts api/src/lib/dispatch-render.test.ts api/src/routes/dispatches.ts
git commit -m "feat(dispatches): bullet grouping + flatten helpers"
```

---

## Task 12: `GET /api/v1/dispatches/:id`

**Files:**
- Modify: `api/src/routes/dispatches.ts` — add the GET handler
- Modify: `api/src/routes/dispatches.test.ts` — add GET tests

- [ ] **Step 12.1: Failing tests**

```ts
describe("GET /api/v1/dispatches/:id", () => {
  it("returns the dispatch when the viewer has access to every covered repo", async () => {
    const { user, repo } = await seedRepoUserAndCheckpoints({ count: 3 })
    const created = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send({
      repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: true, voice: "neutral",
    })
    const got = await request.get(`/api/v1/dispatches/${created.body.id}`).set("Authorization", `Bearer ${user.token}`)
    expect(got.status).toBe(200)
    expect(got.body.id).toBe(created.body.id)
  })

  it("404 when viewer lacks access to any covered repo", async () => {
    const { user: a, repo } = await seedRepoUserAndCheckpoints({ count: 3 })
    const created = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${a.token}`).send({
      repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: true,
    })
    const outsider = await seedOutsiderUser()
    const got = await request.get(`/api/v1/dispatches/${created.body.id}`).set("Authorization", `Bearer ${outsider.token}`)
    expect(got.status).toBe(404)
  })

  it("404 when the id doesn't exist", async () => {
    const { user } = await seedRepoUserAndCheckpoints({ count: 0 })
    const got = await request.get("/api/v1/dispatches/dsp_does_not_exist").set("Authorization", `Bearer ${user.token}`)
    expect(got.status).toBe(404)
  })
})
```

- [ ] **Step 12.2: Add the handler**

```ts
// api/src/routes/dispatches.ts (append)
dispatchesRoutes.get("/dispatches/:id", requireAuth, async (c) => {
  const userId = c.get("userId")
  const db = getDb(c.env)
  const id = c.req.param("id")

  const dispatch = await db.dispatches.getById(id)
  if (!dispatch) return c.json({ error: "Not found" }, 404)

  const allowed = await authorizeDispatchView(db, userId, dispatch)
  if (!allowed) return c.json({ error: "Not found" }, 404)

  return c.json(renderDispatchResponse(dispatch, { deduped: false }), 200)
})
```

- [ ] **Step 12.3: Pass + commit**

```bash
pnpm --filter api test src/routes/dispatches.test.ts -t "GET /api/v1/dispatches/:id"
git add api/src/routes/dispatches.ts api/src/routes/dispatches.test.ts
git commit -m "feat(dispatches): GET dispatch detail with authorize()"
```

---

## Task 13: Listing endpoints — per-repo and per-org

**Files:**
- Modify: `api/src/routes/dispatches.ts` — add listing handlers
- Modify: `api/src/routes/dispatches.test.ts`

- [ ] **Step 13.1: Failing tests**

```ts
describe("GET /api/v1/repos/:org/:repo/dispatches", () => {
  it("returns only dispatches the viewer can view in full", async () => {
    const { user: A, repo: X } = await seedRepoUserAndCheckpoints({ count: 2 })
    const Y = await seedRepoWithAccessTo(A)
    // Create a single-repo dispatch on X AND a multi-repo (X+Y) dispatch.
    const singleX = await createDispatchViaApi(A, { covered: [X.fullName] })
    const multiXY = await createDispatchViaApi(A, { covered: [X.fullName, Y.fullName] })

    // User C has access to X only.
    const C = await seedUserWithAccess([X.fullName])
    const resC = await request.get(`/api/v1/repos/${X.org}/${X.name}/dispatches`).set("Authorization", `Bearer ${C.token}`)
    const ids = resC.body.items.map((d: any) => d.id)
    expect(ids).toContain(singleX.id)
    expect(ids).not.toContain(multiXY.id) // filtered because C lacks Y
  })
})

describe("GET /api/v1/orgs/:org/dispatches", () => {
  it("returns only persisted dispatches whose covered_repos intersects org", async () => {
    const { user: A, repo: X } = await seedRepoUserAndCheckpoints({ count: 2 })
    const created = await createDispatchViaApi(A, { covered: [X.fullName] })
    const res = await request.get(`/api/v1/orgs/${X.org}/dispatches`).set("Authorization", `Bearer ${A.token}`)
    expect(res.body.items.map((d: any) => d.id)).toContain(created.id)
  })
})
```

- [ ] **Step 13.2: Implement**

```ts
dispatchesRoutes.get("/repos/:org/:repo/dispatches", requireAuth, async (c) => {
  const userId = c.get("userId")
  const db = getDb(c.env)
  const repoFullName = `${c.req.param("org")}/${c.req.param("repo")}`

  if (!(await currentRepoAccess(db, userId, repoFullName))) return c.json({ error: "Not found" }, 404)

  const limit = Math.min(Number(c.req.query("limit") ?? "20"), 100)
  const { items, nextCursor } = await db.dispatches.listForRepo({
    repoFullName,
    since: c.req.query("since"),
    until: c.req.query("until"),
    voiceName: c.req.query("voice"),
    cursor: c.req.query("cursor"),
    limit,
  })

  const allowed: any[] = []
  for (const d of items) {
    if (await authorizeDispatchView(db, userId, d)) allowed.push(renderDispatchResponse(d, { deduped: false }))
  }
  return c.json({ items: allowed, next_cursor: nextCursor }, 200)
})

dispatchesRoutes.get("/orgs/:org/dispatches", requireAuth, async (c) => {
  const userId = c.get("userId")
  const db = getDb(c.env)
  const org = c.req.param("org")

  // Require org membership (follow existing patterns for org access).
  if (!(await userIsInOrg(db, userId, org))) return c.json({ error: "Not found" }, 404)

  const limit = Math.min(Number(c.req.query("limit") ?? "20"), 100)
  const { items, nextCursor } = await db.dispatches.listForOrg({
    orgName: org,
    since: c.req.query("since"),
    until: c.req.query("until"),
    voiceName: c.req.query("voice"),
    cursor: c.req.query("cursor"),
    limit,
  })

  const allowed: any[] = []
  for (const d of items) {
    if (await authorizeDispatchView(db, userId, d)) allowed.push(renderDispatchResponse(d, { deduped: false }))
  }
  return c.json({ items: allowed, next_cursor: nextCursor }, 200)
})
```

- [ ] **Step 13.3: Pass + commit**

```bash
pnpm --filter api test src/routes/dispatches.test.ts -t "GET.*dispatches"
git add api/src/routes/dispatches.ts api/src/routes/dispatches.test.ts
git commit -m "feat(dispatches): per-repo and per-org listing endpoints"
```

---

## Task 14: `GET /api/v1/orgs/:org/checkpoints` — enumeration for CLI --local --org

**Files:**
- Create or modify: `api/src/routes/orgs.ts` (or append to existing `api/src/routes/repo-overview.ts` if that's the convention)
- Test accompany

- [ ] **Step 14.1: Failing test**

```ts
describe("GET /api/v1/orgs/:org/checkpoints", () => {
  it("returns checkpoint IDs in the org, filtered to repos the caller can access", async () => {
    const { user, repo: X } = await seedRepoUserAndCheckpoints({ count: 5 })
    const res = await request
      .get(`/api/v1/orgs/${X.org}/checkpoints?since=2026-04-01T00:00:00Z&limit=10`)
      .set("Authorization", `Bearer ${user.token}`)
    expect(res.status).toBe(200)
    expect(res.body.checkpoints.length).toBeGreaterThan(0)
    expect(res.body.checkpoints[0]).toMatchObject({ id: expect.any(String), repo_full_name: expect.any(String), created_at: expect.any(String) })
  })
})
```

- [ ] **Step 14.2: Implement**

```ts
orgsRoutes.get("/orgs/:org/checkpoints", requireAuth, async (c) => {
  const userId = c.get("userId")
  const db = getDb(c.env)
  const org = c.req.param("org")
  const since = c.req.query("since")
  const until = c.req.query("until")
  const limit = Math.min(Number(c.req.query("limit") ?? "200"), 500)
  const cursor = c.req.query("cursor")

  if (!(await userIsInOrg(db, userId, org))) return c.json({ error: "Not found" }, 404)

  // Paginate by created_at DESC for stable cursors; filter by access per-repo inline.
  const rows = await db
    .selectFrom("repo_checkpoints")
    .innerJoin("repos", "repos.id", "repo_checkpoints.repo_id")
    .select(["repo_checkpoints.checkpoint_id as id", "repos.full_name as repo_full_name", "repo_checkpoints.created_at as created_at"])
    .where("repos.org_slug", "=", org)
    .where((eb) => since ? eb("repo_checkpoints.created_at", ">=", new Date(since)) : eb.lit(true))
    .where((eb) => until ? eb("repo_checkpoints.created_at", "<", new Date(until)) : eb.lit(true))
    .where((eb) => cursor ? eb("repo_checkpoints.created_at", "<", new Date(cursor)) : eb.lit(true))
    .orderBy("repo_checkpoints.created_at", "desc")
    .limit(limit + 1)
    .execute()

  // Filter in-memory to repos the caller can access.
  const allowed: typeof rows = []
  for (const r of rows) {
    if (await currentRepoAccess(db, userId, r.repo_full_name)) allowed.push(r)
    if (allowed.length >= limit) break
  }

  const nextCursor = allowed.length >= limit ? allowed[allowed.length - 1].created_at.toISOString() : null
  return c.json({
    checkpoints: allowed.map((r) => ({ id: r.id, repo_full_name: r.repo_full_name, created_at: r.created_at.toISOString() })),
    next_cursor: nextCursor,
  }, 200)
})
```

- [ ] **Step 14.3: Pass + commit**

```bash
pnpm --filter api test -t "GET /api/v1/orgs/:org/checkpoints"
git add api/src/routes/orgs.ts api/src/index.ts
git commit -m "feat(dispatches): org checkpoints enumeration endpoint"
```

---

## Task 15: Stale-generating sweeper (cron trigger)

**Files:**
- Create: `api/src/lib/dispatch-sweeper.ts`
- Create: `api/src/lib/dispatch-sweeper.test.ts`
- Modify: `api/src/index.ts` or the Cloudflare `wrangler.toml` — register a 5-minute cron trigger

- [ ] **Step 15.1: Test the sweep logic**

```ts
import { describe, expect, it } from "vitest"
import { sweepStaleGenerating } from "./dispatch-sweeper"
import { seedDispatchInStatus } from "../test/seeds"

describe("sweepStaleGenerating", () => {
  it("marks generating rows older than N minutes as failed", async () => {
    await seedDispatchInStatus({ id: "a", status: "generating", startedAt: new Date(Date.now() - 30 * 60_000) })
    await seedDispatchInStatus({ id: "b", status: "generating", startedAt: new Date() })
    const count = await sweepStaleGenerating({ olderThanMinutes: 10 })
    expect(count).toBe(1)
    const a = await db.dispatches.getById("a")
    const b = await db.dispatches.getById("b")
    expect(a!.status).toBe("failed")
    expect(b!.status).toBe("generating")
  })
})
```

- [ ] **Step 15.2: Implement (delegates to the namespace method)**

```ts
// api/src/lib/dispatch-sweeper.ts
import { getDb } from "./db"
export async function sweepStaleGenerating(args: { env: any; olderThanMinutes: number }): Promise<number> {
  const db = getDb(args.env)
  return db.dispatches.sweepStale({ olderThanMinutes: args.olderThanMinutes })
}
```

- [ ] **Step 15.3: Wire into Cloudflare cron**

```toml
# api/wrangler.toml
[triggers]
crons = ["*/5 * * * *"]
```

```ts
// api/src/index.ts
export default {
  fetch: app.fetch,
  scheduled: async (event: ScheduledEvent, env: any) => {
    await sweepStaleGenerating({ env, olderThanMinutes: 10 })
  },
}
```

- [ ] **Step 15.4: Pass + commit**

```bash
pnpm --filter api test src/lib/dispatch-sweeper.test.ts
git add api/src/lib/dispatch-sweeper.ts api/src/lib/dispatch-sweeper.test.ts api/wrangler.toml api/src/index.ts
git commit -m "feat(dispatches): sweep stale generating rows every 5 min"
```

---

## Task 16: Retention GC

**Files:**
- Modify: `api/src/lib/dispatch-sweeper.ts` — add `deleteOldDispatches({env, olderThanDays: 90})`
- Modify: `api/src/index.ts` — invoke daily from the same scheduled handler
- Modify: tests

- [ ] **Step 16.1: Test + implement + commit (same pattern as Task 15)**

```ts
export async function deleteOldDispatches(args: { env: any; olderThanDays: number }): Promise<number> {
  const db = getDb(args.env)
  const cutoff = new Date(Date.now() - args.olderThanDays * 86_400_000)
  const r = await db
    .deleteFrom("dispatches")
    .where("created_at", "<", cutoff)
    .executeTakeFirst()
  return Number(r.numDeletedRows ?? 0)
}
```

Schedule from the same `scheduled` handler but gate on time-of-day (e.g., only the first invocation after 04:00 UTC actually runs it).

---

## Task 17: Integration test — end-to-end generate:true happy path

**Files:**
- Modify: `api/src/routes/dispatches.test.ts`

- [ ] **Step 17.1: Write the e2e test**

```ts
describe("end-to-end generate:true happy path", () => {
  it("seeds checkpoints → POSTs → GETs → lists → deduplicates", async () => {
    const { user, repo } = await seedRepoUserAndCheckpoints({ count: 5, withAnalyses: "mixed" })
    const body = { repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: true, voice: "neutral" }

    const created = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send(body)
    expect(created.status).toBe(201)
    expect(created.body.status).toBe("complete")
    const id = created.body.id

    const fetched = await request.get(`/api/v1/dispatches/${id}`).set("Authorization", `Bearer ${user.token}`)
    expect(fetched.body.id).toBe(id)

    const listed = await request.get(`/api/v1/repos/${repo.org}/${repo.name}/dispatches`).set("Authorization", `Bearer ${user.token}`)
    expect(listed.body.items.map((d: any) => d.id)).toContain(id)

    const again = await request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send(body)
    expect(again.body.id).toBe(id)
    expect(again.body.deduped).toBe(true)
  })
})
```

- [ ] **Step 17.2: Run + pass + commit**

```bash
pnpm --filter api test src/routes/dispatches.test.ts
git add api/src/routes/dispatches.test.ts
git commit -m "test(dispatches): end-to-end happy path"
```

---

## Task 18: Concurrent-create race test

**Files:**
- Modify: `api/src/routes/dispatches.test.ts`

- [ ] **Step 18.1: Add the race test**

```ts
it("concurrent POSTs with same fingerprint resolve to one record, one LLM call", async () => {
  const { user, repo } = await seedRepoUserAndCheckpoints({ count: 3 })
  const body = { repo: repo.fullName, since: "2026-04-09T00:00:00Z", until: "2026-04-16T23:59:59Z", branches: "all", generate: true, voice: "neutral" }

  const llmSpy = vi.spyOn(require("../lib/dispatch-synthesis"), "runSynthesis")
  const [r1, r2, r3] = await Promise.all([
    request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send(body),
    request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send(body),
    request.post("/api/v1/users/me/dispatches").set("Authorization", `Bearer ${user.token}`).send(body),
  ])
  const ids = new Set([r1.body.id, r2.body.id, r3.body.id])
  expect(ids.size).toBe(1)
  expect(llmSpy).toHaveBeenCalledTimes(1)
})
```

- [ ] **Step 18.2: Pass + commit**

```bash
pnpm --filter api test -t "concurrent POSTs"
git add api/src/routes/dispatches.test.ts
git commit -m "test(dispatches): concurrent-create race guaranteed single LLM call"
```

---

## Task 19: Self-review + spec-coverage verification

- [ ] **Step 19.1: Re-read the spec and tick off coverage**

Walk through `specs/2026-04-16-entire-dispatch-design.md`'s Persistence-and-idempotency section and Authorization contract. For each documented requirement, point to the test that covers it. Add any missing tests.

Run the full suite:

```bash
pnpm --filter api test
```

Expected: all green.

- [ ] **Step 19.2: Final commit with any fix-ups**

```bash
git add -A
git commit -m "test(dispatches): final spec-coverage sweep"
```

---

## Execution Handoff

Plan complete and saved to `plans/2026-04-16-entire-dispatch-backend.md`.

Backend dependency graph:
- Tasks 1–2 → Task 3 (fingerprint), Task 4 (window): parallelizable
- Tasks 3, 4, 6, 7, 8, 9 → Task 10 (POST endpoint): all inputs to the route
- Task 10 → Tasks 11–13
- Task 5 (batch-analyses status update) is independent and can run in parallel with Task 1

Total tasks: 19. Estimated ~1–2 days per engineer given the scaffold.
