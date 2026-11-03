-- Карта книги (P12 §5.9, T-12-21). Четыре таблицы извлечения действующих лиц:
-- куски текста без дублей, герои с метаданными, карта упоминаний по кускам и
-- роли по отрезкам. Текст кусков не дублируется — book_chunk хранит номер,
-- главу и хеш; извлечение заново разбирает файл полки (B-03.7), в базе только
-- выкованное. Миграция аддитивная: старые миры до 0005 не имеют этих таблиц,
-- перенос записей не нужен (SCR-§4).
CREATE TABLE book_chunk (
    id INTEGER PRIMARY KEY,
    book_id INTEGER NOT NULL REFERENCES book(id),
    ord INTEGER NOT NULL,
    chapter TEXT NOT NULL DEFAULT '',
    text_hash TEXT NOT NULL,
    UNIQUE (book_id, ord)
);

CREATE TABLE book_character (
    id INTEGER PRIMARY KEY,
    book_id INTEGER NOT NULL REFERENCES book(id),
    name TEXT NOT NULL,
    aliases TEXT NOT NULL DEFAULT '[]',
    appearance TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    traits TEXT NOT NULL DEFAULT '[]',
    confidence REAL NOT NULL DEFAULT 0,
    first_chunk INTEGER NOT NULL DEFAULT 0,
    last_chunk INTEGER NOT NULL DEFAULT 0,
    mentions INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'candidate',
    created_at TEXT NOT NULL
);

CREATE TABLE book_character_mention (
    character_id INTEGER NOT NULL REFERENCES book_character(id),
    chunk_id INTEGER NOT NULL REFERENCES book_chunk(id),
    count INTEGER NOT NULL,
    PRIMARY KEY (character_id, chunk_id)
);

CREATE TABLE book_character_role (
    id INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES book_character(id),
    from_chunk INTEGER NOT NULL,
    to_chunk INTEGER NOT NULL,
    role TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0,
    evidence TEXT NOT NULL DEFAULT ''
);
