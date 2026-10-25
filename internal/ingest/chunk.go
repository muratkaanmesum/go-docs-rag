package ingest

import (
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type Chunk struct {
	URL         string
	PageTitle   string
	HeadingPath string
	DocType     string
	Published   *time.Time
	Content     string // prefix + text: the string that gets embedded
	Embedding   []float32
}

type section struct {
	level   int // 1-3 for h1-h3; 0 for text before the first heading
	heading string
	anchor  string
	text    strings.Builder
}

// ChunkDocument cuts a page at h1-h3, prefixes every piece with "Page title > heading path",
// drops heading-only sections and splits sections longer than maxWords by paragraph.
func ChunkDocument(d *Document, maxWords int) ([]Chunk, error) {
	sections, err := splitSections(d.HTML)
	if err != nil {
		return nil, err
	}
	type heading struct {
		level int
		text  string
	}
	var stack []heading
	var out []Chunk
	for _, s := range sections {
		if s.level > 0 {
			for len(stack) > 0 && stack[len(stack)-1].level >= s.level {
				stack = stack[:len(stack)-1]
			}
			if !strings.EqualFold(s.heading, d.Title) {
				stack = append(stack, heading{s.level, s.heading})
			}
		}
		text := clean(s.text.String())
		if text == "" {
			continue
		}
		var path []string
		for _, h := range stack {
			path = append(path, h.text)
		}
		headingPath := strings.Join(path, " > ")
		prefix := d.Title
		if headingPath != "" {
			prefix += " > " + headingPath
		}
		url := d.URL
		if s.anchor != "" {
			url += "#" + s.anchor
		}
		for _, piece := range splitByParagraph(text, maxWords) {
			out = append(out, Chunk{
				URL:         url,
				PageTitle:   d.Title,
				HeadingPath: headingPath,
				DocType:     d.DocType,
				Published:   d.Published,
				Content:     prefix + "\n\n" + piece,
			})
		}
	}
	return out, nil
}

func splitSections(page string) ([]*section, error) {
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return nil, err
	}
	cur := &section{}
	sections := []*section{cur}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			cur.text.WriteString(collapseSpace(n.Data))
			return
		case html.ElementNode:
			switch n.Data {
			case "script", "style":
				return
			case "h1", "h2", "h3":
				cur = &section{
					level:   int(n.Data[1] - '0'),
					heading: strings.TrimSpace(collapseSpace(textOf(n))),
					anchor:  attr(n, "id"),
				}
				sections = append(sections, cur)
				return
			case "pre":
				cur.text.WriteString("\n\n```\n" + strings.TrimRight(textOf(n), "\n") + "\n```\n\n")
				return
			case "br":
				cur.text.WriteString("\n")
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && isBlock[n.Data] {
			cur.text.WriteString("\n\n")
		}
	}
	walk(root)
	return sections, nil
}

var isBlock = map[string]bool{
	"p": true, "div": true, "li": true, "ul": true, "ol": true, "table": true, "tr": true,
	"blockquote": true, "dl": true, "dt": true, "dd": true, "h4": true, "h5": true, "h6": true,
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

var (
	spaces     = regexp.MustCompile(`[ \t\r\n]+`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

func collapseSpace(s string) string { return spaces.ReplaceAllString(s, " ") }

func clean(s string) string {
	lines := strings.Split(s, "\n")
	inCode := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
		}
		if inCode && !strings.HasPrefix(t, "```") {
			lines[i] = strings.TrimRight(l, " \t") // keep code indentation
		} else {
			lines[i] = t
		}
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

func splitByParagraph(text string, maxWords int) []string {
	var pieces []string
	var cur []string
	n := 0
	flush := func() {
		if len(cur) > 0 {
			pieces = append(pieces, strings.Join(cur, "\n\n"))
			cur, n = nil, 0
		}
	}
	for p := range strings.SplitSeq(text, "\n\n") {
		w := len(strings.Fields(p))
		if n > 0 && n+w > maxWords {
			flush()
		}
		if w > maxWords {
			words := strings.Fields(p)
			for len(words) > maxWords {
				pieces = append(pieces, strings.Join(words[:maxWords], " "))
				words = words[maxWords:]
			}
			p, w = strings.Join(words, " "), len(words)
		}
		cur = append(cur, p)
		n += w
	}
	flush()
	return pieces
}
