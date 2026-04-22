// Package interactive provides TTY-related helpers shared between the cli
// and strategy packages without inducing an import cycle (strategy cannot
// import cli).
package interactive

import "os"

// CanPromptInteractively reports whether interactive confirmation prompts
// (huh forms, yes/no questions, etc.) can be shown. Returns false in CI,
// tests without ENTIRE_TEST_TTY=1, and other environments without a
// controlling TTY.
//
// ENTIRE_TEST_TTY overrides every other check so tests can exercise both
// interactive and non-interactive paths deterministically without needing
// a real pty:
//   - ENTIRE_TEST_TTY=1 forces interactive mode on
//   - any other non-empty value forces interactive mode off
//   - unset falls through to the CI / /dev/tty checks below
//
// When ENTIRE_TEST_TTY is unset, CI=true short-circuits to false even if a
// /dev/tty is attached. Self-hosted runners and some Docker configurations
// inherit a TTY but cannot respond to prompts, which would otherwise hang
// the pipeline.
func CanPromptInteractively() bool {
	if v, ok := os.LookupEnv("ENTIRE_TEST_TTY"); ok {
		return v == "1"
	}

	if os.Getenv("CI") != "" {
		return false
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = tty.Close()
	return true
}
