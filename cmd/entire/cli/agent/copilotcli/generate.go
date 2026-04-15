package copilotcli

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

var copilotCommandRunner = exec.CommandContext

// GenerateText sends a prompt to the Copilot CLI and returns the raw text response.
func (c *CopilotCLIAgent) GenerateText(ctx context.Context, prompt string, model string) (string, error) {
	if err := agent.ValidateInlinePromptSize("copilot", prompt); err != nil {
		return "", fmt.Errorf("copilot text generation failed: %w", err)
	}

	// --disable-builtin-mcps skips loading the GitHub MCP server, which isn't
	// needed for summary text generation and reduces per-call input tokens.
	args := []string{"-p", prompt, "--allow-all-tools", "--disable-builtin-mcps"}
	if model != "" {
		args = append(args, "--model", model)
	}

	result, err := agent.RunIsolatedTextGeneratorCLI(ctx, copilotCommandRunner, "copilot", "copilot", args, "")
	if err != nil {
		return "", fmt.Errorf("copilot text generation failed: %w", err)
	}
	return result, nil
}
