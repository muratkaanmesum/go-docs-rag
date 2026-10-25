package retrieve

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type Result struct {
	URL         string
	PageTitle   string
	HeadingPath string
	Content     string
	Distance    float64 // cosine distance, smaller is closer
}

// Vector returns the k chunks nearest to the question vector. It always returns k rows
// when the table has them, however far away they are.
func Vector(ctx context.Context, conn *pgx.Conn, qvec []float32, k int) ([]Result, error) {
	rows, err := conn.Query(ctx,
		`SELECT url, page_title, heading_path, content, embedding <=> $1
		 FROM chunks ORDER BY embedding <=> $1 LIMIT $2`,
		pgvector.NewVector(qvec), k)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Result, error) {
		var res Result
		err := r.Scan(&res.URL, &res.PageTitle, &res.HeadingPath, &res.Content, &res.Distance)
		return res, err
	})
}
