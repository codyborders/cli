package codex

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

var codexCommandRunner = exec.CommandContext

// GenerateText sends a prompt to the Codex CLI and returns the raw text response.
func (c *CodexAgent) GenerateText(ctx context.Context, prompt string, model string) (string, error) {
	args := []string{"exec", "--skip-git-repo-check"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "-")

	result, err := agent.RunIsolatedTextGeneratorCLI(ctx, codexCommandRunner, "codex", "codex", args, prompt)
	if err != nil {
		return "", fmt.Errorf("codex text generation failed: %w", err)
	}
	return result, nil
}
