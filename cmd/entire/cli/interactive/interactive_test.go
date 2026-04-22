package interactive

import "testing"

func TestHasTTY_ForcedOn(t *testing.T) {
	t.Setenv("ENTIRE_TEST_TTY", "1")
	if !HasTTY() {
		t.Error("HasTTY() = false; want true when ENTIRE_TEST_TTY=1")
	}
}

func TestHasTTY_ForcedOff(t *testing.T) {
	t.Setenv("ENTIRE_TEST_TTY", "0")
	if HasTTY() {
		t.Error("HasTTY() = true; want false when ENTIRE_TEST_TTY=0")
	}
}
