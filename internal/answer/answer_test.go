package answer

import (
	"slices"
	"testing"

	"go-docs-rag/internal/retrieve"
)

func TestSources(t *testing.T) {
	excerpts := []retrieve.Result{{URL: "https://go.dev/doc/go1.21#slog"}, {URL: "https://go.dev/blog/slog"}}
	got := Sources("Added in Go 1.21 [2][1]. See also [1] and [7].", excerpts)
	want := []string{"[2] https://go.dev/blog/slog", "[1] https://go.dev/doc/go1.21#slog"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := Sources(Refusal, excerpts); len(got) != 0 {
		t.Errorf("refusal cited %q", got)
	}
}
