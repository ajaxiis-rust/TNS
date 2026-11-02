-- Реестр книг в литературной базе мира (P11 §5.3, T-12-10). Сегодня книга
-- опознаётся только строкой origin; реестр даёт выпадающий список, привязку и
-- фильтр «свои записи» без разбора строк. Миграция аддитивная: book_id в
-- bank_pool ссылается на book, NULL — старые записи и сид, поэтому перенос
-- старых миров не нужен (SCR-§4).
CREATE TABLE book (
    id INTEGER PRIMARY KEY,
    shelf_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    lang TEXT NOT NULL,
    genres TEXT NOT NULL DEFAULT '',
    rating TEXT NOT NULL DEFAULT '',
    chapters INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL
);

ALTER TABLE bank_pool ADD COLUMN book_id INTEGER REFERENCES book(id);
