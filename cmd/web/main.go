// Command web serves a local chat page for asking the go.dev docs questions.
package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"go-docs-rag/internal/answer"
	"go-docs-rag/internal/ingest"
	"go-docs-rag/internal/retrieve"
)

//go:embed static
var static embed.FS

type server struct {
	pool     *pgxpool.Pool
	embedder *ingest.Embedder
	model    string
	k        int
}

type chunk struct {
	N           int     `json:"n"`
	URL         string  `json:"url"`
	PageTitle   string  `json:"pageTitle"`
	HeadingPath string  `json:"headingPath"`
	Excerpt     string  `json:"excerpt"`
	Distance    float64 `json:"distance"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:7070", "listen address; keep it on localhost")
	model := flag.String("model", "claude-sonnet-5-5", "Claude model ID")
	k := flag.Int("k", 5, "chunks to retrieve")
	flag.Parse()
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://rag@localhost:5432/rag"
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatal(err)
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	s := &server{pool: pool, embedder: ingest.NewEmbedder(), model: *model, k: *k}
	files, _ := fs.Sub(static, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(files))
	mux.HandleFunc("POST /api/retrieve", s.handleRetrieve)
	mux.HandleFunc("POST /api/answer", s.handleAnswer)

	log.Printf("open http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func (s *server) retrieve(ctx context.Context, question string) ([]retrieve.Result, error) {
	vecs, _, err := s.embedder.Embed(ctx, []string{question})
	if err != nil {
		return nil, fmt.Errorf("embedding the question failed (is Ollama running?): %w", err)
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("database unreachable (is docker compose up?): %w", err)
	}
	defer conn.Release()
	return retrieve.Vector(ctx, conn.Conn(), vecs[0], s.k)
}

func (s *server) handleRetrieve(w http.ResponseWriter, r *http.Request) {
	q, ok := readQuestion(w, r)
	if !ok {
		return
	}
	results, err := s.retrieve(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	out := make([]chunk, len(results))
	for i, res := range results {
		out[i] = chunk{
			N:           i + 1,
			URL:         res.URL,
			PageTitle:   res.PageTitle,
			HeadingPath: res.HeadingPath,
			Excerpt:     excerpt(res.Content, 280),
			Distance:    res.Distance,
		}
	}
	writeJSON(w, map[string]any{"chunks": out})
}

func (s *server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	q, ok := readQuestion(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	results, err := s.retrieve(ctx, q)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	text, err := answer.Ask(ctx, s.model, q, results)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("Claude request failed (check ANTHROPIC_BASE_URL and ANTHROPIC_API_KEY): %w", err))
		return
	}
	writeJSON(w, map[string]any{
		"html":    linkCitations(renderMarkdown(text), results),
		"cited":   answer.Cited(text, len(results)),
		"refused": strings.Contains(text, answer.Refusal),
	})
}

// Raw HTML in the model's answer is escaped: goldmark drops it unless WithUnsafe is set.
var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

func renderMarkdown(text string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(text), &buf); err != nil {
		return "<p>" + strings.ReplaceAll(text, "<", "&lt;") + "</p>"
	}
	return buf.String()
}

var marker = regexp.MustCompile(`\[(\d+)\]`)

func linkCitations(html string, results []retrieve.Result) string {
	return marker.ReplaceAllStringFunc(html, func(m string) string {
		n, _ := strconv.Atoi(m[1 : len(m)-1])
		if n < 1 || n > len(results) {
			return m
		}
		return fmt.Sprintf(`<a class="cite" data-n="%d" href="%s" target="_blank" rel="noopener">%d</a>`, n, results[n-1].URL, n)
	})
}

func excerpt(content string, n int) string {
	if _, body, ok := strings.Cut(content, "\n\n"); ok {
		content = body // drop the "Page > heading" prefix; the UI shows it separately
	}
	content = strings.Join(strings.Fields(content), " ")
	if len(content) <= n {
		return content
	}
	return strings.TrimRightFunc(content[:n], func(r rune) bool { return r != ' ' }) + "…"
}

func readQuestion(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("send JSON like {\"question\": \"...\"}"))
		return "", false
	}
	q := strings.TrimSpace(body.Question)
	if q == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("type a question first"))
		return "", false
	}
	return q, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	log.Print(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
