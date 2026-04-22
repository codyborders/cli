package interactive

import (
	"os"
	"testing"
)

func TestCanPromptInteractively_ForcedOn(t *testing.T) {
	t.Setenv("ENTIRE_TEST_TTY", "1")
	if !CanPromptInteractively() {
		t.Error("CanPromptInteractively() = false; want true when ENTIRE_TEST_TTY=1")
	}
}

func TestCanPromptInteractively_ForcedOff(t *testing.T) {
	t.Setenv("ENTIRE_TEST_TTY", "0")
	if CanPromptInteractively() {
		t.Error("CanPromptInteractively() = true; want false when ENTIRE_TEST_TTY=0")
	}
}

func TestCanPromptInteractively_CI(t *testing.T) {
	// Unset the test-override so CI actually gets consulted.
	t.Setenv("ENTIRE_TEST_TTY", "")
	os.Unsetenv("ENTIRE_TEST_TTY")
	t.Setenv("CI", "true")
	if CanPromptInteractively() {
		t.Error("CanPromptInteractively() = true; want false when CI is set")
	}
}

func TestCanPromptInteractively_TestOverrideBeatsCI(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("ENTIRE_TEST_TTY", "1")
	if !CanPromptInteractively() {
		t.Error("CanPromptInteractively() = false; want true when ENTIRE_TEST_TTY=1 overrides CI")
	}
}
