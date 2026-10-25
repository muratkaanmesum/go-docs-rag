// Command ingest loads the corpus, chunks it, embeds every chunk with Ollama and
// replaces the contents of the chunks table.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"go-docs-rag/internal/ingest"
)

func main() {
	corpus := flag.String("corpus", "corpus", "directory filled by fetch-corpus.sh")
	maxWords := flag.Int("max-words", 600, "split sections longer than this")
	batch := flag.Int("batch", 64, "chunks per embedding request")
	dryRun := flag.Bool("dry-run", false, "load and chunk only; no embedding, no database")
	show := flag.String("show", "", "with -dry-run, print the chunks of pages whose URL contains this")
	flag.Parse()
	ctx := context.Background()

	docs := load(*corpus)
	var chunks []ingest.Chunk
	for _, d := range docs {
		cs, err := ingest.ChunkDocument(d, *maxWords)
		if err != nil {
			log.Fatalf("chunk %s: %v", d.URL, err)
		}
		chunks = append(chunks, cs...)
	}
	printSizes(chunks)

	if *dryRun {
		for _, c := range chunks {
			if *show != "" && strings.Contains(c.URL, *show) {
				fmt.Printf("\n--- %s\n%s\n", c.URL, c.Content)
			}
		}
		return
	}

	start := time.Now()
	e := ingest.NewEmbedder()
	tokens := 0
	for i := 0; i < len(chunks); i += *batch {
		b := chunks[i:min(i+*batch, len(chunks))]
		texts := make([]string, len(b))
		for j, c := range b {
			texts[j] = c.Content
		}
		vecs, n, err := e.Embed(ctx, texts)
		if err != nil {
			log.Fatalf("embed chunks %d-%d: %v", i, i+len(b)-1, err)
		}
		for j := range b {
			b[j].Embedding = vecs[j]
		}
		tokens += n
		fmt.Fprintf(os.Stderr, "\rembedded %d/%d", i+len(b), len(chunks))
	}
	fmt.Fprintln(os.Stderr)
	fmt.Printf("embedded %d tokens in %s (%.0f tokens/s)\n", tokens, time.Since(start).Round(time.Second), float64(tokens)/time.Since(start).Seconds())

	if err := store(ctx, chunks); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("stored %d chunks\n", len(chunks))
}

func load(corpus string) []*ingest.Document {
	var docs []*ingest.Document
	var redirects, untitled, fallbacks []string
	add := func(root, rel, docType string) {
		d, err := ingest.Load(root, rel, docType)
		switch {
		case errors.Is(err, ingest.ErrRedirect):
			redirects = append(redirects, rel)
		case errors.Is(err, ingest.ErrNoTitle):
			untitled = append(untitled, rel)
		case err != nil:
			log.Fatal(err)
		default:
			if d.TemplateFallback {
				fallbacks = append(fallbacks, rel)
			}
			docs = append(docs, d)
		}
	}

	content := filepath.Join(corpus, "website", "_content")
	for _, top := range []string{"doc", "ref", "blog"} {
		err := filepath.WalkDir(filepath.Join(content, top), func(p string, de fs.DirEntry, err error) error {
			if err != nil || de.IsDir() {
				return err
			}
			if ext := filepath.Ext(p); ext != ".md" && ext != ".html" {
				return nil
			}
			rel, err := filepath.Rel(content, p)
			if err != nil {
				return err
			}
			add(content, filepath.ToSlash(rel), top)
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
	}
	for _, f := range []string{"go_spec.html", "go_mem.html"} {
		add(filepath.Join(corpus, "go", "doc"), f, "ref")
	}

	fmt.Printf("pages: %d loaded, %d redirects skipped, %d untitled skipped %v\n", len(docs), len(redirects), len(untitled), untitled)
	fmt.Printf("template fallbacks (directives deleted): %d %v\n", len(fallbacks), fallbacks)
	return docs
}

func printSizes(chunks []ingest.Chunk) {
	words := make([]int, len(chunks))
	for i, c := range chunks {
		words[i] = len(strings.Fields(c.Content))
	}
	slices.Sort(words)
	pct := func(p float64) int { return words[int(p*float64(len(words)-1))] }
	fmt.Printf("chunks: %d; words p50 %d, p90 %d, p99 %d, max %d\n", len(chunks), pct(.5), pct(.9), pct(.99), words[len(words)-1])
}

func store(ctx context.Context, chunks []ingest.Chunk) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://rag@localhost:5432/rag"
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	if err := pgxvec.RegisterTypes(ctx, conn); err != nil {
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "TRUNCATE chunks RESTART IDENTITY"); err != nil {
		return err
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"chunks"},
		[]string{"url", "page_title", "heading_path", "doc_type", "published", "content", "embedding_model", "embedding"},
		pgx.CopyFromSlice(len(chunks), func(i int) ([]any, error) {
			c := chunks[i]
			return []any{c.URL, c.PageTitle, c.HeadingPath, c.DocType, c.Published, c.Content, ingest.EmbeddingModel, pgvector.NewVector(c.Embedding)}, nil
		}))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
