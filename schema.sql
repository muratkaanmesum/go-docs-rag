CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS chunks (
    id              SERIAL PRIMARY KEY,
    url             TEXT NOT NULL,
    page_title      TEXT NOT NULL,
    heading_path    TEXT NOT NULL,
    doc_type        TEXT NOT NULL,
    published       DATE,
    content         TEXT NOT NULL,
    embedding_model TEXT NOT NULL,
    embedding       vector(1024) NOT NULL,
    tsv             tsvector GENERATED ALWAYS AS (to_tsvector('english', content)) STORED
);

CREATE INDEX IF NOT EXISTS chunks_embedding_idx ON chunks USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS chunks_tsv_idx ON chunks USING gin (tsv);
