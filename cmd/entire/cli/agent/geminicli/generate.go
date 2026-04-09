package geminicli

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

var geminiCommandRunner = exec.CommandContext

// GenerateText sends a prompt to the Gemini CLI and returns the raw text response.
func (g *GeminiCLIAgent) GenerateText(ctx context.Context, prompt string, model string) (string, error) {
	args := []string{"-p", ""}
	if model != "" {
		args = append(args, "--model", model)
	}

	result, err := agent.RunIsolatedTextGeneratorCLI(ctx, geminiCommandRunner, "gemini", "gemini", args, prompt)
	if err != nil {
		return "", fmt.Errorf("gemini text generation failed: %w", err)
	}
	return result, nil
}
