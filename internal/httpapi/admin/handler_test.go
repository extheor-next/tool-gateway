package admin

import (
	"testing"
)

func TestFieldStringNilToEmpty(t *testing.T) {
	if got := fieldString(map[string]any{"token": nil}, "token"); got != "" {
		t.Fatalf("expected empty string for nil field, got %q", got)
	}
	if got := fieldString(map[string]any{}, "token"); got != "" {
		t.Fatalf("expected empty string for missing field, got %q", got)
	}
}

func TestMaskSecretPreviewKeepsOnlyFirstAndLastTwoChars(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"a":        "*",
		"ab":       "**",
		"abcd":     "****",
		"abcdef":   "ab****ef",
		"abc12345": "ab****45",
	}

	for input, want := range cases {
		if got := maskSecretPreview(input); got != want {
			t.Fatalf("maskSecretPreview(%q)=%q want %q", input, got, want)
		}
	}
}
