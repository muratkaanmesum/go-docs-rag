package ingest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLoad(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"blog/go1.21.md":         "---\ntitle: Go 1.21 is released!\ndate: 2023-08-08\n---\n\nToday the Go team...\n",
		"doc/effective_go.html":  "<!--{\n\t\"Title\": \"Effective Go\",\n\t\"Template\": true\n}-->\n<h3 id=\"channels\">Channels</h3><p>Use {{\"<\"}} carefully.</p>\n",
		"blog/json-and-go.md":    "---\nredirect: /blog/json\n---\n",
		"doc/tutorial/index.md":  "---\ntitle: Tutorials\n---\n\nStart here.\n",
		"ref/spec.html":          "<!--{\n\t\"Title\": \"The Go Programming Language Specification\",\n\t\"Path\": \"/ref/spec\"\n}-->\n<p>x</p>\n",
		"blog/constants.md":      "---\ntitle: Constants\ntemplate: true\n---\n\nA big one:\n\n{{code \"constants/huge.go\" `/Huge/`}}\n\n{{image \"constants/x.png\"}}\n",
		"blog/constants/huge.go": "package main\n\n// START OMIT\nconst Huge = 1e1000\n// STOP OMIT\n",
		"doc/templates.md":       "---\ntitle: Templates\n---\n\nWrite `{{.Name}}` in your template.\n",
	})

	t.Run("yaml front matter and date", func(t *testing.T) {
		d := mustLoad(t, root, "blog/go1.21.md", "blog")
		if d.Title != "Go 1.21 is released!" || d.URL != "https://go.dev/blog/go1.21" {
			t.Errorf("got %q %q", d.Title, d.URL)
		}
		if d.Published == nil || d.Published.Format("2006-01-02") != "2023-08-08" {
			t.Errorf("published = %v", d.Published)
		}
	})
	t.Run("json front matter, lower-cased keys, template executed", func(t *testing.T) {
		d := mustLoad(t, root, "doc/effective_go.html", "doc")
		if d.Title != "Effective Go" || !strings.Contains(d.HTML, "Use < carefully") {
			t.Errorf("got %q, html %q", d.Title, d.HTML)
		}
		if d.Published != nil {
			t.Errorf("undated page got %v", d.Published)
		}
	})
	t.Run("redirect stub skipped", func(t *testing.T) {
		if _, err := Load(root, "blog/json-and-go.md", "blog"); !errors.Is(err, ErrRedirect) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("index.md maps to directory URL", func(t *testing.T) {
		if d := mustLoad(t, root, "doc/tutorial/index.md", "doc"); d.URL != "https://go.dev/doc/tutorial/" {
			t.Errorf("url = %q", d.URL)
		}
	})
	t.Run("path key overrides file name", func(t *testing.T) {
		if d := mustLoad(t, root, "ref/spec.html", "ref"); d.URL != "https://go.dev/ref/spec" {
			t.Errorf("url = %q", d.URL)
		}
	})
	t.Run("code include inlined without OMIT lines, image dropped", func(t *testing.T) {
		d := mustLoad(t, root, "blog/constants.md", "blog")
		if !strings.Contains(d.HTML, "const Huge = 1e1000") || strings.Contains(d.HTML, "OMIT") || strings.Contains(d.HTML, "{{") {
			t.Errorf("html = %q", d.HTML)
		}
	})
	t.Run("braces in a non-template page are content", func(t *testing.T) {
		if d := mustLoad(t, root, "doc/templates.md", "doc"); !strings.Contains(d.HTML, "{{.Name}}") {
			t.Errorf("html = %q", d.HTML)
		}
	})
}

func mustLoad(t *testing.T, root, rel, docType string) *Document {
	t.Helper()
	d, err := Load(root, rel, docType)
	if err != nil {
		t.Fatalf("Load(%s): %v", rel, err)
	}
	return d
}
