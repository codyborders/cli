package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// GenerateText sends a prompt to the Claude CLI and returns the raw text response.
// Implements the agent.TextGenerator interface.
// The model parameter hints which model to use (e.g., "haiku", "sonnet").
// If empty, defaults to "haiku" for fast, cheap generation.
func (c *ClaudeCodeAgent) GenerateText(ctx context.Context, prompt string, model string) (string, error) {
	claudePath := "claude"
	if model == "" {
		model = "haiku"
	}

	args := []string{
		"--print", "--output-format", "json",
		"--model", model, "--setting-sources", "",
	}
	stdoutText, err := agent.RunIsolatedTextGeneratorCLI(ctx, exec.CommandContext, claudePath, "claude", args, prompt)
	if err != nil {
		return "", fmt.Errorf("claude text generation failed: %w", err)
	}

	// Parse the {"result": "..."} envelope
	var response struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdoutText), &response); err != nil {
		return "", fmt.Errorf("failed to parse claude CLI response: %w", err)
	}

	return response.Result, nil
}
