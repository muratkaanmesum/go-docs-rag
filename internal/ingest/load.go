package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

type Document struct {
	URL       string
	Title     string
	DocType   string
	Published *time.Time
	HTML      string
	// TemplateFallback is set when the page's template could not run and its directives were deleted.
	TemplateFallback bool
}

var (
	ErrRedirect = errors.New("redirect stub")
	ErrNoTitle  = errors.New("no title")
)

// Load reads one page. root is the directory URLs are relative to (website's _content),
// rel the file path inside it. Template directives are executed the way go.dev does:
// {{code}}/{{play}} read files from root, so the result carries the included code.
func Load(root, rel, docType string) (*Document, error) {
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	meta, body, err := parseMeta(b)
	if err != nil {
		return nil, fmt.Errorf("%s: front matter: %w", rel, err)
	}
	if _, ok := meta["redirect"]; ok {
		return nil, ErrRedirect
	}
	title, _ := meta["title"].(string)
	if title == "" {
		return nil, ErrNoTitle
	}

	isMarkdown := strings.HasSuffix(rel, ".md")
	fallback := false
	if t, _ := meta["template"].(bool); t {
		body, fallback = executeTemplate(body, meta, root, path.Dir(rel), isMarkdown)
	}
	if isMarkdown {
		var buf bytes.Buffer
		if err := markdown.Convert(body, &buf); err != nil {
			return nil, fmt.Errorf("%s: markdown: %w", rel, err)
		}
		body = buf.Bytes()
	}

	return &Document{
		URL:       pageURL(rel, meta),
		Title:     title,
		DocType:   docType,
		Published: date(meta["date"]),
		HTML:      string(body),

		TemplateFallback: fallback,
	}, nil
}

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAttribute(), parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
)

var (
	jsonStart = []byte("<!--{")
	jsonEnd   = []byte("}-->")
	yamlStart = []byte("---\n")
	yamlEnd   = []byte("\n---\n")
)

// parseMeta mirrors golang/website internal/web/page.go: JSON in <!--{...}--> (keys
// lower-cased) or YAML between --- lines.
func parseMeta(b []byte) (map[string]any, []byte, error) {
	meta := map[string]any{}
	switch {
	case bytes.HasPrefix(b, jsonStart):
		end := bytes.Index(b, jsonEnd)
		if end < 0 {
			return meta, b, nil
		}
		raw := map[string]any{}
		if err := json.Unmarshal(b[len(jsonStart)-1:end+1], &raw); err != nil {
			return nil, nil, err
		}
		for k, v := range raw {
			meta[strings.ToLower(k)] = v
		}
		return meta, b[end+len(jsonEnd):], nil
	case bytes.HasPrefix(b, yamlStart):
		end := bytes.Index(b, yamlEnd)
		if end < 0 {
			return meta, b, nil
		}
		if err := yaml.Unmarshal(b[len(yamlStart):end+1], &meta); err != nil {
			return nil, nil, err
		}
		return meta, b[end+len(yamlEnd):], nil
	}
	return meta, b, nil
}

// pageURL applies the go.dev rule: x/y/z.md -> /x/y/z, x/y/index.md -> /x/y/, unless
// the page sets its own path.
func pageURL(rel string, meta map[string]any) string {
	if p, ok := meta["path"].(string); ok && p != "" {
		return "https://go.dev" + p
	}
	p := strings.TrimSuffix(rel, path.Ext(rel))
	p = strings.TrimSuffix(p, "index")
	return "https://go.dev/" + p
}

func date(v any) *time.Time {
	switch d := v.(type) {
	case time.Time:
		return &d
	case string:
		if t, err := time.Parse("2006-01-02", d[:min(len(d), 10)]); err == nil {
			return &t
		}
	}
	return nil
}

var directive = regexp.MustCompile(`(?s)\{\{.*?\}\}`)

// executeTemplate runs the page as a Go template with stand-ins for go.dev's functions.
// Pages that call site-only functions (blog listings, release history) fail to parse;
// those fall back to deleting every directive.
func executeTemplate(body []byte, meta map[string]any, root, dir string, isMarkdown bool) ([]byte, bool) {
	read := func(file string) string {
		p := filepath.Join(root, dir, file)
		if strings.HasPrefix(file, "/") {
			p = filepath.Join(root, file)
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return ""
		}
		return string(src)
	}
	include := func(file string, _ ...any) string {
		src := read(file)
		if src == "" {
			return ""
		}
		var keep []string
		for line := range strings.SplitSeq(src, "\n") {
			if !strings.Contains(line, "OMIT") {
				keep = append(keep, line)
			}
		}
		code := strings.TrimSpace(strings.Join(keep, "\n"))
		if isMarkdown {
			return "\n```go\n" + code + "\n```\n"
		}
		return "<pre>" + html.EscapeString(code) + "</pre>"
	}
	funcs := template.FuncMap{
		"code":  include,
		"play":  include,
		"image": func(...any) string { return "" },
		"video": func(...any) string { return "" },
		"raw":   func(s string) string { return s },
		"by":    func(any) string { return "" },
		"file":  read,
		"now":   time.Now,
	}
	var out bytes.Buffer
	t, err := template.New("page").Funcs(funcs).Parse(string(body))
	if err == nil {
		err = t.Execute(&out, meta)
	}
	if err != nil {
		return directive.ReplaceAll(body, nil), true
	}
	return out.Bytes(), false
}
