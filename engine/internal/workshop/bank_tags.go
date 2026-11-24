package workshop

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"tns65/internal/db"
	"tns65/internal/deepread"
)

// bankTagsForChunk собирает настоящие теги записи банка чанка: ключи категорий
// разбора book_chunk_tag (кроме полосы брака) и теги сцен book_scene, в диапазон
// которых входит чанк. Теги — данные разбора, не текст чанка; пустой набор —
// разбор ещё не прошёл.
func bankTagsForChunk(lit *db.Handle, bookID int64, ord int) (string, error) {
	if lit == nil {
		return "", errors.New("workshop: нет литературной базы для тегов банка")
	}
	set := make(map[string]struct{})
	if err := addChunkTagKeys(lit, bookID, ord, set); err != nil {
		return "", err
	}
	if err := addSceneTags(lit, bookID, ord, set); err != nil {
		return "", err
	}
	if len(set) == 0 {
		return "", nil
	}
	tags := make([]string, 0, len(set))
	for tag := range set {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return strings.Join(tags, ","), nil
}

// addChunkTagKeys добавляет ключи категорий чанка: полоса reject — брак, банку
// не тег.
func addChunkTagKeys(lit *db.Handle, bookID int64, ord int, set map[string]struct{}) error {
	rows, err := lit.Query(
		`SELECT t.key FROM book_chunk_tag t JOIN book_chunk c ON c.id = t.chunk_id
		 WHERE c.book_id = ? AND c.ord = ? AND t.band != ?`,
		bookID, ord, deepread.BandReject)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return err
		}
		if key = strings.TrimSpace(key); key != "" {
			set[key] = struct{}{}
		}
	}
	return rows.Err()
}

// addSceneTags добавляет теги сцен книги, покрывающих чанк: book_scene.tags —
// JSON-массив ключей категорий.
func addSceneTags(lit *db.Handle, bookID int64, ord int, set map[string]struct{}) error {
	rows, err := lit.Query(
		`SELECT tags FROM book_scene
		 WHERE book_id = ? AND from_chunk <= ? AND to_chunk >= ?`,
		bookID, ord, ord)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var tags []string
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(raw), &tags); err != nil {
			return fmt.Errorf("workshop: теги сцены: %w", err)
		}
		for _, tag := range tags {
			if tag = strings.TrimSpace(tag); tag != "" {
				set[tag] = struct{}{}
			}
		}
	}
	return rows.Err()
}

// SyncBankTags заполняет пустые теги записей банка книги тегами разбора
// (bankTagsForChunk). Ковка идёт до разбора, а теги чанков появляются позже,
// поэтому forge-book зовёт синхронизацию после разбора; повторный вызов меняет
// только пустые теги и возвращает число заполненных строк.
func SyncBankTags(lit *db.Handle, bookID int64) (int, error) {
	if lit == nil {
		return 0, errors.New("workshop: нет литературной базы для тегов банка")
	}
	rows, err := lit.Query(
		`SELECT id, origin FROM bank_pool WHERE book_id = ? AND tags = ''`, bookID)
	if err != nil {
		return 0, err
	}
	type pending struct {
		id  int64
		ord int
	}
	var work []pending
	for rows.Next() {
		var id int64
		var origin string
		if err := rows.Scan(&id, &origin); err != nil {
			rows.Close()
			return 0, err
		}
		if ord, ok := forgeBookOrd(origin, bookID); ok {
			work = append(work, pending{id: id, ord: ord})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	updated := 0
	for _, w := range work {
		tags, err := bankTagsForChunk(lit, bookID, w.ord)
		if err != nil {
			return updated, err
		}
		if tags == "" {
			continue
		}
		res, err := lit.Exec("bank_pool",
			`UPDATE bank_pool SET tags = ? WHERE id = ? AND tags = ''`, tags, w.id)
		if err != nil {
			return updated, err
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			updated++
		}
	}
	return updated, nil
}

// forgeBookOrd разбирает номер чанка из origin записи банка forge-book.
func forgeBookOrd(origin string, bookID int64) (int, bool) {
	prefix := fmt.Sprintf("forge-book:%d:", bookID)
	if !strings.HasPrefix(origin, prefix) {
		return 0, false
	}
	ord, err := strconv.Atoi(strings.TrimPrefix(origin, prefix))
	if err != nil || ord < 0 {
		return 0, false
	}
	return ord, true
}
