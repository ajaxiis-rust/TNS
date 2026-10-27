-- Лексикон мира (T-07-05, CB-06.9): кандидаты и утверждённые слова мира.
-- Кандидат — слово, одинаково написанное трижды и более в обработанных главах
-- книги (book_text published=1, регистр нечувствителен); слово из лексикона не
-- правится Цензором правок.
CREATE TABLE world_lexicon (
    id INTEGER PRIMARY KEY,
    word TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('candidate','approved')),
    created_at TEXT NOT NULL
);
