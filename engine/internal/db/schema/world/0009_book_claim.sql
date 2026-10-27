-- Заявка книги (T-07-09, CB-10): текст заявки (1–3 предложения) и вектор
-- смысла (vector_id → vectors.id). Предложение заявки отдаёт модель, запись в
-- book_claim/vectors делает core; заявку можно пропустить — книга без заявки
-- функционирует.
CREATE TABLE book_claim (
    id INTEGER PRIMARY KEY,
    text TEXT NOT NULL,
    vector_id INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('proposed','approved')),
    created_at TEXT NOT NULL
);
