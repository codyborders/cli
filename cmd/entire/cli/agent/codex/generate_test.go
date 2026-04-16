package codex

import (
	"context"
	"os/exec"
	"slices"
	"testing"
)

// TestGenerateText_PromptViaStdin verifies that the prompt is passed to the
// Codex CLI via stdin (signaled by the trailing "-" sentinel arg), not as a
// CLI argument, and that expected flags are present. Uses `cat` as a fake
// runner so the stdin round-trip is end-to-end observable through the
// returned output.
func TestGenerateText_PromptViaStdin(t *testing.T) {
	// Not parallel: mutates package-level codexCommandRunner.
	originalRunner := codexCommandRunner
	t.Cleanup(func() {
		codexCommandRunner = originalRunner
	})

	var capturedArgs []string
	codexCommandRunner = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		capturedArgs = args
		return exec.CommandContext(ctx, "cat")
	}

	ag := &CodexAgent{}
	prompt := "this prompt must arrive via stdin, not argv"
	result, err := ag.GenerateText(context.Background(), prompt, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != prompt {
		t.Fatalf("stdin round-trip failed: result=%q, want=%q", result, prompt)
	}

	if slices.Contains(capturedArgs, prompt) {
		t.Fatalf("prompt leaked into argv: %v", capturedArgs)
	}
	for _, expected := range []string{"exec", "--skip-git-repo-check"} {
		if !slices.Contains(capturedArgs, expected) {
			t.Fatalf("expected %q in args, got %v", expected, capturedArgs)
		}
	}
	// The trailing "-" sentinel tells codex to read the prompt from stdin.
	// Without it, the run would either hang waiting for a prompt or misbehave.
	if len(capturedArgs) == 0 || capturedArgs[len(capturedArgs)-1] != "-" {
		t.Fatalf("expected trailing %q stdin sentinel, got %v", "-", capturedArgs)
	}
}

func TestGenerateText_ModelFlagPassedWhenSet(t *testing.T) {
	// Not parallel: mutates package-level codexCommandRunner.
	originalRunner := codexCommandRunner
	t.Cleanup(func() {
		codexCommandRunner = originalRunner
	})

	var capturedArgs []string
	codexCommandRunner = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		capturedArgs = args
		return exec.CommandContext(ctx, "cat")
	}

	ag := &CodexAgent{}
	if _, err := ag.GenerateText(context.Background(), "prompt", "gpt-5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	modelIdx := slices.Index(capturedArgs, "--model")
	if modelIdx < 0 || modelIdx+1 >= len(capturedArgs) {
		t.Fatalf("expected --model in args, got %v", capturedArgs)
	}
	if capturedArgs[modelIdx+1] != "gpt-5" {
		t.Fatalf("expected --model gpt-5, got %v", capturedArgs)
	}
	// --model must come before the "-" sentinel so codex parses it as an option.
	sentinelIdx := slices.Index(capturedArgs, "-")
	if sentinelIdx < modelIdx {
		t.Fatalf("expected --model before %q sentinel, got %v", "-", capturedArgs)
	}
}
