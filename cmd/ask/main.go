// Command ask answers a question about Go from the indexed go.dev docs, with citations.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"go-docs-rag/internal/answer"
	"go-docs-rag/internal/ingest"
	"go-docs-rag/internal/retrieve"
)

func main() {
	k := flag.Int("k", 5, "chunks to retrieve")
	showChunks := flag.Bool("show-chunks", false, "print the retrieved chunks and their distances")
	retrieveOnly := flag.Bool("retrieve-only", false, "stop after retrieval; no LLM call")
	model := flag.String("model", "claude-sonnet-5-5", "Claude model ID")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: ask [flags] "question"`)
		flag.PrintDefaults()
	}
	flag.Parse()
	question := strings.Join(flag.Args(), " ")
	if question == "" {
		flag.Usage()
		os.Exit(2)
	}
	ctx := context.Background()

	vecs, _, err := ingest.NewEmbedder().Embed(ctx, []string{question})
	if err != nil {
		log.Fatal(err)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://rag@localhost:5432/rag"
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)
	if err := pgxvec.RegisterTypes(ctx, conn); err != nil {
		log.Fatal(err)
	}
	results, err := retrieve.Vector(ctx, conn, vecs[0], *k)
	if err != nil {
		log.Fatal(err)
	}

	if *showChunks || *retrieveOnly {
		for i, r := range results {
			fmt.Printf("[%d] %.3f  %s\n", i+1, r.Distance, r.URL)
			if *showChunks {
				fmt.Printf("    %s\n", strings.ReplaceAll(truncate(r.Content, 300), "\n", "\n    "))
			}
		}
		fmt.Println()
	}
	if *retrieveOnly {
		return
	}

	text, err := answer.Ask(ctx, *model, question, results)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(text)
	if sources := answer.Sources(text, results); len(sources) > 0 {
		fmt.Println("\nSources:")
		for _, s := range sources {
			fmt.Println(s)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
