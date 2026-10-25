package ingest

import (
	"strings"
	"testing"
)

// A cut-down doc/pgo.md.
const pgo = `---
title: Profile-guided optimization
---

Starting in Go 1.20, the Go compiler supports profile-guided optimization (PGO).

# Building with PGO {#building}

By default, go build will detect default.pgo files automatically and enable PGO.

# Notes {#notes}

## Merging profiles {#merging-profiles}

The pprof tool can merge multiple profiles.

# Frequently Asked Questions {#faq}

## How does PGO affect binary size?

PGO can result in slightly larger binaries due to additional function inlining.
`

func TestChunkDocument(t *testing.T) {
	root := writeFiles(t, map[string]string{"doc/pgo.md": pgo})
	d := mustLoad(t, root, "doc/pgo.md", "doc")
	chunks, err := ChunkDocument(d, 600)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Chunk{}
	for _, c := range chunks {
		got[c.HeadingPath] = c
	}

	if len(chunks) != 4 {
		t.Errorf("got %d chunks, want 4 (intro, building, merging, binary size): %v", len(chunks), keys(got))
	}
	if _, ok := got["Notes"]; ok {
		t.Error("heading-only section # Notes was kept")
	}
	m, ok := got["Notes > Merging profiles"]
	if !ok {
		t.Fatalf("no chunk under Notes > Merging profiles: %v", keys(got))
	}
	if m.URL != "https://go.dev/doc/pgo#merging-profiles" {
		t.Errorf("url = %q", m.URL)
	}
	if !strings.HasPrefix(m.Content, "Profile-guided optimization > Notes > Merging profiles\n\n") {
		t.Errorf("content = %q", m.Content)
	}
	faq, ok := got["Frequently Asked Questions > How does PGO affect binary size?"]
	if !ok || !strings.Contains(faq.Content, "slightly larger binaries") {
		t.Errorf("12-word FAQ answer missing: %v", keys(got))
	}
	if intro := got[""]; !strings.HasPrefix(intro.Content, "Profile-guided optimization\n\nStarting in Go 1.20") {
		t.Errorf("intro = %q", intro.Content)
	}
}

func TestSplitByParagraph(t *testing.T) {
	p := strings.Repeat("word ", 400)
	pieces := splitByParagraph(p+"\n\n"+p+"\n\n"+strings.Repeat("x ", 1300), 600)
	for i, piece := range pieces {
		if n := len(strings.Fields(piece)); n > 600 {
			t.Errorf("piece %d has %d words", i, n)
		}
	}
	if len(pieces) != 5 {
		t.Errorf("got %d pieces, want 5 (400, 400, 600, 600, 100)", len(pieces))
	}
}

func keys(m map[string]Chunk) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
