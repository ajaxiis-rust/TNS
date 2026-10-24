CREATE TABLE vectors (
    id INTEGER PRIMARY KEY,
    owner_table TEXT NOT NULL,
    owner_id INTEGER NOT NULL,
    model TEXT NOT NULL,
    dim INTEGER NOT NULL CHECK (dim = 1024),
    lang TEXT NOT NULL,
    source_text TEXT NOT NULL,
    vec BLOB NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_vectors_owner ON vectors(owner_table, owner_id);
