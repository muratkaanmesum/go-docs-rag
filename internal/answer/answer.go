package answer

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"go-docs-rag/internal/retrieve"
)

const Refusal = "I couldn't find this in the Go docs."

const instructions = `You answer questions about Go using only the documentation excerpts below.

Rules:
- Cite sources inline as [1], [2] matching the excerpt numbers.
- If the excerpts don't contain the answer, say "` + Refusal + `"
- Don't use outside knowledge.`

func Prompt(question string, excerpts []retrieve.Result) string {
	var b strings.Builder
	b.WriteString(instructions)
	b.WriteString("\n\nExcerpts:\n")
	for i, e := range excerpts {
		// Content already starts with "Page title > heading path".
		fmt.Fprintf(&b, "[%d] %s\n%s\n\n", i+1, e.URL, e.Content)
	}
	b.WriteString("Question: " + question)
	return b.String()
}

// Ask sends the prompt to Claude. The client reads ANTHROPIC_BASE_URL and
// ANTHROPIC_API_KEY from the environment, so requests go through the proxy.
func Ask(ctx context.Context, model, question string, excerpts []retrieve.Result) (string, error) {
	client := anthropic.NewClient()
	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(Prompt(question, excerpts))),
		},
	})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String(), nil
}

var marker = regexp.MustCompile(`\[(\d+)\]`)

// Sources lists the URLs of the excerpts the answer cites, in first-cited order.
// Markers that point at no excerpt are dropped, so every printed link is real.
func Sources(answer string, excerpts []retrieve.Result) []string {
	var out []string
	seen := map[int]bool{}
	for _, m := range marker.FindAllStringSubmatch(answer, -1) {
		n, _ := strconv.Atoi(m[1])
		if n < 1 || n > len(excerpts) || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, fmt.Sprintf("[%d] %s", n, excerpts[n-1].URL))
	}
	return out
}
