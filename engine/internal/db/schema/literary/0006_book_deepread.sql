-- Таблицы разбора книги (P13 §3, T-14-03). Пять таблиц волны: тег категории
-- на чанк с полосой и якорями, места, сцены, стиль книги и staging кандидатов
-- словарей. Миграция аддитивная: миры до 0006 этих таблиц не имеют, перенос
-- записей не нужен (SCR-§4); DDL перенесён из плана дословно.
CREATE TABLE book_chunk_tag (
    chunk_id INTEGER NOT NULL REFERENCES book_chunk(id),
    key TEXT NOT NULL,
    score REAL NOT NULL,
    band TEXT NOT NULL CHECK (band IN ('confident','reject','uncertain')),
    anchors TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (chunk_id, key)
);

CREATE TABLE book_place (
    id INTEGER PRIMARY KEY,
    book_id INTEGER NOT NULL REFERENCES book(id),
    name TEXT NOT NULL,
    aliases TEXT NOT NULL DEFAULT '[]',
    kind TEXT NOT NULL DEFAULT '',
    first_chunk INTEGER NOT NULL DEFAULT 0,
    last_chunk INTEGER NOT NULL DEFAULT 0,
    mentions INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE book_scene (
    id INTEGER PRIMARY KEY,
    book_id INTEGER NOT NULL REFERENCES book(id),
    ord INTEGER NOT NULL,
    from_chunk INTEGER NOT NULL,
    to_chunk INTEGER NOT NULL,
    place_id INTEGER REFERENCES book_place(id),
    time_marker TEXT NOT NULL DEFAULT '',
    participants TEXT NOT NULL DEFAULT '[]',
    tags TEXT NOT NULL DEFAULT '[]',
    emotion TEXT NOT NULL DEFAULT '{}',
    UNIQUE (book_id, ord)
);

CREATE TABLE book_style (
    book_id INTEGER PRIMARY KEY REFERENCES book(id),
    author TEXT NOT NULL DEFAULT '',
    features TEXT NOT NULL DEFAULT '{}',
    labels TEXT NOT NULL DEFAULT '[]',
    drama_profile TEXT NOT NULL DEFAULT '[]',
    computed_at TEXT NOT NULL
);

CREATE TABLE dict_staging (
    id INTEGER PRIMARY KEY,
    dict_key TEXT NOT NULL,
    lemma TEXT NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1,
    evidence TEXT NOT NULL DEFAULT '',
    book_id INTEGER REFERENCES book(id),
    status TEXT NOT NULL DEFAULT 'candidate'
        CHECK (status IN ('candidate','promoted','rejected')),
    repeats INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    promoted_at TEXT,
    UNIQUE (dict_key, lemma)
);
