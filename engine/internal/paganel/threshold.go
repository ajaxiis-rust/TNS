package paganel

import (
	"tns65/internal/db"
)

// RingThresholdDays — порог режима «на пороге» (W-05.4, T-06-06, шаг 1):
// герой ближе двух дней пути до края, или сцена назвала место за краем —
// постройка следующего кольца ставится в очередь. Внутри хода кольцо не
// строится: постановка откладывается до паузы/ночи (шаг 4).
const RingThresholdDays = 2.0

// HeroNearEdge сообщает, ближе ли герой, чем days дней пути, к краю известной
// земли (T-06-06, шаг 2): вокруг героя 8 секторов по 45°; сектор без известного
// места в пределах порога — край в этом направлении; край близко, если такой
// сектор есть. Дни переводятся в километры нормой дня мира (T-06-02).
func HeroNearEdge(h *db.Handle, days float64) (bool, error) {
	norm, err := ReadDayNorm(h)
	if err != nil {
		return false, err
	}
	return NearEdge(h, days*norm)
}

// PlaceBeyondEdge сообщает, лежит ли названное место за краем (T-06-06, шаг 1):
// дальше порога days дней пути от героя и дальше всех известных мест своего
// сектора — то есть за нынешним краем известной земли.
func PlaceBeyondEdge(h *db.Handle, x, y, days float64) (bool, error) {
	hero, err := ReadHeroPosition(h)
	if err != nil {
		return false, err
	}
	places, err := readKnownPlaceCoords(h)
	if err != nil {
		return false, err
	}
	norm, err := ReadDayNorm(h)
	if err != nil {
		return false, err
	}
	return namedPlaceBeyondEdge(hero.X, hero.Y, x, y, places, days*norm), nil
}

// EnsureRingOnEdge — триггер «герой ближе двух дней пути до края» (T-06-06,
// шаги 1 и 3): при срабатывании ставит следующее кольцо в очередь rings
// статусом queued (писатель core). Возвращает true, если кольцо поставлено.
func EnsureRingOnEdge(h *db.Handle, cfg PaganelConfig) (bool, error) {
	near, err := HeroNearEdge(h, RingThresholdDays)
	if err != nil {
		return false, err
	}
	if !near {
		return false, nil
	}
	if _, err := EnqueueRing(h, cfg); err != nil {
		return false, err
	}
	return true, nil
}

// EnsureRingForPlace — триггер «сцена назвала место за краем» (T-06-06, шаги 1
// и 3): при срабатывании ставит следующее кольцо в очередь rings статусом
// queued (писатель core). Возвращает true, если кольцо поставлено.
func EnsureRingForPlace(h *db.Handle, cfg PaganelConfig, x, y float64) (bool, error) {
	beyond, err := PlaceBeyondEdge(h, x, y, RingThresholdDays)
	if err != nil {
		return false, err
	}
	if !beyond {
		return false, nil
	}
	if _, err := EnqueueRing(h, cfg); err != nil {
		return false, err
	}
	return true, nil
}

// namedPlaceBeyondEdge — чистая проверка «место за краем»: место дальше
// thresholdKm от героя и дальше всех известных мест своего сектора.
func namedPlaceBeyondEdge(heroX, heroY, px, py float64, places []PlaceCoord, thresholdKm float64) bool {
	dist := haversineKm(heroX, heroY, px, py)
	if dist <= thresholdKm {
		return false
	}
	sec := sectorIndex(heroX, heroY, px, py)
	for _, p := range places {
		if sectorIndex(heroX, heroY, p.X, p.Y) != sec {
			continue
		}
		if haversineKm(heroX, heroY, p.X, p.Y) >= dist {
			return false
		}
	}
	return true
}
