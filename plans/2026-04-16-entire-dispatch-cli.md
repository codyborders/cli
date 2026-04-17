# Entire Dispatch — CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the `entire dispatch` command: server-mode by default (calls backend `POST /api/v1/users/me/dispatches`), `--local` mode for user-LLM synthesis, an auto-launched `huh` wizard on bare TTY invocation, voice presets (neutral + marvin), and three output formats (text/markdown/json).

**Architecture:** New Go package `cmd/entire/cli/dispatch/`. A `newDispatchCmd()` cobra command routes by `--local` to either `server.go` (POST + render) or `local.go` (enumerate checkpoints locally → batch-fetch analyses → apply fallback chain → optional local LLM via existing `summarize.ClaudeGenerator`). A wizard fires when the command is invoked with no flags on a TTY (matches `entire search` / `entire sessions stop` convention).

**Tech Stack:** Go 1.26.x, cobra, huh (charmbracelet), go-git v6, existing `summarize` and `checkpoint` packages. Tests use the stdlib `testing` package with `t.Parallel()` and `testutil` helpers per CLAUDE.md.

**Companion spec:** [`specs/2026-04-16-entire-dispatch-design.md`](../specs/2026-04-16-entire-dispatch-design.md).

**Worktree:** This plan lives in the CLI repo (current worktree `/Users/alisha/Projects/wt/cli/entire-dispatch`). **Depends on the backend plan** for `--generate` in server mode; can ship with local mode only if backend isn't ready.

---

## File Structure

```
cmd/entire/cli/dispatch/
  dispatch.go                   # orchestration — mode selection, flag wiring
  server.go                     # server-mode client (POST /dispatches)
  server_test.go
  local.go                      # local-mode: enumerate + batch analyses + fallback + render
  local_test.go
  cloud.go                      # shared HTTP client (auth, retries, org enumeration)
  cloud_test.go
  fallback.go                   # 3-step fallback chain (cloud → local summary → commit msg)
  fallback_test.go
  render.go                     # text / markdown / json renderers (format-agnostic)
  render_test.go
  generate.go                   # local --generate path via summarize.ClaudeGenerator
  generate_test.go
  types.go                      # Dispatch, Repo, Section, Bullet — shared
  voices/
    neutral.md                  # shipped preset
    marvin.md                   # shipped preset
  voices.go                     # go:embed + resolveVoice()
  voices_test.go
  wizard.go                     # huh-based 7-step wizard
  wizard_test.go
  flags.go                      # flag validation + parse (--since, --branches list, --voice resolution)
  flags_test.go
  dispatch_test.go              # top-level mode-selection tests

cmd/entire/cli/dispatch.go      # top-level cobra command entry (newDispatchCmd wired in root.go)
cmd/entire/cli/root.go          # modify: register dispatch command
cmd/entire/cli/integration_test/dispatch_server_test.go     # integration: mocked server
cmd/entire/cli/integration_test/dispatch_local_test.go      # integration: mocked cloud analyses
e2e/vogon/main.go               # modify if needed to handle dispatch-relevant prompts
e2e/tests/dispatch_test.go      # vogon-backed E2E canary
```

---

## Task 1: Scaffold package + types

**Files:**
- Create: `cmd/entire/cli/dispatch/types.go`
- Create: `cmd/entire/cli/dispatch/dispatch.go`

- [ ] **Step 1.1: Write the types file**

```go
// cmd/entire/cli/dispatch/types.go
package dispatch

import "time"

// Dispatch is the rendered, in-memory representation returned from either server or local mode.
type Dispatch struct {
    ID            string // empty in --local mode and in server-mode generate:false / dry-run
    FingerprintHash string // empty in non-persisted modes
    WebURL        string
    Window        Window
    CoveredRepos  []string
    Repos         []RepoGroup
    Totals        Totals
    Warnings      Warnings
    GeneratedText string

    // mode flags for clients
    DryRun          bool
    RequestedGenerate bool
    Generated       bool // true when generated_text is present
    Deduped         bool
}

type Window struct {
    NormalizedSince time.Time
    NormalizedUntil time.Time
    FirstCheckpointAt time.Time
    LastCheckpointAt  time.Time
}

type RepoGroup struct {
    FullName string
    Sections []Section
}

type Section struct {
    Label   string
    Bullets []Bullet
}

type Bullet struct {
    CheckpointID string
    Text         string
    Source       string // "cloud_analysis" | "local_summary" | "commit_message"
    Branch       string
    CreatedAt    time.Time
    Labels       []string
}

type Totals struct {
    Checkpoints         int
    UsedCheckpointCount int
    Branches            int
    FilesTouched        int
}

type Warnings struct {
    AccessDeniedCount   int
    PendingCount        int
    FailedCount         int
    UnknownCount        int
    UncategorizedCount  int
}
```

- [ ] **Step 1.2: Scaffold `dispatch.go` orchestration + commit**

```go
// cmd/entire/cli/dispatch/dispatch.go
package dispatch

import (
    "context"
    "errors"
)

type Mode int

const (
    ModeServer Mode = iota
    ModeLocal
)

// Options holds all parsed CLI inputs.
type Options struct {
    Mode          Mode
    RepoPaths     []string   // --repos
    Org           string     // --org
    Since         string     // raw --since (resolved to time)
    Until         string     // raw --until (or "now")
    Branches      []string   // resolved branch names; "all" handled by caller
    AllBranches   bool
    Generate      bool
    Voice         string     // raw --voice value; resolution happens in voices.go
    Format        string     // "text" | "markdown" | "json"
    DryRun        bool
    Wait          bool
}

// Run is the entry point after flags are parsed. It dispatches to server or local implementations.
func Run(ctx context.Context, opts Options) (*Dispatch, error) {
    if opts.Mode == ModeServer && len(opts.RepoPaths) > 0 {
        return nil, errors.New("--repos requires --local")
    }
    if opts.Mode == ModeServer {
        return runServer(ctx, opts)
    }
    return runLocal(ctx, opts)
}
```

Commit:

```bash
git add cmd/entire/cli/dispatch/types.go cmd/entire/cli/dispatch/dispatch.go
git commit -m "dispatch: scaffold package with types and mode router"
```

---

## Task 2: Voices — embed presets + resolve

**Files:**
- Create: `cmd/entire/cli/dispatch/voices/neutral.md`
- Create: `cmd/entire/cli/dispatch/voices/marvin.md`
- Create: `cmd/entire/cli/dispatch/voices.go`
- Create: `cmd/entire/cli/dispatch/voices_test.go`

- [ ] **Step 2.1: Write preset markdown (same content as backend Task 6)**

Same body text as the backend plan's `neutral.md` and `marvin.md`.

- [ ] **Step 2.2: Failing test**

```go
// cmd/entire/cli/dispatch/voices_test.go
package dispatch

import (
    "testing"
)

func TestResolveVoice_PresetMatch(t *testing.T) {
    t.Parallel()
    got := ResolveVoice("marvin")
    if !got.IsPreset { t.Fatal("expected preset") }
    if got.Name != "marvin" { t.Fatalf("expected name=marvin, got %q", got.Name) }
    if got.Text == "" { t.Fatal("expected non-empty preset text") }
}

func TestResolveVoice_CaseInsensitive(t *testing.T) {
    t.Parallel()
    got := ResolveVoice("MARVIN")
    if !got.IsPreset { t.Fatal("expected preset for MARVIN") }
}

func TestResolveVoice_FilePathFallback(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    path := dir + "/voice.md"
    if err := os.WriteFile(path, []byte("my voice"), 0o600); err != nil { t.Fatal(err) }
    got := ResolveVoice(path)
    if got.IsPreset { t.Fatal("should not be preset") }
    if got.Text != "my voice" { t.Fatalf("wanted file content, got %q", got.Text) }
}

func TestResolveVoice_LiteralStringFallback(t *testing.T) {
    t.Parallel()
    got := ResolveVoice("sardonic AI named Gary")
    if got.IsPreset { t.Fatal("expected literal") }
    if got.Text != "sardonic AI named Gary" { t.Fatalf("expected passthrough, got %q", got.Text) }
}

func TestResolveVoice_EmptyDefaultsToNeutral(t *testing.T) {
    t.Parallel()
    got := ResolveVoice("")
    if !got.IsPreset || got.Name != "neutral" { t.Fatalf("expected neutral default, got %+v", got) }
}
```

- [ ] **Step 2.3: Fail**

```bash
go test ./cmd/entire/cli/dispatch/ -run TestResolveVoice -v
```

- [ ] **Step 2.4: Implement**

```go
// cmd/entire/cli/dispatch/voices.go
package dispatch

import (
    _ "embed"
    "os"
    "strings"
)

//go:embed voices/neutral.md
var voiceNeutral string

//go:embed voices/marvin.md
var voiceMarvin string

type Voice struct {
    Name     string
    Text     string
    IsPreset bool
}

var presets = map[string]string{
    "neutral": voiceNeutral,
    "marvin":  voiceMarvin,
}

// ResolveVoice applies the three-step resolution: preset name → file path → literal.
func ResolveVoice(value string) Voice {
    if value == "" {
        return Voice{Name: "neutral", Text: voiceNeutral, IsPreset: true}
    }
    if text, ok := presets[strings.ToLower(value)]; ok {
        return Voice{Name: strings.ToLower(value), Text: text, IsPreset: true}
    }
    // Try file path (must contain a path separator or exist on disk).
    if info, err := os.Stat(value); err == nil && !info.IsDir() {
        b, err := os.ReadFile(value)
        if err == nil {
            return Voice{Name: "", Text: string(b), IsPreset: false}
        }
    }
    return Voice{Name: "", Text: value, IsPreset: false}
}

// ListPresetNames returns the shipped preset names (for wizard display).
func ListPresetNames() []string { return []string{"neutral", "marvin"} }
```

- [ ] **Step 2.5: Pass + commit**

```bash
go test ./cmd/entire/cli/dispatch/ -run TestResolveVoice -v
git add cmd/entire/cli/dispatch/voices cmd/entire/cli/dispatch/voices.go cmd/entire/cli/dispatch/voices_test.go
git commit -m "dispatch: ship neutral + marvin voice presets"
```

---

## Task 3: Flag parsing — `--since`, `--branches`, etc.

**Files:**
- Create: `cmd/entire/cli/dispatch/flags.go`
- Create: `cmd/entire/cli/dispatch/flags_test.go`

- [ ] **Step 3.1: Failing tests**

```go
// cmd/entire/cli/dispatch/flags_test.go
package dispatch

import (
    "testing"
    "time"
)

func TestParseSince_GoDuration(t *testing.T) {
    t.Parallel()
    now := time.Date(2026, 4, 16, 14, 32, 0, 0, time.UTC)
    got, err := ParseSinceAtNow("7d", now)
    if err != nil { t.Fatal(err) }
    want := now.Add(-7 * 24 * time.Hour)
    if !got.Equal(want) { t.Fatalf("want %v got %v", want, got) }
}

func TestParseSince_GitStyle(t *testing.T) {
    t.Parallel()
    now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
    got, err := ParseSinceAtNow("2 days ago", now)
    if err != nil { t.Fatal(err) }
    if !got.Equal(now.Add(-48 * time.Hour)) { t.Fatalf("got %v", got) }
}

func TestParseSince_ISO(t *testing.T) {
    t.Parallel()
    got, err := ParseSinceAtNow("2026-04-09", time.Now())
    if err != nil { t.Fatal(err) }
    if got.Year() != 2026 || got.Month() != 4 || got.Day() != 9 { t.Fatalf("got %v", got) }
}

func TestParseBranches_List(t *testing.T) {
    t.Parallel()
    b, allBranches, err := ParseBranches("main,release", /*currentBranch*/ "main")
    if err != nil { t.Fatal(err) }
    if allBranches { t.Fatal("all should be false") }
    if len(b) != 2 || b[0] != "main" || b[1] != "release" { t.Fatalf("got %v", b) }
}

func TestParseBranches_All(t *testing.T) {
    t.Parallel()
    b, allBranches, _ := ParseBranches("all", "main")
    if !allBranches { t.Fatal("all should be true") }
    if b != nil { t.Fatalf("expected nil slice, got %v", b) }
}

func TestParseBranches_Default(t *testing.T) {
    t.Parallel()
    b, allBranches, _ := ParseBranches("", "feat/x")
    if allBranches { t.Fatal("all should be false") }
    if len(b) != 1 || b[0] != "feat/x" { t.Fatalf("got %v", b) }
}
```

- [ ] **Step 3.2: Fail**

```bash
go test ./cmd/entire/cli/dispatch/ -run TestParse -v
```

- [ ] **Step 3.3: Implement**

```go
// cmd/entire/cli/dispatch/flags.go
package dispatch

import (
    "fmt"
    "regexp"
    "strings"
    "time"
)

var (
    goDurationPat = regexp.MustCompile(`^(\d+)([smhdw])$`)
    relativePat   = regexp.MustCompile(`^(\d+)\s*(second|minute|hour|day|week|month)s?\s*ago$`)
)

// ParseSinceAtNow parses any of: Go duration ("7d"), git-style ("2 days ago"), or ISO date ("2026-04-09" or full RFC3339).
func ParseSinceAtNow(value string, now time.Time) (time.Time, error) {
    value = strings.TrimSpace(value)
    if m := goDurationPat.FindStringSubmatch(value); m != nil {
        n, _ := time.ParseDuration(m[1] + map[string]string{"s": "s", "m": "m", "h": "h", "d": "h", "w": "h"}[m[2]])
        switch m[2] {
        case "d": return now.Add(-time.Duration(mustInt(m[1])) * 24 * time.Hour), nil
        case "w": return now.Add(-time.Duration(mustInt(m[1])) * 7 * 24 * time.Hour), nil
        default: return now.Add(-n), nil
        }
    }
    if m := relativePat.FindStringSubmatch(strings.ToLower(value)); m != nil {
        n := time.Duration(mustInt(m[1]))
        unit := map[string]time.Duration{
            "second": time.Second, "minute": time.Minute, "hour": time.Hour,
            "day": 24 * time.Hour, "week": 7 * 24 * time.Hour, "month": 30 * 24 * time.Hour,
        }[m[2]]
        return now.Add(-n * unit), nil
    }
    // Try RFC3339 then date-only.
    if t, err := time.Parse(time.RFC3339, value); err == nil { return t, nil }
    if t, err := time.Parse("2006-01-02", value); err == nil { return t, nil }
    return time.Time{}, fmt.Errorf("unparseable --since: %q", value)
}

func ParseBranches(value string, currentBranch string) (branches []string, all bool, err error) {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "":
        return []string{currentBranch}, false, nil
    case "all", "*":
        return nil, true, nil
    }
    parts := strings.Split(value, ",")
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p != "" { out = append(out, p) }
    }
    return out, false, nil
}

func mustInt(s string) int64 {
    var n int64
    fmt.Sscanf(s, "%d", &n)
    return n
}
```

- [ ] **Step 3.4: Pass + commit**

```bash
go test ./cmd/entire/cli/dispatch/ -run TestParse -v
git add cmd/entire/cli/dispatch/flags.go cmd/entire/cli/dispatch/flags_test.go
git commit -m "dispatch: flag parsing for --since and --branches"
```

---

## Task 4: Cloud client scaffolding

**Files:**
- Create: `cmd/entire/cli/dispatch/cloud.go`
- Create: `cmd/entire/cli/dispatch/cloud_test.go`

- [ ] **Step 4.1: Failing test — happy path post, 401, 5xx**

```go
// cmd/entire/cli/dispatch/cloud_test.go
package dispatch

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestCloudClient_PostDispatches_Happy(t *testing.T) {
    t.Parallel()
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/api/v1/users/me/dispatches" { http.NotFound(w, r); return }
        if r.Header.Get("Authorization") == "" { w.WriteHeader(401); return }
        var body map[string]any
        json.NewDecoder(r.Body).Decode(&body)
        if body["repo"] != "entireio/cli" { t.Fatalf("bad body: %v", body) }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(201)
        w.Write([]byte(`{"id":"dsp_1","status":"complete","fingerprint_hash":"fp","deduped":false,"web_url":"https://e/1","window":{"normalized_since":"2026-04-09T00:00:00Z","normalized_until":"2026-04-16T00:00:00Z"},"covered_repos":["entireio/cli"],"repos":[],"totals":{"checkpoints":0,"used_checkpoint_count":0,"branches":0,"files_touched":0},"warnings":{"access_denied_count":0,"pending_count":0,"failed_count":0,"unknown_count":0,"uncategorized_count":0},"generated_text":"hi"}`))
    }))
    defer srv.Close()
    c := NewCloudClient(CloudConfig{BaseURL: srv.URL, Token: "t"})
    got, err := c.CreateDispatch(ctx, CreateDispatchRequest{Repo: "entireio/cli", Since: "2026-04-09T00:00:00Z", Until: "2026-04-16T00:00:00Z", Branches: []string{"all"}, Generate: true})
    if err != nil { t.Fatal(err) }
    if got.ID != "dsp_1" { t.Fatalf("bad id: %v", got.ID) }
}

func TestCloudClient_Unauthorized(t *testing.T) {
    t.Parallel()
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) }))
    defer srv.Close()
    c := NewCloudClient(CloudConfig{BaseURL: srv.URL, Token: ""})
    _, err := c.CreateDispatch(ctx, CreateDispatchRequest{Repo: "x/y"})
    if err == nil || !strings.Contains(err.Error(), "login") { t.Fatalf("expected auth error, got %v", err) }
}
```

- [ ] **Step 4.2: Fail + implement + pass + commit**

Implement `CloudClient`, `CloudConfig`, `CreateDispatch`, and stubs for `FetchBatchAnalyses`, `EnumerateOrgCheckpoints` — follow the existing HTTP client pattern in `cmd/entire/cli/search/search.go` and reuse `auth.LookupCurrentToken` for the token.

```go
// cmd/entire/cli/dispatch/cloud.go
package dispatch

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "time"
)

type CloudConfig struct {
    BaseURL string
    Token   string
    HTTP    *http.Client
    Timeout time.Duration
}

type CloudClient struct { cfg CloudConfig }

func NewCloudClient(c CloudConfig) *CloudClient {
    if c.HTTP == nil { c.HTTP = &http.Client{Timeout: c.Timeout} }
    if c.Timeout == 0 { c.HTTP.Timeout = 30 * time.Second }
    return &CloudClient{cfg: c}
}

type CreateDispatchRequest struct {
    Repo     string   `json:"repo,omitempty"`
    Org      string   `json:"org,omitempty"`
    Since    string   `json:"since"`
    Until    string   `json:"until"`
    Branches any      `json:"branches"`  // []string or "all"
    Generate bool     `json:"generate"`
    Voice    string   `json:"voice,omitempty"`
    DryRun   bool     `json:"dry_run,omitempty"`
}

type CreateDispatchResponse struct {
    ID               string   `json:"id,omitempty"`
    Status           string   `json:"status,omitempty"`
    FingerprintHash  string   `json:"fingerprint_hash,omitempty"`
    Deduped          bool     `json:"deduped,omitempty"`
    WebURL           string   `json:"web_url,omitempty"`
    CoveredRepos     []string `json:"covered_repos,omitempty"`
    Window           ApiWindow `json:"window"`
    Repos            []ApiRepo `json:"repos"`
    Totals           ApiTotals `json:"totals"`
    Warnings         ApiWarnings `json:"warnings"`
    GeneratedText    string   `json:"generated_text,omitempty"`
    DryRun           bool     `json:"dry_run,omitempty"`
    RequestedGenerate *bool   `json:"requested_generate,omitempty"`
    Generate         *bool    `json:"generate,omitempty"`
}

type ApiWindow struct {
    NormalizedSince         string `json:"normalized_since"`
    NormalizedUntil         string `json:"normalized_until"`
    FirstCheckpointCreatedAt string `json:"first_checkpoint_created_at,omitempty"`
    LastCheckpointCreatedAt  string `json:"last_checkpoint_created_at,omitempty"`
}
type ApiRepo struct {
    FullName string       `json:"full_name"`
    Sections []ApiSection `json:"sections"`
}
type ApiSection struct {
    Label   string      `json:"label"`
    Bullets []ApiBullet `json:"bullets"`
}
type ApiBullet struct {
    CheckpointID string   `json:"checkpoint_id"`
    Text         string   `json:"text"`
    Source       string   `json:"source"`
    Branch       string   `json:"branch"`
    CreatedAt    string   `json:"created_at"`
    Labels       []string `json:"labels"`
}
type ApiTotals struct {
    Checkpoints         int `json:"checkpoints"`
    UsedCheckpointCount int `json:"used_checkpoint_count"`
    Branches            int `json:"branches"`
    FilesTouched        int `json:"files_touched"`
}
type ApiWarnings struct {
    AccessDeniedCount  int `json:"access_denied_count"`
    PendingCount       int `json:"pending_count"`
    FailedCount        int `json:"failed_count"`
    UnknownCount       int `json:"unknown_count"`
    UncategorizedCount int `json:"uncategorized_count"`
}

func (c *CloudClient) CreateDispatch(ctx context.Context, req CreateDispatchRequest) (*CreateDispatchResponse, error) {
    body, _ := json.Marshal(req)
    httpReq, _ := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/api/v1/users/me/dispatches", bytes.NewReader(body))
    httpReq.Header.Set("Content-Type", "application/json")
    if c.cfg.Token != "" { httpReq.Header.Set("Authorization", "Bearer "+c.cfg.Token) }
    res, err := c.cfg.HTTP.Do(httpReq)
    if err != nil { return nil, err }
    defer res.Body.Close()
    if res.StatusCode == 401 || res.StatusCode == 403 {
        return nil, errors.New("login expired — run `entire login`")
    }
    if res.StatusCode == 404 {
        return nil, fmt.Errorf("scope not accessible or endpoint not deployed")
    }
    if res.StatusCode >= 500 {
        raw, _ := io.ReadAll(res.Body)
        return nil, fmt.Errorf("server error %d: %s — try --local", res.StatusCode, string(raw))
    }
    var out CreateDispatchResponse
    if err := json.NewDecoder(res.Body).Decode(&out); err != nil { return nil, fmt.Errorf("decoding response: %w", err) }
    return &out, nil
}

// FetchBatchAnalyses posts checkpoint ids to the existing batch-analyses endpoint and returns the per-id statuses.
func (c *CloudClient) FetchBatchAnalyses(ctx context.Context, repoFullName string, ids []string) (map[string]AnalysisStatus, error) {
    // Implement by POSTing to /api/v1/users/me/checkpoints/analyses/batch with {repoFullName, checkpointIds}
    // and decoding the response into map[string]AnalysisStatus. See backend Task 5.
    // ... (implementation shown in Task 5 below)
    return nil, errors.New("unimplemented")
}
```

Commit:

```bash
git add cmd/entire/cli/dispatch/cloud.go cmd/entire/cli/dispatch/cloud_test.go
git commit -m "dispatch: cloud client for POST /dispatches"
```

---

## Task 5: Cloud client — `FetchBatchAnalyses`

**Files:**
- Modify: `cmd/entire/cli/dispatch/cloud.go`
- Modify: `cmd/entire/cli/dispatch/cloud_test.go`

- [ ] **Step 5.1: Failing test + implement + pass + commit**

```go
type AnalysisStatus struct {
    Status  string   `json:"status"` // complete | pending | generating | failed | not_visible | unknown
    Summary string   `json:"summary,omitempty"`
    Labels  []string `json:"labels,omitempty"`
}

func (c *CloudClient) FetchBatchAnalyses(ctx context.Context, repoFullName string, ids []string) (map[string]AnalysisStatus, error) {
    if len(ids) == 0 { return map[string]AnalysisStatus{}, nil }
    out := map[string]AnalysisStatus{}
    for chunk := range chunkIDs(ids, 200) {
        body, _ := json.Marshal(map[string]any{"repoFullName": repoFullName, "checkpointIds": chunk})
        req, _ := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/api/v1/users/me/checkpoints/analyses/batch", bytes.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
        res, err := c.cfg.HTTP.Do(req)
        if err != nil { return nil, err }
        defer res.Body.Close()
        if res.StatusCode != 200 { return nil, fmt.Errorf("batch analyses: %d", res.StatusCode) }
        var payload struct { Analyses map[string]AnalysisStatus `json:"analyses"` }
        if err := json.NewDecoder(res.Body).Decode(&payload); err != nil { return nil, err }
        for k, v := range payload.Analyses { out[k] = v }
    }
    return out, nil
}
```

Commit: `dispatch: FetchBatchAnalyses with 200-ID pagination`.

---

## Task 6: Fallback chain

**Files:**
- Create: `cmd/entire/cli/dispatch/fallback.go`
- Create: `cmd/entire/cli/dispatch/fallback_test.go`

Same logic as the backend `applyFallbackChain` from Plan 1 Task 7, plus the local-summary source that only applies in `--local` mode:

- [ ] **Step 6.1: Test the six-way chain**

```go
// cmd/entire/cli/dispatch/fallback_test.go
package dispatch

import "testing"

type fakeAnalysis struct{ status, summary string; labels []string }
type candidate struct {
    ID, RepoFullName, Branch, CommitSubject, LocalSummaryTitle string
    HasLocalSummary bool
}

// tests: cloud complete → cloud_analysis bullet
// cloud pending/generating → skipped + pending_count++
// cloud unknown + local summary present → local_summary bullet + unknown_count++
// cloud unknown + no local summary + commit message → commit_message bullet + unknown_count++
// cloud failed + commit → commit_message bullet + failed_count++
// cloud not_visible → commit_message or local_summary + access_denied_count++
// no analysis, no commit, no local summary → omit + uncategorized_count++
```

- [ ] **Step 6.2: Implement + pass + commit**

Keep the implementation small — the logic is a straight-line switch on `status` + two fallback lookups.

Commit: `dispatch: per-checkpoint fallback chain`.

---

## Task 7: Local-mode enumeration

**Files:**
- Create: `cmd/entire/cli/dispatch/local.go`
- Create: `cmd/entire/cli/dispatch/local_test.go`

- [ ] **Step 7.1: Failing test — uses testutil + a seeded repo**

```go
// cmd/entire/cli/dispatch/local_test.go
package dispatch

import (
    "context"
    "testing"
    "time"

    "github.com/entireio/cli/cmd/entire/cli/testutil"
)

func TestLocalMode_EnumeratesCheckpoints(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    testutil.InitRepo(t, dir)
    testutil.WriteFile(t, dir, "a.txt", "x")
    testutil.GitAdd(t, dir, "a.txt")
    testutil.GitCommit(t, dir, "initial")
    // Seed an Entire checkpoint on entire/checkpoints/v1 via testutil helpers — follow the pattern in strategy tests.
    testutil.SeedCheckpoint(t, dir, testutil.CheckpointSpec{ID: "cp1", Branch: "main", CreatedAt: time.Now()})

    t.Chdir(dir)
    opts := Options{Mode: ModeLocal, Since: "7d", Branches: []string{"main"}, Generate: false, Format: "json"}
    got, err := Run(context.Background(), opts)
    if err != nil { t.Fatal(err) }
    if got.Totals.Checkpoints != 1 { t.Fatalf("expected 1 candidate, got %d", got.Totals.Checkpoints) }
}
```

- [ ] **Step 7.2: Fail + implement**

```go
// cmd/entire/cli/dispatch/local.go
package dispatch

import (
    "context"
    "fmt"
    "time"

    "github.com/entireio/cli/cmd/entire/cli/auth"
    "github.com/entireio/cli/cmd/entire/cli/paths"
    "github.com/entireio/cli/cmd/entire/cli/strategy"
)

func runLocal(ctx context.Context, opts Options) (*Dispatch, error) {
    repoRoot, err := paths.WorktreeRoot(ctx)
    if err != nil { return nil, fmt.Errorf("not in a git repo: %w", err) }

    now := time.Now().UTC()
    since, err := ParseSinceAtNow(opts.Since, now)
    if err != nil { return nil, err }
    normalizedSince, normalizedUntil := NormalizeWindow(since, now)

    // Enumerate local checkpoints in the window for the configured branches.
    candidates, err := strategy.ListCheckpoints(ctx, repoRoot, strategy.ListOptions{
        Since:    normalizedSince,
        Until:    normalizedUntil,
        Branches: opts.Branches, // nil means all; empty string handled upstream
    })
    if err != nil { return nil, fmt.Errorf("enumerating checkpoints: %w", err) }

    // Resolve the user's token (for the batch analyses call).
    token, err := auth.LookupCurrentToken()
    if err != nil { return nil, fmt.Errorf("reading credentials: %w", err) }
    if token == "" { return nil, fmt.Errorf("dispatch requires login — run `entire login`") }

    cloud := NewCloudClient(CloudConfig{BaseURL: cloudBaseURL(), Token: token})

    // Group candidates by repo for batch calls.
    byRepo := groupCandidatesByRepo(candidates)
    analyses := make(map[string]AnalysisStatus)
    for repoFullName, ids := range byRepo {
        m, err := cloud.FetchBatchAnalyses(ctx, repoFullName, ids)
        if err != nil { return nil, err }
        for k, v := range m { analyses[k] = v }
    }

    result := applyFallbackChain(candidates, analyses)

    d := &Dispatch{
        Repos:        groupBulletsByRepo(result.Used),
        Totals:       computeTotals(candidates, result.Used),
        Warnings:     result.Warnings,
        Window: Window{
            NormalizedSince: normalizedSince,
            NormalizedUntil: normalizedUntil,
            FirstCheckpointAt: firstAt(result.Used),
            LastCheckpointAt:  lastAt(result.Used),
        },
    }

    if opts.Generate {
        text, err := generateLocalPrompt(ctx, d.Repos, opts.Voice)
        if err != nil { return nil, err }
        d.GeneratedText = text
        d.Generated = true
    }

    return d, nil
}

func NormalizeWindow(since time.Time, until time.Time) (time.Time, time.Time) {
    floored := since.Truncate(time.Minute)
    ceiled := until.Truncate(time.Minute)
    if !until.Equal(ceiled) { ceiled = ceiled.Add(time.Minute) }
    return floored, ceiled
}
```

- [ ] **Step 7.3: Pass + commit**

```bash
go test ./cmd/entire/cli/dispatch/ -run TestLocalMode -v
git add cmd/entire/cli/dispatch/local.go cmd/entire/cli/dispatch/local_test.go
git commit -m "dispatch: local-mode enumeration + batch analyses"
```

---

## Task 8: Server-mode client wrapper

**Files:**
- Create: `cmd/entire/cli/dispatch/server.go`
- Create: `cmd/entire/cli/dispatch/server_test.go`

- [ ] **Step 8.1: Failing integration test with a mocked server**

```go
// cmd/entire/cli/dispatch/server_test.go
package dispatch

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/entireio/cli/cmd/entire/cli/testutil"
)

func TestServerMode_HappyPath(t *testing.T) {
    t.Parallel()
    // Stand up a mock server returning a complete dispatch.
    mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(201)
        json.NewEncoder(w).Encode(map[string]any{
            "id": "dsp_abc", "status": "complete", "fingerprint_hash": "fp",
            "deduped": false, "web_url": "https://entire.io/dispatches/dsp_abc",
            "window": map[string]any{"normalized_since": "2026-04-09T00:00:00Z", "normalized_until": "2026-04-16T00:00:00Z"},
            "covered_repos": []string{"entireio/cli"},
            "repos": []any{}, "totals": map[string]any{"checkpoints":0,"used_checkpoint_count":0,"branches":0,"files_touched":0},
            "warnings": map[string]any{"access_denied_count":0,"pending_count":0,"failed_count":0,"unknown_count":0,"uncategorized_count":0},
            "generated_text": "Hello",
        })
    }))
    defer mock.Close()

    dir := t.TempDir()
    testutil.InitRepo(t, dir)
    testutil.GitRemoteAdd(t, dir, "origin", "https://github.com/entireio/cli.git")
    t.Chdir(dir)

    t.Setenv("ENTIRE_API_BASE_URL", mock.URL)
    t.Setenv("ENTIRE_AUTH_TOKEN", "test-token")

    got, err := Run(context.Background(), Options{Mode: ModeServer, Since: "7d", Branches: []string{"main"}, Generate: true, Voice: "neutral", Format: "text"})
    if err != nil { t.Fatal(err) }
    if got.ID != "dsp_abc" { t.Fatalf("bad id: %v", got.ID) }
    if got.GeneratedText != "Hello" { t.Fatalf("bad text: %q", got.GeneratedText) }
}
```

- [ ] **Step 8.2: Implement + pass + commit**

```go
// cmd/entire/cli/dispatch/server.go
package dispatch

import (
    "context"
    "fmt"
    "time"

    "github.com/entireio/cli/cmd/entire/cli/auth"
    "github.com/entireio/cli/cmd/entire/cli/paths"
)

func runServer(ctx context.Context, opts Options) (*Dispatch, error) {
    token, err := auth.LookupCurrentToken()
    if err != nil { return nil, err }
    if token == "" { return nil, fmt.Errorf("dispatch requires login — run `entire login`") }

    now := time.Now().UTC()
    since, err := ParseSinceAtNow(opts.Since, now)
    if err != nil { return nil, err }

    var repoFullName string
    if opts.Org == "" {
        repoFullName, err = paths.ResolveCurrentRepoFullName(ctx)
        if err != nil { return nil, fmt.Errorf("resolving current repo: %w", err) }
    }

    var branches any = opts.Branches
    if opts.AllBranches { branches = "all" }

    cloud := NewCloudClient(CloudConfig{BaseURL: cloudBaseURL(), Token: token})
    req := CreateDispatchRequest{
        Repo:     repoFullName,
        Org:      opts.Org,
        Since:    since.Format(time.RFC3339),
        Until:    now.Format(time.RFC3339),
        Branches: branches,
        Generate: opts.Generate,
        Voice:    opts.Voice,
        DryRun:   opts.DryRun,
    }
    res, err := cloud.CreateDispatch(ctx, req)
    if err != nil { return nil, err }
    return apiToDispatch(res), nil
}
```

Commit: `dispatch: server-mode client wrapping POST /dispatches`.

---

## Task 9: Renderers — text / markdown / json

**Files:**
- Create: `cmd/entire/cli/dispatch/render.go`
- Create: `cmd/entire/cli/dispatch/render_test.go`
- Create golden files under `cmd/entire/cli/dispatch/testdata/`

- [ ] **Step 9.1: Golden-file tests**

```go
// cmd/entire/cli/dispatch/render_test.go
package dispatch

import (
    "os"
    "strings"
    "testing"
)

func TestRender_Text_Golden(t *testing.T) {
    t.Parallel()
    d := testDispatchFixture()
    got := RenderText(d)
    want, _ := os.ReadFile("testdata/text.golden")
    if strings.TrimSpace(got) != strings.TrimSpace(string(want)) {
        t.Fatalf("mismatch\nwant:\n%s\ngot:\n%s", want, got)
    }
}
// ... mirror for markdown and json
```

- [ ] **Step 9.2: Implement renderers + commit**

Implement `RenderText`, `RenderMarkdown`, `RenderJSON`. Small, pure functions. Commit with the golden files.

---

## Task 10: Local `--generate` via `summarize.ClaudeGenerator`

**Files:**
- Create: `cmd/entire/cli/dispatch/generate.go`
- Create: `cmd/entire/cli/dispatch/generate_test.go`

- [ ] **Step 10.1: Build a system prompt (voice + bullets) and delegate to `summarize.ClaudeGenerator`**

Reuse `cmd/entire/cli/summarize`'s Anthropic client. Wrap with a dispatch-specific system prompt composed from the voice + "group by theme, preserve facts" instructions.

Commit: `dispatch: local --generate via summarize.ClaudeGenerator`.

---

## Task 11: Cobra command entry

**Files:**
- Create: `cmd/entire/cli/dispatch.go`
- Modify: `cmd/entire/cli/root.go` — register `newDispatchCmd()`

- [ ] **Step 11.1: Implement newDispatchCmd**

```go
// cmd/entire/cli/dispatch.go
package cli

import (
    "fmt"
    "os"

    "github.com/entireio/cli/cmd/entire/cli/dispatch"
    "github.com/spf13/cobra"
    "golang.org/x/term"
)

func newDispatchCmd() *cobra.Command {
    var (
        flagLocal       bool
        flagSince       string
        flagBranches    string
        flagRepos       []string
        flagOrg         string
        flagGenerate    bool
        flagVoice       string
        flagFormat      string
        flagDryRun      bool
        flagWait        bool
    )

    cmd := &cobra.Command{
        Use:   "dispatch",
        Short: "Generate a dispatch summarizing recent agent work",
        RunE: func(cmd *cobra.Command, _ []string) error {
            anyFlagSet := cmd.Flags().NFlag() > 0
            isTTY := term.IsTerminal(int(os.Stdin.Fd()))
            if !anyFlagSet && isTTY {
                // Auto-wizard
                opts, err := dispatch.RunWizard(cmd.Context())
                if err != nil { return err }
                return runAndRender(cmd, opts)
            }
            opts, err := parseDispatchFlags(flagLocal, flagSince, flagBranches, flagRepos, flagOrg, flagGenerate, flagVoice, flagFormat, flagDryRun, flagWait)
            if err != nil { return err }
            return runAndRender(cmd, opts)
        },
    }
    cmd.Flags().BoolVar(&flagLocal, "local", false, "use local LLM tokens (user-paid) instead of server synthesis")
    cmd.Flags().StringVar(&flagSince, "since", "7d", "time window (go duration, git-style, or ISO date)")
    cmd.Flags().StringVar(&flagBranches, "branches", "", "comma-separated branch names, or 'all'")
    cmd.Flags().StringSliceVar(&flagRepos, "repos", nil, "local repo paths (requires --local)")
    cmd.Flags().StringVar(&flagOrg, "org", "", "enumerate dispatches across an org")
    cmd.Flags().BoolVar(&flagGenerate, "generate", false, "synthesize LLM prose")
    cmd.Flags().StringVar(&flagVoice, "voice", "", "voice preset name, file path, or literal description")
    cmd.Flags().StringVar(&flagFormat, "format", "text", "output format: text | markdown | json")
    cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "preview without persisting")
    cmd.Flags().BoolVar(&flagWait, "wait", false, "block until pending analyses complete (5 min cap)")
    return cmd
}
```

```go
// cmd/entire/cli/root.go (modify)
rootCmd.AddCommand(newDispatchCmd())
```

- [ ] **Step 11.2: Wire up + commit**

Commit: `dispatch: register top-level cobra command`.

---

## Task 12: Wizard

**Files:**
- Create: `cmd/entire/cli/dispatch/wizard.go`
- Create: `cmd/entire/cli/dispatch/wizard_test.go`

- [ ] **Step 12.1: Write `RunWizard` with 7 steps using `huh`**

Follow the established pattern from `cmd/entire/cli/sessions.go` (`NewAccessibleForm(...)`). Each step sets one field on `Options`. The final "Confirm & Run" step prints the equivalent CLI command and runs it.

- [ ] **Step 12.2: Golden-file tests for accessible-mode output**

With `ACCESSIBLE=1` the wizard should degrade to plain-text prompts. Use scripted stdin to drive the happy path.

- [ ] **Step 12.3: Commit**

```bash
git add cmd/entire/cli/dispatch/wizard.go cmd/entire/cli/dispatch/wizard_test.go
git commit -m "dispatch: huh-based wizard for bare TTY invocation"
```

---

## Task 13: Integration tests — `integration_test/dispatch_*.go`

**Files:**
- Create: `cmd/entire/cli/integration_test/dispatch_server_test.go`
- Create: `cmd/entire/cli/integration_test/dispatch_local_test.go`

- [ ] **Step 13.1: Server-mode integration**

Spin up an `httptest.Server` mock, set `ENTIRE_API_BASE_URL`, run `entire dispatch --since 7d --generate --voice neutral` in a seeded repo, assert the rendered output contains the mocked `generated_text`.

- [ ] **Step 13.2: Local-mode integration**

Seed checkpoints in a test repo. Mock the batch-analyses endpoint. Run `entire dispatch --local --since 7d`. Assert bullets are present and `warnings.unknown_count` surfaces correctly when an ID isn't known to the mock.

- [ ] **Step 13.3: Commit**

```bash
git add cmd/entire/cli/integration_test/dispatch_server_test.go cmd/entire/cli/integration_test/dispatch_local_test.go
git commit -m "dispatch: integration tests for server and local modes"
```

---

## Task 14: Vogon canary

**Files:**
- Modify (maybe): `e2e/vogon/main.go` — ensure it handles dispatch-relevant prompts (if needed; likely no changes needed since vogon just creates checkpoints)
- Create: `e2e/tests/dispatch_test.go`

- [ ] **Step 14.1: Write canary test**

```go
// e2e/tests/dispatch_test.go
//go:build e2e
// ...
func TestE2E_Dispatch_VogonCanary(t *testing.T) {
    env := NewE2ERepoEnv(t)
    env.RunVogonPrompts(t, "add file a.go", "add file b.go", "add file c.go")
    out := env.Run(t, "entire", "dispatch", "--local", "--since", "1d", "--format", "json")
    assertContains(t, out, `"checkpoints":3`)
}
```

- [ ] **Step 14.2: Run + fix vogon if its prompt parser needs updates + commit**

```bash
mise run test:e2e:canary TestE2E_Dispatch_VogonCanary
git add e2e/tests/dispatch_test.go
git commit -m "test(dispatch): e2e canary via vogon"
```

---

## Task 15: Documentation + help text

**Files:**
- Modify: `cmd/entire/cli/dispatch.go` — `Long:` with examples
- Modify: `CLAUDE.md` — add a short note about dispatch under the CLI directory listing

- [ ] **Step 15.1: Write help text + commit**

---

## Task 16: Self-review + CI check

- [ ] **Step 16.1: Run CI locally**

```bash
mise run check    # fmt + lint + test:ci
```

Expected: all green.

- [ ] **Step 16.2: Final fix-up commit**

```bash
git add -A
git commit -m "dispatch: final fit-and-finish for v1"
```

---

## Execution Handoff

Plan complete and saved to `plans/2026-04-16-entire-dispatch-cli.md`.

Dependency graph within this plan:
- Tasks 1–6 (types, voices, flags, cloud client, fallback) in parallel
- Task 7 (local mode) depends on 1, 3, 4, 5, 6
- Task 8 (server mode) depends on 1, 4
- Task 9 (renderers) depends on 1
- Task 10 (local --generate) depends on 1, 9
- Task 11 (cobra command) depends on 1–10
- Task 12 (wizard) depends on 11
- Tasks 13–14 (tests) depend on everything

Total tasks: 16. Estimated ~1 week for one engineer; parallelizable across 2–3.
