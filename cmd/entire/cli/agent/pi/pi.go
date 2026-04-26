// Package pi implements the Agent interface for Pi.
package pi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/agent/types"
	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/entireio/cli/cmd/entire/cli/validation"
)

//nolint:gochecknoinits // Agent self-registration is the intended pattern.
func init() {
	agent.Register(agent.AgentNamePi, NewPiAgent)
}

// PiAgent implements the Agent interface for Pi.
type PiAgent struct{}

// NewPiAgent creates a new Pi agent instance.
func NewPiAgent() agent.Agent {
	return &PiAgent{}
}

func (a *PiAgent) Name() types.AgentName   { return agent.AgentNamePi }
func (a *PiAgent) Type() types.AgentType   { return agent.AgentTypePi }
func (a *PiAgent) Description() string     { return "Pi - terminal coding agent" }
func (a *PiAgent) IsPreview() bool         { return true }
func (a *PiAgent) ProtectedDirs() []string { return []string{".pi"} }

func (a *PiAgent) DetectPresence(ctx context.Context) (bool, error) {
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		repoRoot = "."
	}

	if _, err := os.Stat(filepath.Join(repoRoot, ".pi")); err == nil {
		return true, nil
	}
	return false, nil
}

// ReadTranscript reads the Pi JSONL transcript for a session.
func (a *PiAgent) ReadTranscript(sessionRef string) ([]byte, error) {
	if strings.TrimSpace(sessionRef) == "" {
		return nil, fmt.Errorf("Pi transcript path cannot be empty")
	}
	data, err := os.ReadFile(sessionRef) //nolint:gosec // Path comes from Pi's session manager hook payload.
	if err != nil {
		return nil, fmt.Errorf("failed to read Pi transcript: %w", err)
	}
	return data, nil
}

// ChunkTranscript splits Pi's JSONL transcript at line boundaries.
func (a *PiAgent) ChunkTranscript(_ context.Context, content []byte, maxSize int) ([][]byte, error) {
	return agent.ChunkJSONL(content, maxSize)
}

// ReassembleTranscript joins Pi JSONL transcript chunks.
func (a *PiAgent) ReassembleTranscript(chunks [][]byte) ([]byte, error) {
	return agent.ReassembleJSONL(chunks), nil
}

func (a *PiAgent) GetSessionID(input *agent.HookInput) string {
	if input == nil {
		return ""
	}
	return input.SessionID
}

// GetSessionDir returns Pi's per-project session directory for repoPath.
func (a *PiAgent) GetSessionDir(repoPath string) (string, error) {
	if override := os.Getenv("ENTIRE_TEST_PI_SESSION_DIR"); override != "" {
		return override, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := SanitizePathForPi(repoPath)
	return filepath.Join(homeDir, ".pi", "agent", "sessions", projectDir), nil
}

// ResolveSessionFile returns the best-known Pi transcript path for a session.
func (a *PiAgent) ResolveSessionFile(sessionDir, agentSessionID string) string {
	if strings.TrimSpace(agentSessionID) == "" {
		return ""
	}
	if filepath.IsAbs(agentSessionID) || strings.HasSuffix(agentSessionID, ".jsonl") {
		return agentSessionID
	}
	if err := validation.ValidateSessionID(agentSessionID); err != nil {
		return ""
	}
	if sessionDir == "" {
		return ""
	}

	matches, err := filepath.Glob(filepath.Join(sessionDir, "*_"+agentSessionID+".jsonl"))
	if err == nil && len(matches) > 0 {
		sort.Strings(matches)
		return matches[len(matches)-1]
	}

	return filepath.Join(sessionDir, PiSessionFileName(time.Now(), agentSessionID))
}

// ResolveRestoredSessionFile returns the Pi transcript path for restored session data.
func (a *PiAgent) ResolveRestoredSessionFile(sessionDir, agentSessionID string, transcriptData []byte) (string, error) {
	if sessionDir == "" {
		return "", fmt.Errorf("Pi session directory cannot be empty")
	}
	if strings.TrimSpace(agentSessionID) == "" {
		return "", fmt.Errorf("Pi session ID cannot be empty")
	}
	if err := validation.ValidateSessionID(agentSessionID); err != nil {
		return "", fmt.Errorf("invalid Pi session ID: %w", err)
	}
	timestamp := parseTranscriptStartTime(transcriptData)
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	return filepath.Join(sessionDir, PiSessionFileName(timestamp, agentSessionID)), nil
}

// ReadSession reads a Pi session from the transcript path carried by a hook.
func (a *PiAgent) ReadSession(input *agent.HookInput) (*agent.AgentSession, error) {
	if input == nil {
		return nil, fmt.Errorf("Pi hook input cannot be nil")
	}
	if input.SessionRef == "" {
		return nil, fmt.Errorf("Pi session_ref cannot be empty")
	}

	data, err := a.ReadTranscript(input.SessionRef)
	if err != nil {
		return nil, err
	}
	modifiedFiles, extractErr := extractModifiedFilesFromTranscript(data, 0)
	if extractErr != nil {
		return nil, extractErr
	}
	return &agent.AgentSession{
		SessionID:     input.SessionID,
		AgentName:     a.Name(),
		SessionRef:    input.SessionRef,
		StartTime:     parseTranscriptStartTime(data),
		NativeData:    data,
		ModifiedFiles: modifiedFiles,
	}, nil
}

// WriteSession writes restored Pi JSONL content so `pi --session <id>` can find it.
func (a *PiAgent) WriteSession(_ context.Context, session *agent.AgentSession) error {
	if session == nil {
		return fmt.Errorf("Pi session cannot be nil")
	}
	if strings.TrimSpace(session.SessionRef) == "" {
		return fmt.Errorf("Pi session_ref cannot be empty")
	}
	if len(session.NativeData) == 0 {
		return fmt.Errorf("Pi session data cannot be empty")
	}

	if err := os.MkdirAll(filepath.Dir(session.SessionRef), 0o700); err != nil {
		return fmt.Errorf("failed to create Pi session directory: %w", err)
	}
	if err := os.WriteFile(session.SessionRef, session.NativeData, 0o600); err != nil {
		return fmt.Errorf("failed to write Pi session: %w", err)
	}
	return nil
}

func (a *PiAgent) FormatResumeCommand(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return "pi --resume"
	}
	return fmt.Sprintf("pi --session %s", sessionID)
}

// SanitizePathForPi matches Pi's per-project session directory naming scheme.
func SanitizePathForPi(repoPath string) string {
	cleanPath := filepath.Clean(repoPath)
	normalizedPath := filepath.ToSlash(cleanPath)
	normalizedPath = strings.Trim(normalizedPath, "/")
	normalizedPath = strings.ReplaceAll(normalizedPath, ":", "")
	if normalizedPath == "." {
		normalizedPath = ""
	}
	if normalizedPath == "" {
		return "----"
	}
	return "--" + strings.ReplaceAll(normalizedPath, "/", "-") + "--"
}

// PiSessionFileName returns Pi's JSONL filename format for a timestamp and session ID.
func PiSessionFileName(timestamp time.Time, sessionID string) string {
	formatted := timestamp.UTC().Format("2006-01-02T15-04-05.000Z")
	formatted = strings.Replace(formatted, ".", "-", 1)
	return fmt.Sprintf("%s_%s.jsonl", formatted, sessionID)
}
