//go:build windows

package agents

import (
	"context"
	"os"
	"path/filepath"
)

// cleanConfigDir creates an isolated temp directory for CLAUDE_CONFIG_DIR so
// that E2E test runs don't inherit any user settings (CLAUDE.md, skills,
// projects, plugins, etc.).
//
// On CI, it copies .claude.json (which Bootstrap() wrote with the API key
// and hasCompletedOnboarding). We copy instead of symlinking because Windows
// symlinks require elevated privileges or Developer Mode.
// Locally, it writes a minimal .claude.json to skip the onboarding flow.
func cleanConfigDir() (string, error) {
	dst, err := os.MkdirTemp("", "claude-config-*")
	if err != nil {
		return "", err
	}

	if os.Getenv("CI") != "" {
		if home, err := os.UserHomeDir(); err == nil {
			src := filepath.Join(home, ".claude", ".claude.json")
			if data, err := os.ReadFile(src); err == nil {
				_ = os.WriteFile(filepath.Join(dst, ".claude.json"), data, 0o644)
			}
		}
	} else {
		config := `{"hasCompletedOnboarding":true}`
		if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
			config = `{"hasCompletedOnboarding":true,"primaryApiKey":"` + apiKey + `"}`
		}
		_ = os.WriteFile(filepath.Join(dst, ".claude.json"), []byte(config), 0o644)
	}

	return dst, nil
}

// StartSession returns nil on Windows because ConPTY cannot reliably deliver
// key escape sequences to huh/bubbletea TUI forms. Tests that require
// interactive sessions are skipped on Windows.
func (c *Claude) StartSession(_ context.Context, _ string) (Session, error) {
	return nil, nil
}
