package pi

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

const (
	HookNameSessionStart     = "session-start"
	HookNameUserPromptSubmit = "user-prompt-submit"
	HookNameStop             = "stop"
	HookNamePreCompact       = "pre-compact"
	HookNameSessionEnd       = "session-end"
)

type hookPayload struct {
	HookType   string         `json:"hook_type"`
	SessionID  string         `json:"session_id"`
	SessionRef string         `json:"session_ref"`
	Timestamp  string         `json:"timestamp"`
	CWD        string         `json:"cwd"`
	Prompt     string         `json:"prompt"`
	Model      string         `json:"model"`
	RawData    map[string]any `json:"raw_data"`
}

// HookNames returns the hook verbs supported under `entire hooks pi`.
func (a *PiAgent) HookNames() []string {
	return []string{
		HookNameSessionStart,
		HookNameUserPromptSubmit,
		HookNameStop,
		HookNamePreCompact,
		HookNameSessionEnd,
	}
}

// ParseHookEvent translates Pi extension payloads into normalized lifecycle events.
func (a *PiAgent) ParseHookEvent(_ context.Context, hookName string, stdin io.Reader) (*agent.Event, error) {
	payload, err := agent.ReadAndParseHookInput[hookPayload](stdin)
	if err != nil {
		return nil, err
	}

	eventType, ok := eventTypeForHook(hookName)
	if !ok {
		return nil, nil //nolint:nilnil // nil event = no lifecycle action for unknown hooks.
	}

	return &agent.Event{
		Type:       eventType,
		SessionID:  payload.SessionID,
		SessionRef: payload.SessionRef,
		Prompt:     payload.Prompt,
		Model:      payload.Model,
		Timestamp:  parseHookTimestamp(payload.Timestamp),
		Metadata:   metadataFromRawData(payload),
	}, nil
}

func eventTypeForHook(hookName string) (agent.EventType, bool) {
	switch hookName {
	case HookNameSessionStart:
		return agent.SessionStart, true
	case HookNameUserPromptSubmit:
		return agent.TurnStart, true
	case HookNameStop:
		return agent.TurnEnd, true
	case HookNamePreCompact:
		return agent.Compaction, true
	case HookNameSessionEnd:
		return agent.SessionEnd, true
	default:
		return 0, false
	}
}

func parseHookTimestamp(value string) time.Time {
	if value == "" {
		return time.Now()
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Now()
	}
	return parsed
}

func metadataFromRawData(payload *hookPayload) map[string]string {
	metadata := make(map[string]string)
	if payload == nil {
		return metadata
	}
	if payload.CWD != "" {
		metadata["cwd"] = payload.CWD
	}
	if payload.HookType != "" {
		metadata["hook_type"] = payload.HookType
	}
	for key, value := range payload.RawData {
		if rendered := renderMetadataValue(value); rendered != "" {
			metadata[key] = rendered
		}
	}
	return metadata
}

func renderMetadataValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return fmt.Sprint(typed)
	}
}
