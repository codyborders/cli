package agents

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGeminiPromptEnv_TrustsWorkspaceForHeadlessRuns(t *testing.T) {
	t.Setenv("ENTIRE_TEST_TTY", "1")
	t.Setenv("GEMINI_CLI_TRUST_WORKSPACE", "false")

	repoDir := filepath.Join(t.TempDir(), "repo")
	env := geminiPromptEnv(repoDir)

	if got := envValue(env, "GEMINI_CLI_TRUST_WORKSPACE"); got != "true" {
		t.Fatalf("GEMINI_CLI_TRUST_WORKSPACE = %q, want true", got)
	}
	if got := envValue(env, "ENTIRE_TEST_TTY"); got != "" {
		t.Fatalf("ENTIRE_TEST_TTY = %q, want unset", got)
	}
	if got := envValue(env, "HOME"); got != geminiTestHomeDir(repoDir) {
		t.Fatalf("HOME = %q, want %q", got, geminiTestHomeDir(repoDir))
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}
