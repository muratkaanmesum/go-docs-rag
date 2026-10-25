package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const EmbeddingModel = "bge-m3"

type Embedder struct {
	URL string // Ollama base URL
}

func NewEmbedder() *Embedder {
	url := os.Getenv("OLLAMA_URL")
	if url == "" {
		url = "http://localhost:11434"
	}
	return &Embedder{URL: url}
}

// Embed returns one vector per input, in input order, and the number of tokens processed.
// truncate is off so an input over the model's context fails instead of being cut silently.
func (e *Embedder) Embed(ctx context.Context, inputs []string) ([][]float32, int, error) {
	body, err := json.Marshal(map[string]any{"model": EmbeddingModel, "input": inputs, "truncate": false})
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.URL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, 0, fmt.Errorf("ollama embed: %s: %s", resp.Status, msg)
	}
	var out struct {
		Embeddings      [][]float32 `json:"embeddings"`
		PromptEvalCount int         `json:"prompt_eval_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, err
	}
	if len(out.Embeddings) != len(inputs) {
		return nil, 0, fmt.Errorf("ollama embed: got %d vectors for %d inputs", len(out.Embeddings), len(inputs))
	}
	return out.Embeddings, out.PromptEvalCount, nil
}
