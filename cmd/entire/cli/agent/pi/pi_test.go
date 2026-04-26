package pi

// These tests cover Pi integration boundaries: repository extension installation,
// lifecycle hook parsing, and Pi JSONL transcript metadata extraction.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

const sampleSessionID = "019dc5f7-ddd8-755f-b3eb-85f206f7b6dc"

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // Test helper reads paths created by the test.
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}

func TestPiInstallHooksWritesProjectExtension(t *testing.T) {
	repoDir := t.TempDir()
	t.Chdir(repoDir)

	agent := &PiAgent{}
	installed, err := agent.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}
	if installed != 1 {
		t.Fatalf("InstallHooks() = %d, want 1", installed)
	}

	extensionPath := filepath.Join(repoDir, ".pi", "extensions", "entire", "index.ts")
	data := readTestFile(t, extensionPath)
	if !strings.Contains(data, extensionMarker) {
		t.Fatalf("extension missing marker at %s", extensionPath)
	}
	if !strings.Contains(data, `callEntireHook(ctx, "user-prompt-submit"`) {
		t.Fatalf("extension missing user-prompt-submit listener: %s", data)
	}
	if !strings.Contains(data, "hooks pi ${hookName}") {
		t.Fatalf("extension missing Pi hook command: %s", data)
	}
	if !agent.AreHooksInstalled(context.Background()) {
		t.Fatal("AreHooksInstalled() = false, want true")
	}

	installed, err = agent.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("second InstallHooks() error = %v", err)
	}
	if installed != 0 {
		t.Fatalf("second InstallHooks() = %d, want 0", installed)
	}

	if err := agent.UninstallHooks(context.Background()); err != nil {
		t.Fatalf("UninstallHooks() error = %v", err)
	}
	if _, err := os.Stat(extensionPath); !os.IsNotExist(err) {
		t.Fatalf("expected extension file removed, stat error = %v", err)
	}
}

func TestPiInstallHooksLocalDevUsesGoRunCommand(t *testing.T) {
	repoDir := t.TempDir()
	t.Chdir(repoDir)

	agent := &PiAgent{}
	if _, err := agent.InstallHooks(context.Background(), true, true); err != nil {
		t.Fatalf("InstallHooks(localDev) error = %v", err)
	}

	data := readTestFile(t, filepath.Join(repoDir, ".pi", "extensions", "entire", "index.ts"))
	if !strings.Contains(data, `go run \"$(git rev-parse --show-toplevel)\"/cmd/entire/main.go`) {
		t.Fatalf("local dev extension missing go run command: %s", data)
	}
}

func TestPiParseHookEvent(t *testing.T) {
	t.Parallel()

	input := `{"hook_type":"user-prompt-submit","session_id":"` + sampleSessionID + `","session_ref":"/tmp/pi-session.jsonl","timestamp":"2026-04-25T12:30:00.123Z","cwd":"/repo","prompt":"change the header","model":"anthropic/claude","raw_data":{"leaf_id":"abc123","reason":"test"}}`
	event, err := (&PiAgent{}).ParseHookEvent(context.Background(), HookNameUserPromptSubmit, strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseHookEvent() error = %v", err)
	}
	if event == nil {
		t.Fatal("ParseHookEvent() returned nil event")
	}
	if event.Type != agent.TurnStart {
		t.Fatalf("event.Type = %v, want %v", event.Type, agent.TurnStart)
	}
	if event.SessionID != sampleSessionID {
		t.Fatalf("event.SessionID = %q, want %q", event.SessionID, sampleSessionID)
	}
	if event.SessionRef != "/tmp/pi-session.jsonl" {
		t.Fatalf("event.SessionRef = %q", event.SessionRef)
	}
	if event.Prompt != "change the header" {
		t.Fatalf("event.Prompt = %q", event.Prompt)
	}
	if event.Model != "anthropic/claude" {
		t.Fatalf("event.Model = %q", event.Model)
	}
	if event.Metadata["leaf_id"] != "abc123" {
		t.Fatalf("leaf metadata = %q", event.Metadata["leaf_id"])
	}
	if event.Timestamp.Format(time.RFC3339Nano) != "2026-04-25T12:30:00.123Z" {
		t.Fatalf("event.Timestamp = %s", event.Timestamp.Format(time.RFC3339Nano))
	}
}

func TestPiTranscriptExtraction(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	transcriptPath := filepath.Join(repoDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"type":"session","id":"` + sampleSessionID + `","timestamp":"2026-04-25T12:00:00.168Z","cwd":"/repo"}`,
		`{"type":"message","id":"u1","timestamp":"2026-04-25T12:00:01Z","message":{"role":"user","content":[{"type":"text","text":"write a page"}]}}`,
		`{"type":"message","id":"a1","timestamp":"2026-04-25T12:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","name":"write","arguments":{"path":"index.html"}},{"type":"toolCall","name":"edit","arguments":{"path":"style.css"}}],"usage":{"input":10,"output":20,"cacheRead":30,"cacheWrite":40}}}`,
		`{"type":"message","id":"a2","timestamp":"2026-04-25T12:00:03Z","message":{"role":"assistant","content":[{"type":"toolCall","name":"read","arguments":{"path":"ignored.txt"}},{"type":"toolCall","name":"write","arguments":{"path":"index.html"}}],"usage":{"inputTokens":1,"outputTokens":2}}}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	agent := &PiAgent{}
	position, err := agent.GetTranscriptPosition(transcriptPath)
	if err != nil {
		t.Fatalf("GetTranscriptPosition() error = %v", err)
	}
	if position != 4 {
		t.Fatalf("position = %d, want 4", position)
	}

	files, currentPosition, err := agent.ExtractModifiedFilesFromOffset(transcriptPath, 1)
	if err != nil {
		t.Fatalf("ExtractModifiedFilesFromOffset() error = %v", err)
	}
	if currentPosition != 4 {
		t.Fatalf("currentPosition = %d, want 4", currentPosition)
	}
	if strings.Join(files, ",") != "index.html,style.css" {
		t.Fatalf("files = %v", files)
	}

	prompts, err := agent.ExtractPrompts(transcriptPath, 0)
	if err != nil {
		t.Fatalf("ExtractPrompts() error = %v", err)
	}
	if len(prompts) != 1 || prompts[0] != "write a page" {
		t.Fatalf("prompts = %v", prompts)
	}

	usage, err := agent.CalculateTokenUsage([]byte(transcript), 0)
	if err != nil {
		t.Fatalf("CalculateTokenUsage() error = %v", err)
	}
	if usage.InputTokens != 11 || usage.OutputTokens != 22 {
		t.Fatalf("usage tokens = %+v", usage)
	}
	if usage.CacheReadTokens != 30 || usage.CacheCreationTokens != 40 {
		t.Fatalf("usage cache = %+v", usage)
	}
	if usage.APICallCount != 2 {
		t.Fatalf("usage.APICallCount = %d, want 2", usage.APICallCount)
	}
}

func TestPiSessionPaths(t *testing.T) {
	t.Parallel()

	if got := SanitizePathForPi("/Users/user/projects/my-pi"); got != "--Users-user-projects-my-pi--" {
		t.Fatalf("SanitizePathForPi() = %q", got)
	}

	timestamp := time.Date(2026, 4, 25, 12, 0, 0, 168000000, time.UTC)
	if got := PiSessionFileName(timestamp, sampleSessionID); got != "2026-04-25T12-00-00-168Z_"+sampleSessionID+".jsonl" {
		t.Fatalf("PiSessionFileName() = %q", got)
	}
}
