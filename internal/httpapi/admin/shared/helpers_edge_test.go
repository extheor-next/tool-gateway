package shared

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── intFromQuery edge cases ─────────────────────────────────────────

func TestIntFromQueryPresent(t *testing.T) {
	req := httptest.NewRequest("GET", "/?limit=5", nil)
	if got := intFromQuery(req, "limit", 10); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestIntFromQueryMissing(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if got := intFromQuery(req, "limit", 10); got != 10 {
		t.Fatalf("expected default 10, got %d", got)
	}
}

func TestIntFromQueryInvalid(t *testing.T) {
	req := httptest.NewRequest("GET", "/?limit=abc", nil)
	if got := intFromQuery(req, "limit", 10); got != 10 {
		t.Fatalf("expected default 10 for invalid, got %d", got)
	}
}

func TestIntFromQueryNegative(t *testing.T) {
	req := httptest.NewRequest("GET", "/?limit=-3", nil)
	if got := intFromQuery(req, "limit", 10); got != -3 {
		t.Fatalf("expected -3, got %d", got)
	}
}

func TestIntFromQueryZero(t *testing.T) {
	req := httptest.NewRequest("GET", "/?limit=0", nil)
	if got := intFromQuery(req, "limit", 10); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

// ─── nilIfEmpty ──────────────────────────────────────────────────────

func TestNilIfEmptyEmpty(t *testing.T) {
	if nilIfEmpty("") != nil {
		t.Fatal("expected nil for empty string")
	}
}

func TestNilIfEmptyNonEmpty(t *testing.T) {
	if nilIfEmpty("hello") != "hello" {
		t.Fatal("expected 'hello'")
	}
}

// ─── nilIfZero ───────────────────────────────────────────────────────

func TestNilIfZeroZero(t *testing.T) {
	if nilIfZero(0) != nil {
		t.Fatal("expected nil for zero")
	}
}

func TestNilIfZeroNonZero(t *testing.T) {
	if nilIfZero(42) != int64(42) {
		t.Fatal("expected 42")
	}
}

func TestNilIfZeroNegative(t *testing.T) {
	if nilIfZero(-1) != int64(-1) {
		t.Fatal("expected -1")
	}
}

// ─── toStringSlice ───────────────────────────────────────────────────

func TestToStringSliceFromAnySlice(t *testing.T) {
	input := []any{"a", "b", "c"}
	got, ok := toStringSlice(input)
	if !ok || len(got) != 3 {
		t.Fatalf("expected 3 strings, got %#v ok=%v", got, ok)
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected values: %#v", got)
	}
}

func TestToStringSliceFromMixed(t *testing.T) {
	input := []any{"hello", 42, true}
	got, ok := toStringSlice(input)
	if !ok {
		t.Fatal("expected ok for mixed types")
	}
	if got[0] != "hello" || got[1] != "42" || got[2] != "true" {
		t.Fatalf("unexpected values: %#v", got)
	}
}

func TestToStringSliceFromNonSlice(t *testing.T) {
	_, ok := toStringSlice("not a slice")
	if ok {
		t.Fatal("expected not ok for string input")
	}
}

func TestToStringSliceFromNil(t *testing.T) {
	_, ok := toStringSlice(nil)
	if ok {
		t.Fatal("expected not ok for nil input")
	}
}

func TestToStringSliceEmpty(t *testing.T) {
	got, ok := toStringSlice([]any{})
	if !ok {
		t.Fatal("expected ok for empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %#v", got)
	}
}

func TestToStringSliceTrimsWhitespace(t *testing.T) {
	got, ok := toStringSlice([]any{" hello ", " world "})
	if !ok {
		t.Fatal("expected ok")
	}
	if got[0] != "hello" || got[1] != "world" {
		t.Fatalf("expected trimmed values, got %#v", got)
	}
}

// ─── fieldString edge cases ──────────────────────────────────────────

func TestFieldStringNonString(t *testing.T) {
	got := fieldString(map[string]any{"key": 42}, "key")
	if got != "42" {
		t.Fatalf("expected '42' for int, got %q", got)
	}
}

func TestFieldStringBool(t *testing.T) {
	got := fieldString(map[string]any{"key": true}, "key")
	if got != "true" {
		t.Fatalf("expected 'true', got %q", got)
	}
}

func TestFieldStringWhitespace(t *testing.T) {
	got := fieldString(map[string]any{"key": "  hello  "}, "key")
	if got != "hello" {
		t.Fatalf("expected trimmed 'hello', got %q", got)
	}
}

// ─── statusOr ────────────────────────────────────────────────────────

func TestStatusOrZeroReturnsDefault(t *testing.T) {
	if got := statusOr(0, http.StatusOK); got != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, got)
	}
}

func TestStatusOrNonZeroReturnsValue(t *testing.T) {
	if got := statusOr(http.StatusBadRequest, http.StatusOK); got != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, got)
	}
}
