// Профиль цензуры и слота по рейтингу мира (T-12-07, W-14.3, IMP-13): рейтинг
// world_meta.censorship выбирает словарь (16+ — база + stoplist_16.txt, 18+ —
// база, 21+ — только stoplist_21.txt) и ограничивает пресет слота (18+ — только
// локальный).
package workshop

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"tns65/internal/censor"
	"tns65/internal/db"
	"tns65/internal/lemmarpc"
)

// Rating16Plus / Rating18Plus / Rating21Plus — рейтинги цензуры мира (B-01,
// W-14.3, IMP-13).
const (
	Rating16Plus = "16+"
	Rating18Plus = "18+"
	Rating21Plus = "21+"
)

// censorshipKey — ключ world_meta с рейтингом цензуры мира (T-02-01, поле 14).
const censorshipKey = "censorship"

// censorPackLang — язык заводского Цензора ковки: словарь читается из packs/ru.
const censorPackLang = "ru"

// WorldRating читает рейтинг цензуры мира из world_meta.censorship (значение —
// JSON-строка, T-02-03, шаг 3). Записи нет или значение пусто — дефолт 16+
// (расширенный словарь; пресеты слота не ограничены).
func WorldRating(world *db.Handle) (string, error) {
	if world == nil {
		return "", errors.New("workshop: нет хэндла мира для чтения рейтинга")
	}
	var raw string
	err := world.QueryRow(`SELECT value FROM world_meta WHERE "key" = ?`, censorshipKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Rating16Plus, nil
	}
	if err != nil {
		return "", err
	}
	var rating string
	if err := json.Unmarshal([]byte(raw), &rating); err != nil {
		return "", fmt.Errorf("workshop: разбор world_meta.%s: %w", censorshipKey, err)
	}
	if rating == "" {
		return Rating16Plus, nil
	}
	return rating, nil
}

// ProviderForRating ограничивает пресет слота рейтингом мира (W-14.3, шаг 3):
// 18+ — только локальный слот, внешние провайдеры для мира исключены; 16+ и
// прочие рейтинги используют запрошенный пресет.
func ProviderForRating(rating string, requested Provider) Provider {
	if rating == Rating18Plus {
		return ProviderLocal
	}
	return requested
}

// ProviderForWorld читает рейтинг мира и возвращает допустимый пресет слота для
// запрошенного провайдера (18+ → local).
func ProviderForWorld(world *db.Handle, requested Provider) (Provider, error) {
	rating, err := WorldRating(world)
	if err != nil {
		return "", err
	}
	return ProviderForRating(rating, requested), nil
}

// activeRatingMu/activeRating — рейтинг активного мира, поднимаемый проводкой
// webapi при открытии мира (T-12-07). Пустая строка — рейтинг не задан.
var (
	activeRatingMu sync.RWMutex
	activeRating   string
)

// SetRating задаёт рейтинг активного мира (проводка при открытии мира, T-12-07):
// 16+ — расширенный словарь, 18+ — только локальный слот. Пустая строка снимает
// рейтинг (базовый словарь).
func SetRating(rating string) {
	activeRatingMu.Lock()
	activeRating = rating
	activeRatingMu.Unlock()
}

// ActiveRating возвращает рейтинг активного мира; "" — не задан.
func ActiveRating() string {
	activeRatingMu.RLock()
	defer activeRatingMu.RUnlock()
	return activeRating
}

// checkerCensor — обёртка *censor.Checker в контракт Censor; nil — fail-closed,
// чтобы отказ сборки профиля не ронял ковку (E-01).
type checkerCensor struct {
	c *censor.Checker
}

func (w checkerCensor) Check(text string) (bool, []string) {
	if w.c == nil {
		return false, nil
	}
	return w.c.Check(text)
}

// rating16Once / rating21Once — профили собираются один раз на процесс: резидент
// лемматизатора и словарь не пересоздаются на каждый вызов.
var (
	rating16Once sync.Once
	rating21Once sync.Once
)

// rating16Censor / rating21Censor — собранные профили; nil — сборка не удалась
// (fail-closed).
var (
	rating16Censor *censor.Checker
	rating21Censor *censor.Checker
)

// buildRatingCensor собирает чекер профиля рейтинга из пакета ru; nil — пакет
// или резидент лемматизатора недоступны (fail-closed).
func buildRatingCensor(rating string) *censor.Checker {
	root := editEngineRoot()
	if root == "" {
		return nil
	}
	packDir := filepath.Join(root, "packs", censorPackLang)
	stoplist, err := censor.LoadStoplistProfile(packDir, rating)
	if err != nil {
		return nil
	}
	binary := editPackBinary(root)
	if binary == "" {
		return nil
	}
	return censor.New(lemmarpc.NewClient(binary, nil), stoplist)
}

// censorForRating возвращает Цензора рейтинга: 16+ — база + stoplist_16.txt,
// 21+ — только stoplist_21.txt (пустой словарь отключает цензора), остальные
// (18+, пустой) — только база. Тестовая подмена (SetCensor) имеет приоритет и
// разрешается в censorForWorld.
func censorForRating(rating string) Censor {
	switch rating {
	case Rating16Plus:
		rating16Once.Do(func() { rating16Censor = buildRatingCensor(Rating16Plus) })
		return checkerCensor{c: rating16Censor}
	case Rating21Plus:
		rating21Once.Do(func() { rating21Censor = buildRatingCensor(Rating21Plus) })
		return checkerCensor{c: rating21Censor}
	default:
		return censorAdapter{}
	}
}

// censorForWorld возвращает Цензора ковки по рейтингу мира: тестовая подмена
// (SetCensor) имеет приоритет, иначе рейтинг выбирает профиль словаря.
func censorForWorld(world *db.Handle) Censor {
	censorMu.RLock()
	override := curCensor
	censorMu.RUnlock()
	if override != nil {
		return override
	}
	rating, err := WorldRating(world)
	if err != nil {
		return censorAdapter{}
	}
	return censorForRating(rating)
}
