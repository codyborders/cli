package interactive

import "testing"

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

func TestCanPromptInteractively_ForcedOffForAnyNonOneValue(t *testing.T) {
	t.Setenv("ENTIRE_TEST_TTY", "yes")
	if CanPromptInteractively() {
		t.Error("CanPromptInteractively() = true; want false when ENTIRE_TEST_TTY=yes")
	}
}
