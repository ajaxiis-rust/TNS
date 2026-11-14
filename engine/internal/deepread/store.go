package deepread

import (
	"encoding/json"
	"errors"
	"fmt"

	"tns65/internal/db"
)

// ErrNoStore — запись тегов без литературной базы: тег чанка некуда положить
// (W-23.3, писатель — forge).
var ErrNoStore = errors.New("deepread: нет литературной базы для тегов чанков")

// WriteChunkTags пишет теги чанков в book_chunk_tag одной транзакцией (W-23.3,
// T-14-05): строки чанка заменяются целиком, поэтому повторная запись тех же
// тегов строк не плодит, а обновляет их. Пустой набор тегов чанка стирает его
// строки.
func WriteChunkTags(lit *db.Handle, tags []ChunkTags) error {
	if lit == nil {
		return ErrNoStore
	}
	return lit.Tx(db.WriterForge, func(tx *db.Tx) error {
		for _, chunk := range tags {
			if chunk.ChunkID <= 0 {
				return fmt.Errorf("deepread: тег чанка: chunk_id %d", chunk.ChunkID)
			}
			if _, err := tx.Exec("book_chunk_tag",
				`DELETE FROM book_chunk_tag WHERE chunk_id = ?`, chunk.ChunkID); err != nil {
				return fmt.Errorf("deepread: теги чанка %d: %w", chunk.ChunkID, err)
			}

			seen := make(map[string]struct{}, len(chunk.Tags))
			for _, tag := range chunk.Tags {
				if tag.Key == "" {
					return fmt.Errorf("deepread: тег чанка %d: пустой ключ", chunk.ChunkID)
				}
				if _, dup := seen[tag.Key]; dup {
					return fmt.Errorf("deepread: тег чанка %d: ключ %q повторяется", chunk.ChunkID, tag.Key)
				}
				seen[tag.Key] = struct{}{}
				if !validBand(tag.Band) {
					return fmt.Errorf("deepread: тег чанка %d: ключ %q: полоса %q вне confident/reject/uncertain",
						chunk.ChunkID, tag.Key, tag.Band)
				}
				anchors, err := marshalAnchors(tag.Anchors)
				if err != nil {
					return err
				}
				if _, err := tx.Exec("book_chunk_tag",
					`INSERT INTO book_chunk_tag (chunk_id, key, score, band, anchors) VALUES (?, ?, ?, ?, ?)`,
					chunk.ChunkID, tag.Key, tag.Score, tag.Band, anchors); err != nil {
					return fmt.Errorf("deepread: тег чанка %d, ключ %q: %w", chunk.ChunkID, tag.Key, err)
				}
			}
		}
		return nil
	})
}

// validBand — полоса тега входит в канон DDL book_chunk_tag.
func validBand(band string) bool {
	switch band {
	case BandConfident, BandUncertain, BandReject:
		return true
	}
	return false
}

// marshalAnchors кодирует якоря JSON-массивом строк; пустой набор — «[]», как
// в дефолте колонки anchors.
func marshalAnchors(anchors []string) (string, error) {
	if len(anchors) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(anchors)
	if err != nil {
		return "", fmt.Errorf("deepread: якоря тега: %w", err)
	}
	return string(raw), nil
}
