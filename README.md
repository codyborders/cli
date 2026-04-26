# Entire CLI

Entire records AI coding sessions for a repository and links them to normal git commits. It stores transcripts, prompts, file lists, tool activity, token usage, and checkpoint metadata on the separate `entire/checkpoints/v1` branch. Your working branch keeps only the commits you make.

Use Entire when you need to answer why a change happened, restore files from a previous agent step, or resume a tracked session on another machine.

## Requirements

- Git
- macOS, Linux, or Windows
- An installed and authenticated [supported agent](#agent-hook-configuration)

## Quick Start

```bash
# Install stable via Homebrew
brew tap entireio/tap
brew install --cask entire

# Or install nightly via Homebrew
brew tap entireio/tap
brew install --cask entire@nightly

# Or install stable via install.sh
curl -fsSL https://entire.io/install.sh | bash

# Or install nightly via install.sh
curl -fsSL https://entire.io/install.sh | bash -s -- --channel nightly

# Or install stable via Scoop (Windows)
scoop bucket add entire https://github.com/entireio/scoop-bucket.git
scoop install entire/cli

# Or install via Go (development/manual setup)
go install github.com/entireio/cli/cmd/entire@latest

# Linux: Add Go binaries to PATH if needed
export PATH="$HOME/go/bin:$PATH"

# Enable in your project
cd your-project && entire enable

# Check tracking state
entire status
```

After first setup, use `entire configure` to add or remove agents and change setup options. Use `entire enable` and `entire disable` only to toggle tracking.

## Release Channels

Entire publishes two channels:

| Channel | Use case | Install |
| --- | --- | --- |
| `stable` | Default release for normal use. | `brew install --cask entire`, `curl -fsSL https://entire.io/install.sh \| bash`, or `scoop install entire/cli` |
| `nightly` | Prerelease builds with newer changes. | `brew install --cask entire@nightly` or `curl -fsSL https://entire.io/install.sh \| bash -s -- --channel nightly` |

## Typical Workflow

### 1. Enable Entire in Your Repository

```bash
entire enable
```

On a new repository, `entire enable` creates settings, installs git hooks, and asks which agent hooks to install. To choose an agent without the prompt, pass `--agent`:

```bash
entire enable --agent cursor
```

Once the repo is configured, `entire enable` turns tracking back on if it was disabled. Use `entire configure` when you want to change installed agents or update setup fields.

Entire does not commit to your active branch. It records session metadata on `entire/checkpoints/v1` when you commit.

### 2. Work with Your AI Agent

Use your agent normally. Entire receives lifecycle events from the installed agent hooks and records the active session in the background.

```bash
entire status
```

### 3. Rewind to a Checkpoint

```bash
entire rewind
```

`entire rewind` lists checkpoints for the current session. Choose one to restore the tracked files to that state without rewriting your git history.

### 4. Resume a Session

```bash
entire resume <branch>
```

`entire resume` checks out the branch, restores checkpointed session metadata, and prints the agent command needed to continue.

### 5. Disable Entire

```bash
entire disable
```

This removes Entire hooks. It does not remove your code commits.

## Key Concepts

### Sessions

A session is one tracked interaction with an AI agent. It includes prompts, responses, modified files, timestamps, tool calls, and token usage.

Session IDs use this format:

```text
YYYY-MM-DD-<UUID>
```

Example:

```text
2026-01-08-abc123de-f456-7890-abcd-ef1234567890
```

Session data is stored on `entire/checkpoints/v1`, not on your working branch.

### Checkpoints

A checkpoint is a saved state inside a session. Checkpoints are created when you or the agent make a git commit. Checkpoint IDs are 12-character hex strings, such as `a3b2c4d5e6f7`.

### How It Works

```text
Your Branch                    entire/checkpoints/v1
     │                                  │
     ▼                                  │
[Base Commit]                           │
     │                                  │
     │  ┌─── Agent works ───┐           │
     │  │  Step 1           │           │
     │  │  Step 2           │           │
     │  │  Step 3           │           │
     │  └───────────────────┘           │
     │                                  │
     ▼                                  ▼
[Your Commit] ─────────────────► [Session Metadata]
     │                           (transcript, prompts,
     │                            files touched)
     ▼
```

When you commit, Entire writes the matching session metadata to `entire/checkpoints/v1` and links it to the commit.

### Strategy

Entire uses a manual-commit strategy. It never creates commits on your active branch, works from the branch you already use, restores files without rewriting history, and keeps session metadata on `entire/checkpoints/v1`.

### Git Worktrees

Git worktrees get independent session tracking. You can run separate agent sessions in separate worktrees without sharing session state between them.

### Concurrent Sessions

Multiple AI sessions can run from the same commit. If you start another session while a previous one has uncommitted work, Entire warns you and tracks both sessions separately.

## Local Device Auth Testing

Use this flow when testing the CLI device auth flow against a local `entire.io` checkout:

```bash
# In your app repo
cd ../entire.io-1
mise run dev

# In this repo, point the CLI at the local API
cd ../cli
export ENTIRE_API_BASE_URL=http://localhost:8787

# Run the smoke test
./scripts/local-device-auth-smoke.sh
```

Useful development commands:

```bash
# Run the login flow against a local server
# The command prompts before opening the browser.
go run ./cmd/entire login --insecure-http-auth

# Run focused integration coverage for login
go test -tags=integration ./cmd/entire/cli/integration_test -run TestLogin
```

## Commands Reference

| Command | Description |
| --- | --- |
| `entire clean` | Clean session data and orphaned Entire data. Use `--all` for repo-wide cleanup. |
| `entire configure` | Configure agents and setup options for the current repository. |
| `entire disable` | Remove Entire hooks from the repository. |
| `entire doctor` | Fix or clean stuck sessions. |
| `entire enable` | Enable Entire in the repository. |
| `entire explain` | Explain a session or commit. |
| `entire login` | Authenticate with Entire device auth. |
| `entire resume` | Switch to a branch, restore session metadata, and print continuation commands. |
| `entire rewind` | Restore files from a previous checkpoint. |
| `entire status` | Show current session info. |
| `entire sessions` | View and manage tracked agent sessions. |
| `entire version` | Show the CLI version. |

### `entire enable` Flags

| Flag | Description |
| --- | --- |
| `--agent <name>` | AI agent to install hooks for: `claude-code`, `codex`, `gemini`, `opencode`, `pi`, `cursor`, `factoryai-droid`, or `copilot-cli`. |
| `--force`, `-f` | Reinstall hooks after removing existing Entire hooks. |
| `--local` | Write settings to `settings.local.json` instead of `settings.json`. |
| `--project` | Write settings to `settings.json` even if it already exists. |
| `--skip-push-sessions` | Disable automatic pushes for session logs on git push. |
| `--checkpoint-remote <provider:owner/repo>` | Push checkpoint branches to a separate repo, such as `github:org/checkpoints-repo`. |
| `--telemetry=false` | Disable anonymous usage analytics. |

Examples:

```bash
entire enable --agent claude-code
entire enable
entire enable --force
entire enable --local
```

`entire enable` is mainly a toggle. On an unconfigured repo, it also bootstraps setup. Once the repo is configured, `entire configure` is the clearer command for agent and setup changes.

### `entire configure`

Use `entire configure` after setup to add or remove agents, reinstall hooks for selected agents, or update settings such as `--checkpoint-remote` and `--skip-push-sessions`.

```bash
# Add or remove agents interactively
entire configure

# Install or refresh hooks for one agent
entire configure --agent claude-code --force

# Update setup settings
entire configure --checkpoint-remote github:myorg/checkpoints-private

# Remove one agent's hooks
entire configure --remove claude-code
```

## Configuration

Entire reads settings from `.entire/settings.json` and `.entire/settings.local.json`.

### settings.json

Shared project settings usually go in git:

```json
{
  "enabled": true
}
```

### settings.local.json

Personal overrides stay local and are gitignored by default:

```json
{
  "enabled": false,
  "log_level": "debug"
}
```

### Configuration Options

| Option | Values | Description |
| --- | --- | --- |
| `enabled` | `true`, `false` | Enable or disable Entire. |
| `log_level` | `debug`, `info`, `warn`, `error` | Logging verbosity. |
| `strategy_options.push_sessions` | `true`, `false` | Push `entire/checkpoints/v1` automatically on git push. |
| `strategy_options.checkpoint_remote` | `{"provider": "github", "repo": "org/repo"}` | Push checkpoint branches to a separate repo. |
| `strategy_options.summarize.enabled` | `true`, `false` | Generate AI summaries at commit time. |
| `telemetry` | `true`, `false` | Send anonymous usage statistics to Posthog. |

Local settings override project settings field by field. `entire status` shows both project and local values.

### Agent Hook Configuration

Each agent stores hook configuration in its own project directory. `entire enable` installs hooks for the selected agents in these locations:

| Agent | Hook Location | Format |
| --- | --- | --- |
| Claude Code | `.claude/settings.json` | JSON hooks config |
| Codex | `.codex/hooks.json` | JSON hooks config |
| Copilot CLI | `.github/hooks/entire.json` | JSON hooks config |
| Cursor | `.cursor/hooks.json` | JSON hooks config |
| Factory AI Droid | `.factory/settings.json` | JSON hooks config |
| Gemini CLI | `.gemini/settings.json` | JSON hooks config |
| OpenCode | `.opencode/plugins/entire.ts` | TypeScript plugin |
| Pi | `.pi/extensions/entire/index.ts` | TypeScript extension |

You can enable more than one agent in the same repository. Entire detects active agents by checking for installed hooks rather than by reading a single setting.

### Checkpoint Remote

By default, Entire pushes `entire/checkpoints/v1` to the same remote as your code. To push checkpoint data to a separate repository, set `checkpoint_remote`:

```json
{
  "strategy_options": {
    "checkpoint_remote": {
      "provider": "github",
      "repo": "myorg/checkpoints-private"
    }
  }
}
```

Or set it from the CLI:

```bash
entire enable --checkpoint-remote github:myorg/checkpoints-private
```

Entire derives the git URL using the same protocol as your push remote. It fetches an existing checkpoint branch when needed, pushes `entire/checkpoints/v1` to the checkpoint repo, skips pushes when a fork owner does not match the checkpoint repo owner, and warns without blocking your main push when the checkpoint remote is unavailable.

#### `ENTIRE_CHECKPOINT_TOKEN`

`ENTIRE_CHECKPOINT_TOKEN` provides a dedicated token for checkpoint repository operations without changing credentials for your main repo.

When the token is set, Entire injects it into HTTPS checkpoint fetch and push operations. If `checkpoint_remote` is configured, Entire prefers an HTTPS URL for that remote. If `checkpoint_remote` is missing or cannot be loaded, Entire falls back to `origin` and converts SSH or HTTPS remotes to HTTPS for token-based auth when possible.

### Auto-Summarization

Entire can generate AI summaries for checkpoints at commit time:

```json
{
  "strategy_options": {
    "summarize": {
      "enabled": true
    }
  }
}
```

This requires an installed and authenticated `claude` CLI. Summary failures are logged and do not block commits. Other summary backends may be added later.

### Agent-Specific Steps and Limits

Codex setup also writes `.codex/config.toml` with `codex_hooks = true`. If you configure Codex manually, keep that setting enabled.

Cursor IDE and Cursor Agent CLI support `doctor`, `status`, and related commands, but `entire rewind` is not available for Cursor yet.

Copilot CLI is supported. Copilot in VS Code, other IDEs, and github.com is not supported.

## Security & Privacy

Session transcripts are stored in your git repository on `entire/checkpoints/v1`. If your repository is public, checkpoint data is public too.

Entire redacts detected secrets such as API keys and other credentials before writing to `entire/checkpoints/v1`, but redaction is best-effort. Temporary shadow branches used during a session may contain unredacted data and should not be pushed. See [docs/security-and-privacy.md](docs/security-and-privacy.md).

## Troubleshooting

### Common Issues

| Issue | Solution |
| --- | --- |
| "Not a git repository" | Change into a Git repository first. |
| "Entire is disabled" | Run `entire enable`. |
| "No rewind points found" | Work with the configured agent, then commit your changes. |
| "shadow branch conflict" | Run `entire clean --force`. |

### SSH Authentication Errors

If `entire resume` fails with this error:

```text
Failed to fetch metadata: failed to fetch entire/checkpoints/v1 from origin: ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain
```

Add GitHub's host keys to `known_hosts`:

```bash
ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts
ssh-keyscan -t ecdsa github.com >> ~/.ssh/known_hosts
```

This works around a [go-git SSH issue](https://github.com/go-git/go-git/issues/411).

### Debug Mode

```bash
# Via environment variable
ENTIRE_LOG_LEVEL=debug entire status

# Or via settings.local.json
{
  "log_level": "debug"
}
```

### Cleaning Up State

```bash
# Clean session data for current commit
entire clean --force

# Clean all orphaned data across the repository
entire clean --all --force

# Disable and re-enable
entire disable && entire enable --force
```

### Accessibility

For screen reader users, enable accessible mode:

```bash
export ACCESSIBLE=1
entire enable
```

Accessible mode uses text prompts instead of interactive TUI elements.

## Development

This project uses [mise](https://mise.jdx.dev/) for task automation and dependency management.

### Prerequisites

- [mise](https://mise.jdx.dev/). Install with `curl https://mise.run | sh`.

### Getting Started

```bash
git clone https://github.com/entireio/cli.git
cd cli
mise install
mise trust
mise run build
```

### Dev Container

The repo includes a `.devcontainer/` configuration that installs the system packages used by local development and CI (`git`, `tmux`, `gnome-keyring`, and others) and then bootstraps the repo's `mise` toolchain.

Start it with the `devcontainer` CLI:

```bash
devcontainer up --workspace-folder .
devcontainer exec --workspace-folder . bash -lc '.devcontainer/run-with-keyring.sh'
```

The container's `postCreateCommand` runs `mise trust --yes && mise install`, so Go, `golangci-lint`, `gotestsum`, `shellcheck`, and the canary E2E helper binaries are available after creation. Use `.devcontainer/run-with-keyring.sh <command>` for commands that touch the Linux keyring, including `mise run test:ci`.

If `ENTIRE_DEVCONTAINER_KEYRING_PASSWORD` is set, `.devcontainer/run-with-keyring.sh` uses it for the keyring. If it is unset, the script generates a random password for the session.

### Common Tasks

```bash
mise run test
mise run test:integration
mise run test:ci
mise run lint
mise run fmt
```

## Getting Help

```bash
entire --help
entire <command> --help
```

Report bugs or request features at https://github.com/entireio/cli/issues. See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.

## License

MIT License. See [LICENSE](LICENSE) for details.
