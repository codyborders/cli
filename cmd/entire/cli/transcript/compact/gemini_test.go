package compact

import (
	"testing"
)

func TestCompact_GeminiFixture(t *testing.T) {
	t.Parallel()
	assertFixtureTransform(t, agentOpts("gemini-cli"), "testdata/gemini_full.jsonl", "testdata/gemini_expected.jsonl")
}

func TestCompact_GeminiTokenUsage(t *testing.T) {
	t.Parallel()

	input := []byte(`{
		"sessionId":"ses-1",
		"messages":[
			{
				"id":"msg-u1","timestamp":"2026-01-01T00:00:00Z","type":"user",
				"content":"hello"
			},
			{
				"id":"msg-a1","timestamp":"2026-01-01T00:00:01Z","type":"gemini",
				"content":"Hi there!",
				"tokens":{"input":200,"output":75,"cached":0,"thoughts":10,"tool":0,"total":285}
			}
		]
	}`)

	expected := []string{
		`{"v":1,"agent":"gemini-cli","cli_version":"0.5.1","type":"user","ts":"2026-01-01T00:00:00Z","content":[{"text":"hello"}]}`,
		`{"v":1,"agent":"gemini-cli","cli_version":"0.5.1","type":"assistant","ts":"2026-01-01T00:00:01Z","id":"msg-a1","input_tokens":200,"output_tokens":75,"content":[{"type":"text","text":"Hi there!"}]}`,
	}

	result, err := Compact(input, agentOpts("gemini-cli"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONLines(t, result, expected)
}

func TestCompact_GeminiNoTokensOmitsFields(t *testing.T) {
	t.Parallel()

	input := []byte(`{
		"sessionId":"ses-1",
		"messages":[
			{
				"id":"msg-a1","timestamp":"2026-01-01T00:00:01Z","type":"gemini",
				"content":"no tokens here"
			}
		]
	}`)

	expected := []string{
		`{"v":1,"agent":"gemini-cli","cli_version":"0.5.1","type":"assistant","ts":"2026-01-01T00:00:01Z","id":"msg-a1","content":[{"type":"text","text":"no tokens here"}]}`,
	}

	result, err := Compact(input, agentOpts("gemini-cli"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONLines(t, result, expected)
}

func TestCompact_GeminiTextOnlyAssistant(t *testing.T) {
	t.Parallel()

	input := []byte(`{
		"sessionId":"ses-1",
		"messages":[
			{
				"id":"msg-u1","timestamp":"2026-01-01T00:00:00Z","type":"user",
				"content":"what is go?"
			},
			{
				"id":"msg-a1","timestamp":"2026-01-01T00:00:01Z","type":"gemini",
				"content":"Go is a programming language.",
				"tokens":{"input":50,"output":20}
			}
		]
	}`)

	expected := []string{
		`{"v":1,"agent":"gemini-cli","cli_version":"0.5.1","type":"user","ts":"2026-01-01T00:00:00Z","content":[{"text":"what is go?"}]}`,
		`{"v":1,"agent":"gemini-cli","cli_version":"0.5.1","type":"assistant","ts":"2026-01-01T00:00:01Z","id":"msg-a1","input_tokens":50,"output_tokens":20,"content":[{"type":"text","text":"Go is a programming language."}]}`,
	}

	result, err := Compact(input, agentOpts("gemini-cli"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONLines(t, result, expected)
}

func TestCompact_GeminiInfoMessagesDropped(t *testing.T) {
	t.Parallel()

	input := []byte(`{
		"sessionId":"ses-1",
		"messages":[
			{
				"id":"msg-i1","timestamp":"2026-01-01T00:00:00Z","type":"info",
				"content":"Session started"
			},
			{
				"id":"msg-u1","timestamp":"2026-01-01T00:00:01Z","type":"user",
				"content":"hello"
			}
		]
	}`)

	expected := []string{
		`{"v":1,"agent":"gemini-cli","cli_version":"0.5.1","type":"user","ts":"2026-01-01T00:00:01Z","content":[{"text":"hello"}]}`,
	}

	result, err := Compact(input, agentOpts("gemini-cli"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONLines(t, result, expected)
}

func TestIsGeminiFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{
			name: "valid gemini",
			in:   `{"sessionId":"s1","messages":[]}`,
			want: true,
		},
		{
			name: "opencode has info key",
			in:   `{"info":{"id":"s1"},"messages":[]}`,
			want: false,
		},
		{
			name: "JSONL not JSON object",
			in:   `{"type":"user","message":{}}` + "\n" + `{"type":"assistant","message":{}}`,
			want: false,
		},
		{
			name: "empty",
			in:   "",
			want: false,
		},
		{
			name: "missing messages key",
			in:   `{"sessionId":"s1"}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isGeminiFormat([]byte(tt.in))
			if got != tt.want {
				t.Errorf("isGeminiFormat(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
