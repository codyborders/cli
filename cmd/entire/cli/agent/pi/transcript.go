package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// Compile-time interface assertions.
var _ agent.TranscriptAnalyzer = (*PiAgent)(nil)
var _ agent.PromptExtractor = (*PiAgent)(nil)
var _ agent.TokenCalculator = (*PiAgent)(nil)

type transcriptRecord struct {
	Type      string        `json:"type"`
	Timestamp string        `json:"timestamp"`
	Message   transcriptMsg `json:"message"`
}

type transcriptMsg struct {
	Role    string           `json:"role"`
	Content json.RawMessage  `json:"content"`
	Usage   *transcriptUsage `json:"usage"`
}

type transcriptUsage struct {
	Input        int `json:"input"`
	InputTokens  int `json:"inputTokens"`
	Output       int `json:"output"`
	OutputTokens int `json:"outputTokens"`
	CacheRead    int `json:"cacheRead"`
	CacheWrite   int `json:"cacheWrite"`
}

type contentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// GetTranscriptPosition returns Pi transcript position as the JSONL line count.
func (a *PiAgent) GetTranscriptPosition(path string) (int, error) {
	if strings.TrimSpace(path) == "" {
		return 0, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // Path comes from Pi's session manager hook payload.
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read Pi transcript position: %w", err)
	}
	return countJSONLLines(data), nil
}

// ExtractModifiedFilesFromOffset extracts files from Pi write/edit tool calls.
func (a *PiAgent) ExtractModifiedFilesFromOffset(path string, startOffset int) ([]string, int, error) {
	if startOffset < 0 {
		return nil, 0, fmt.Errorf("start offset cannot be negative: %d", startOffset)
	}
	data, err := os.ReadFile(path) //nolint:gosec // Path comes from Pi's session manager hook payload.
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read Pi transcript: %w", err)
	}
	currentPosition := countJSONLLines(data)
	files, extractErr := extractModifiedFilesFromTranscript(data, startOffset)
	if extractErr != nil {
		return nil, 0, extractErr
	}
	return files, currentPosition, nil
}

// ExtractPrompts returns user prompts from a Pi transcript at or after fromOffset.
func (a *PiAgent) ExtractPrompts(sessionRef string, fromOffset int) ([]string, error) {
	if fromOffset < 0 {
		return nil, fmt.Errorf("prompt offset cannot be negative: %d", fromOffset)
	}
	data, err := os.ReadFile(sessionRef) //nolint:gosec // Path comes from Pi's session manager hook payload.
	if err != nil {
		return nil, fmt.Errorf("failed to read Pi transcript for prompts: %w", err)
	}

	var prompts []string
	if err := scanTranscriptRecords(data, fromOffset, func(record transcriptRecord) {
		if record.Type != "message" || record.Message.Role != "user" {
			return
		}
		text := extractText(record.Message.Content)
		if text != "" {
			prompts = append(prompts, text)
		}
	}); err != nil {
		return nil, err
	}
	return prompts, nil
}

// CalculateTokenUsage sums Pi assistant usage records after fromOffset.
func (a *PiAgent) CalculateTokenUsage(transcriptData []byte, fromOffset int) (*agent.TokenUsage, error) {
	if fromOffset < 0 {
		return nil, fmt.Errorf("token offset cannot be negative: %d", fromOffset)
	}
	usage := &agent.TokenUsage{}
	if err := scanTranscriptRecords(transcriptData, fromOffset, func(record transcriptRecord) {
		if record.Type != "message" || record.Message.Role != "assistant" || record.Message.Usage == nil {
			return
		}
		usage.InputTokens += record.Message.Usage.Input + record.Message.Usage.InputTokens
		usage.OutputTokens += record.Message.Usage.Output + record.Message.Usage.OutputTokens
		usage.CacheReadTokens += record.Message.Usage.CacheRead
		usage.CacheCreationTokens += record.Message.Usage.CacheWrite
		usage.APICallCount++
	}); err != nil {
		return nil, err
	}
	return usage, nil
}

func extractModifiedFilesFromTranscript(transcriptData []byte, startOffset int) ([]string, error) {
	filesByPath := make(map[string]bool)
	var files []string
	err := scanTranscriptRecords(transcriptData, startOffset, func(record transcriptRecord) {
		if record.Type != "message" || record.Message.Role != "assistant" {
			return
		}
		for _, block := range decodeContentBlocks(record.Message.Content) {
			if !toolMutatesFiles(block.Name) {
				continue
			}
			path := pathFromArguments(block.Arguments)
			if path == "" || filesByPath[path] {
				continue
			}
			filesByPath[path] = true
			files = append(files, path)
		}
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func parseTranscriptStartTime(transcriptData []byte) time.Time {
	var startTime time.Time
	_ = scanTranscriptRecords(transcriptData, 0, func(record transcriptRecord) {
		if !startTime.IsZero() || record.Type != "session" {
			return
		}
		parsed, err := time.Parse(time.RFC3339Nano, record.Timestamp)
		if err == nil {
			startTime = parsed
		}
	})
	return startTime
}

func scanTranscriptRecords(transcriptData []byte, startOffset int, visit func(transcriptRecord)) error {
	if startOffset < 0 {
		return fmt.Errorf("transcript offset cannot be negative: %d", startOffset)
	}
	recordPosition := 0
	for index, line := range strings.Split(string(transcriptData), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		recordPosition++
		if recordPosition <= startOffset {
			continue
		}
		var record transcriptRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return fmt.Errorf("failed to parse Pi transcript line %d: %w", index+1, err)
		}
		visit(record)
	}
	return nil
}

func countJSONLLines(data []byte) int {
	lineCount := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			lineCount++
		}
	}
	return lineCount
}

func decodeContentBlocks(content json.RawMessage) []contentBlock {
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil
	}
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		return blocks
	}
	var block contentBlock
	if err := json.Unmarshal(content, &block); err == nil {
		return []contentBlock{block}
	}
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return []contentBlock{{Type: "text", Text: text}}
	}
	return nil
}

func extractText(content json.RawMessage) string {
	var parts []string
	for _, block := range decodeContentBlocks(content) {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func toolMutatesFiles(toolName string) bool {
	switch toolName {
	case "edit", "write", "multi_edit", "notebook_edit":
		return true
	default:
		return false
	}
}

func pathFromArguments(arguments map[string]any) string {
	if len(arguments) == 0 {
		return ""
	}
	for _, key := range []string{"path", "filePath", "file_path", "notebookPath", "notebook_path"} {
		if value, ok := arguments[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
