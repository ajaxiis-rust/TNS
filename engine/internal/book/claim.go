package book

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"tns65/internal/db"
	"tns65/internal/models"
)

// Заявка книги (T-07-09, CB-10): заявка — текст (1–3 предложения) плюс вектор
// смысла; предложение заявки отдаёт модель, запись в book_claim/vectors делает
// core. Заявку можно пропустить — книга без заявки функционирует.

// ClaimStatus — статус заявки (CHECK в 0009_book_claim.sql).
type ClaimStatus string

const (
	// ClaimProposed — предложение модели, ждёт решения игрока (утвердить или
	// поправить).
	ClaimProposed ClaimStatus = "proposed"
	// ClaimApproved — заявка игрока (записана или утверждена).
	ClaimApproved ClaimStatus = "approved"
)

// Claim — заявка книги: текст плюс вектор смысла.
type Claim struct {
	ID        int64
	Text      string
	VectorID  int64
	Status    ClaimStatus
	CreatedAt string
}

// Пороги и числа заявки (шаги 1, 3, 4).
const (
	ClaimMinSentences    = 1       // минимум предложений в тексте заявки.
	ClaimMaxSentences    = 3       // максимум предложений в тексте заявки.
	ClaimMinSessions     = 3       // порог «после нескольких сессий» для предложения.
	ClaimNightForgeBonus = 30      // +30 к приоритету заказов тем заявки ночной ковки.
	ClaimProposalRole    = "claim" // роль слота предложения заявки.

	claimVectorTable = "book_claim" // owner_table вектора заявки в vectors.
	claimVectorModel = "bge-m3"     // эмбеддинг-модель заявки (та же, что и главы).
	claimVectorLang  = "ru"
)

var (
	// ErrClaimSentences — текст заявки не 1–3 предложения.
	ErrClaimSentences = errors.New("book: заявка — не 1–3 предложения")
	// ErrClaimTooFewSessions — предложение заявки до трёх сессий.
	ErrClaimTooFewSessions = errors.New("book: заявка предлагается только после трёх сессий")
	// ErrNoClaim — действующая заявка не задана.
	ErrNoClaim = errors.New("book: заявка не задана")
	// ErrClaimNotFound — заявка по id не найдена.
	ErrClaimNotFound = errors.New("book: заявка не найдена")
	// ErrNoChapterVectors — векторов глав нет, дрейф не вычислим.
	ErrNoChapterVectors = errors.New("book: нет векторов глав для дрейфа")
)

// ValidateClaim проверяет, что заявка — 1–3 предложения по сплиттеру `. ! ? …`
// (тот же сплиттер, что у дайджеста T-07-06).
func ValidateClaim(text string) error {
	n := CountDigestSentences(text)
	if n < ClaimMinSentences || n > ClaimMaxSentences {
		return fmt.Errorf("%w: %d предложений, нужно %d–%d", ErrClaimSentences, n, ClaimMinSentences, ClaimMaxSentences)
	}
	return nil
}

// ClaimSlot — слот предложения заявки; счётчик вызовов инжектируется тестом.
type ClaimSlot interface {
	Generate(ctx context.Context, role, prompt string) (string, error)
}

// SaveClaim записывает заявку (текст + вектор смысла) писателем core в
// book_claim и vectors. Текст заявки пишет игрок (авторство с первой строки);
// предложение заявки отдаёт модель, в базу напрямую не пишет.
func SaveClaim(ctx context.Context, h *db.Handle, embedder models.Embedder, text string, status ClaimStatus) (Claim, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if h == nil {
		return Claim{}, errors.New("book: нет хэндла мира")
	}
	if embedder == nil {
		return Claim{}, errors.New("book: эмбеддер не подключён")
	}
	if err := ValidateClaim(text); err != nil {
		return Claim{}, err
	}
	if status != ClaimProposed && status != ClaimApproved {
		return Claim{}, errors.New("book: статус заявки не задан")
	}

	vec, err := embedder.Embed(ctx, text)
	if err != nil {
		return Claim{}, err
	}
	blob, err := chapterVectorBlob(vec)
	if err != nil {
		return Claim{}, err
	}
	created := nowUTC()

	var claim Claim
	err = h.Tx(db.WriterCore, func(tx *db.Tx) error {
		// Заявка сперва без вектора (vector_id=0), чтобы получить её id для
		// owner_id вектора; вектор дошивается следующим запросом.
		res, err := tx.Exec("book_claim",
			`INSERT INTO book_claim (text, vector_id, status, created_at) VALUES (?, 0, ?, ?)`,
			text, string(status), created)
		if err != nil {
			return err
		}
		claimID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		res, err = tx.Exec("vectors",
			`INSERT INTO vectors (owner_table, owner_id, model, dim, lang, source_text, vec, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			claimVectorTable, claimID, claimVectorModel, models.EmbedDim, claimVectorLang, text, blob, created)
		if err != nil {
			return err
		}
		vectorID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		if _, err := tx.Exec("book_claim",
			`UPDATE book_claim SET vector_id = ? WHERE id = ?`, vectorID, claimID); err != nil {
			return err
		}

		claim = Claim{ID: claimID, Text: text, VectorID: vectorID, Status: status, CreatedAt: created}
		return nil
	})
	if err != nil {
		return Claim{}, err
	}
	return claim, nil
}

// ProposeClaim формирует предложение заявки по накопленным паттернам игрока
// (карта Пифии — player_atlas) с опорой на синопсис анкеты и сохраняет его со
// статусом proposed. Предложение формируется только после трёх и более сессий.
func ProposeClaim(ctx context.Context, h *db.Handle, slot ClaimSlot, embedder models.Embedder) (Claim, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if h == nil {
		return Claim{}, errors.New("book: нет хэндла мира")
	}
	if slot == nil {
		return Claim{}, errors.New("book: слот предложения заявки не подключён")
	}
	if embedder == nil {
		return Claim{}, errors.New("book: эмбеддер не подключён")
	}

	var sessions int
	if err := h.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		return Claim{}, err
	}
	if sessions < ClaimMinSessions {
		return Claim{}, ErrClaimTooFewSessions
	}

	patterns, err := playerPatterns(h)
	if err != nil {
		return Claim{}, err
	}
	synopsis, err := claimSynopsis(h)
	if err != nil {
		return Claim{}, err
	}

	text, err := slot.Generate(ctx, ClaimProposalRole, claimPrompt(synopsis, patterns))
	if err != nil {
		return Claim{}, err
	}
	return SaveClaim(ctx, h, embedder, text, ClaimProposed)
}

// ApproveClaim утверждает предложение заявки (статус proposed → approved).
// Игрок правит текст, записывая новую заявку через SaveClaim со статусом approved.
func ApproveClaim(h *db.Handle, claimID int64) error {
	if h == nil {
		return errors.New("book: нет хэндла мира")
	}
	if claimID <= 0 {
		return errors.New("book: id заявки не задан")
	}
	return h.Tx(db.WriterCore, func(tx *db.Tx) error {
		res, err := tx.Exec("book_claim",
			`UPDATE book_claim SET status = ? WHERE id = ? AND status = ?`,
			string(ClaimApproved), claimID, string(ClaimProposed))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrClaimNotFound
		}
		return nil
	})
}

// CurrentClaim возвращает действующую (утверждённую) заявку книги; без неё —
// ErrNoClaim. Книга без заявки функционирует.
func CurrentClaim(h *db.Handle) (Claim, error) {
	if h == nil {
		return Claim{}, errors.New("book: нет хэндла мира")
	}
	var claim Claim
	var status string
	err := h.QueryRow(
		`SELECT id, text, vector_id, status, created_at FROM book_claim
		 WHERE status = ? ORDER BY id DESC LIMIT 1`, string(ClaimApproved)).
		Scan(&claim.ID, &claim.Text, &claim.VectorID, &status, &claim.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Claim{}, ErrNoClaim
	}
	if err != nil {
		return Claim{}, err
	}
	claim.Status = ClaimStatus(status)
	return claim, nil
}

// NightForgePriority — приоритет заказа темы заявки при ночной ковке: +30 к
// базовому приоритету (шаг 4).
func NightForgePriority(base int) int {
	return base + ClaimNightForgeBonus
}

// ClaimDrift — зазор «заявка ↔ накопленная книга» (шаг 4):
// drift = 1 − cos(вектор заявки, центроид векторов глав). Число для полосы
// дайджеста; без векторов глав дрейф не вычислим.
func ClaimDrift(h *db.Handle, claimID int64) (float64, error) {
	if h == nil {
		return 0, errors.New("book: нет хэндла мира")
	}
	if claimID <= 0 {
		return 0, errors.New("book: id заявки не задан")
	}

	claimVec, err := claimVector(h, claimID)
	if err != nil {
		return 0, err
	}
	chapterVecs, err := chapterVectors(h)
	if err != nil {
		return 0, err
	}
	return claimDrift(claimVec, chapterVecs)
}

// claimVector читает вектор заявки из vectors (owner_table='book_claim').
func claimVector(h *db.Handle, claimID int64) ([]float32, error) {
	var blob []byte
	err := h.QueryRow(
		`SELECT vec FROM vectors WHERE owner_table = ? AND owner_id = ?`,
		claimVectorTable, claimID).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrClaimNotFound
	}
	if err != nil {
		return nil, err
	}
	return decodeVectorBlob(blob)
}

// chapterVectors читает векторы опубликованных глав (owner_table='book_chapters').
func chapterVectors(h *db.Handle) ([][]float32, error) {
	rows, err := h.Query(`SELECT vec FROM vectors WHERE owner_table = ? ORDER BY owner_id`, chapterVectorTable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out [][]float32
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		vec, err := decodeVectorBlob(blob)
		if err != nil {
			return nil, err
		}
		out = append(out, vec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// decodeVectorBlob раскодирует BLOB Float32 little-endian в вектор (D-17.2).
func decodeVectorBlob(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, fmt.Errorf("book: длина BLOB вектора %d не кратна 4", len(blob))
	}
	vec := make([]float32, len(blob)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec, nil
}

// claimDrift — 1 − cos(вектор заявки, центроид векторов глав). Векторы заявки и
// глав нормированы; центроид нормируется перед косинусом.
func claimDrift(claim []float32, chapters [][]float32) (float64, error) {
	if len(claim) == 0 {
		return 0, errors.New("book: вектор заявки пуст")
	}
	if len(chapters) == 0 {
		return 0, ErrNoChapterVectors
	}
	dim := len(claim)

	centroid := make([]float32, dim)
	for _, c := range chapters {
		if len(c) != dim {
			return 0, fmt.Errorf("book: размерность вектора главы %d, want %d", len(c), dim)
		}
		for i, v := range c {
			centroid[i] += v
		}
	}
	n := float32(len(chapters))
	var dot, normSq float64
	for i := range centroid {
		centroid[i] /= n
		dot += float64(claim[i]) * float64(centroid[i])
		normSq += float64(centroid[i]) * float64(centroid[i])
	}
	norm := math.Sqrt(normSq)
	if norm == 0 {
		return 0, ErrNoChapterVectors
	}
	return 1 - dot/norm, nil
}

// playerPatterns собирает накопленные паттерны игрока из карты Пифии
// (player_atlas): каждая грань — «theme: значение».
func playerPatterns(h *db.Handle) ([]string, error) {
	rows, err := h.Query(`SELECT theme, interest FROM player_atlas ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var theme, interest string
		if err := rows.Scan(&theme, &interest); err != nil {
			return nil, err
		}
		out = append(out, theme+": "+interest)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// claimSynopsis читает синопсис анкеты из world_meta (опорная точка первого
// предложения; значения world_meta — JSON-строки, T-02-03).
func claimSynopsis(h *db.Handle) (string, error) {
	var raw string
	err := h.QueryRow(`SELECT value FROM world_meta WHERE "key" = 'synopsis'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("book: world_meta.synopsis не найден")
	}
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", fmt.Errorf("book: разбор world_meta.synopsis: %w", err)
	}
	return s, nil
}

// claimPrompt — промпт предложения заявки: синопсис анкеты и паттерны карты
// игрока.
func claimPrompt(synopsis string, patterns []string) string {
	raw, _ := json.Marshal(map[string]any{
		"task":     ClaimProposalRole,
		"synopsis": synopsis,
		"patterns": patterns,
	})
	return string(raw)
}
