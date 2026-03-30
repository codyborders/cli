//go:build unix

package agents

import (
	"os"
	"path/filepath"
)

// cleanConfigDir creates an isolated temp directory for CLAUDE_CONFIG_DIR so
// that E2E test runs don't inherit any user settings (CLAUDE.md, skills,
// projects, plugins, etc.).
//
// On CI, it symlinks .claude.json (which Bootstrap() wrote with the API key
// and hasCompletedOnboarding). Locally, it writes a minimal .claude.json to
// skip the onboarding flow — Keychain-based auth works without any other files.
func cleanConfigDir() (string, error) {
	dst, err := os.MkdirTemp("", "claude-config-*")
	if err != nil {
		return "", err
	}

	if os.Getenv("CI") != "" {
		if home, err := os.UserHomeDir(); err == nil {
			src := filepath.Join(home, ".claude", ".claude.json")
			if _, err := os.Stat(src); err == nil {
				_ = os.Symlink(src, filepath.Join(dst, ".claude.json"))
			}
		}
	} else {
		_ = os.WriteFile(filepath.Join(dst, ".claude.json"),
			[]byte(`{"hasCompletedOnboarding":true}`), 0o644)
	}

	return dst, nil
}
