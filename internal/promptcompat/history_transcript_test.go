package promptcompat

import (
	"strings"
	"testing"
)

func TestBuildOpenAICurrentInputContextTranscriptCompactsOldLargeEntries(t *testing.T) {
	largeOld := strings.Repeat("old-log-line\n", 2000)
	messages := []any{
		map[string]any{"role": "tool", "name": "Bash", "content": largeOld},
		map[string]any{"role": "user", "content": "important older requirement"},
	}
	for i := 0; i < 6; i++ {
		messages = append(messages, map[string]any{"role": "user", "content": "recent turn marker"})
	}

	transcript := BuildOpenAICurrentInputContextTranscript(messages)

	if len(transcript) > 12000 {
		t.Fatalf("expected compact history transcript, got %d bytes", len(transcript))
	}
	if strings.Count(transcript, "old-log-line") > 220 {
		t.Fatalf("expected old large entry to be truncated, got transcript length %d", len(transcript))
	}
	if !strings.Contains(transcript, "[older large entry truncated") {
		t.Fatalf("expected truncation marker, got %q", transcript)
	}
	if !strings.Contains(transcript, "important older requirement") {
		t.Fatalf("expected short older context preserved, got %q", transcript)
	}
	if strings.Count(transcript, "recent turn marker") != 6 {
		t.Fatalf("expected recent turns preserved, got %q", transcript)
	}
}
