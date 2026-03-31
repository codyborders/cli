package strategy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CheckpointTokenEnvVar is the environment variable for providing a bearer token
// used to authenticate git push/fetch operations for checkpoint branches.
// The token is injected as an HTTP Authorization header for HTTPS remotes.
// SSH remotes ignore the token (with a warning).
const CheckpointTokenEnvVar = "ENTIRE_CHECKPOINT_TOKEN"

// CheckpointGitCommand creates an exec.Cmd for a git operation that may need
// checkpoint token authentication. If ENTIRE_CHECKPOINT_TOKEN is set and the
// target resolves to an HTTPS remote, the bearer token is injected via
// GIT_CONFIG_COUNT/GIT_CONFIG_KEY_*/GIT_CONFIG_VALUE_* environment variables.
//
// For SSH remotes, a warning is printed to stderr and the token is not injected.
// For empty/unset tokens, the command is returned unmodified.
//
// The target parameter is used to determine the remote protocol. It can be:
//   - A URL (e.g., "https://github.com/org/repo.git")
//   - A remote name (e.g., "origin") — resolved via `git remote get-url`
//
// The args parameter contains the full git command arguments (e.g., "push", "--no-verify", remote, branch).
func CheckpointGitCommand(ctx context.Context, target string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdin = nil // Disconnect stdin to prevent hanging in hook context

	token := strings.TrimSpace(os.Getenv(CheckpointTokenEnvVar))
	if token == "" {
		return cmd
	}

	protocol := resolveTargetProtocol(ctx, target)

	switch protocol {
	case protocolSSH:
		fmt.Fprintf(os.Stderr, "[entire] Warning: %s is set but remote uses SSH — token ignored for SSH remotes\n", CheckpointTokenEnvVar)
		return cmd
	case protocolHTTPS:
		cmd.Env = appendCheckpointTokenEnv(os.Environ(), token)
		return cmd
	default:
		// Unknown protocol (e.g., local path, or resolution failed) — don't inject
		return cmd
	}
}

// appendCheckpointTokenEnv appends GIT_CONFIG_COUNT-based env vars to inject
// an Authorization bearer token header into git HTTP requests.
// It filters out any pre-existing GIT_CONFIG_COUNT/KEY/VALUE entries to avoid
// conflicts, then appends the new ones.
func appendCheckpointTokenEnv(baseEnv []string, token string) []string {
	filtered := make([]string, 0, len(baseEnv)+3)
	for _, e := range baseEnv {
		if strings.HasPrefix(e, "GIT_CONFIG_COUNT=") ||
			strings.HasPrefix(e, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(e, "GIT_CONFIG_VALUE_") {
			continue
		}
		filtered = append(filtered, e)
	}
	return append(filtered,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Bearer "+token,
	)
}

// resolveTargetProtocol determines whether a push/fetch target uses SSH or HTTPS.
// Returns protocolSSH, protocolHTTPS, or "" if unknown.
func resolveTargetProtocol(ctx context.Context, target string) string {
	var rawURL string
	if isURL(target) {
		rawURL = target
	} else {
		// Remote name — resolve to URL
		var err error
		rawURL, err = getRemoteURL(ctx, target)
		if err != nil {
			return ""
		}
	}

	info, err := parseGitRemoteURL(rawURL)
	if err != nil {
		return ""
	}
	return info.protocol
}
