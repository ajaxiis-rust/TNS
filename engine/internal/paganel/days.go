package paganel

import (
	"fmt"

	"tns65/internal/db"
	"tns65/internal/geo"
)

// DaysCalc — расчёт дней пути на лету поверх internal/geo (T-06-02): дни
// никогда не хранятся в базе и всегда вычисляются из координат и трения
// (W-06.2, W-06.5).
type DaysCalc struct {
	gm  *Geomodel
	geo *geo.Geo
}

// NewDaysCalc создаёт расчётчик дней с нормой дня мира dayNormKm (км в день);
// значение вне диапазона [20,30] — отказ с причиной (T-06-02, шаг 2, W-06.4).
func NewDaysCalc(dayNormKm float64, gm *Geomodel) (*DaysCalc, error) {
	g, err := geo.NewGeo(dayNormKm)
	if err != nil {
		return nil, fmt.Errorf("paganel: %w", err)
	}
	return &DaysCalc{gm: gm, geo: g}, nil
}

// DayNormKm возвращает норму дня мира расчётчика.
func (c *DaysCalc) DayNormKm() float64 { return c.geo.DayNormKm() }

// ReadDayNorm читает dayNormKm из world_meta (ключ dayNormKm): записи нет —
// дефолт 25; значение вне диапазона [20,30] — отказ с причиной (T-06-02, шаг 2).
func ReadDayNorm(h *db.Handle) (float64, error) {
	return geo.ReadDayNorm(h)
}

// Days — дни пути между местами from и to (T-06-02, шаг 1, W-06.3): известный
// путь — length_km/(норма·трение); пути нет — геодезическое расстояние с
// трением 1.0 (оценка). Результат — дробные дни; показ игроку — geo.CeilDays.
func (c *DaysCalc) Days(from, to Place) float64 {
	gf := geo.Place{X: from.X, Y: from.Y}
	gt := geo.Place{X: to.X, Y: to.Y}
	if p := c.knownPath(from.ID, to.ID); p != nil {
		return c.geo.Days(gf, gt, &geo.Path{LengthKm: p.LengthKm, Friction: p.FrictionValue()})
	}
	return c.geo.Days(gf, gt, nil)
}

// knownPath — известный путь между местами в любом направлении; при нескольких
// путях выбирается быстрейший (минимум дней). nil — известного пути нет.
func (c *DaysCalc) knownPath(a, b int64) *Path {
	if c.gm == nil {
		return nil
	}
	var best *Path
	var bestDays float64
	for i := range c.gm.Paths {
		p := &c.gm.Paths[i]
		if (p.FromPlaceID == a && p.ToPlaceID == b) || (p.FromPlaceID == b && p.ToPlaceID == a) {
			days := p.LengthKm / (c.geo.DayNormKm() * p.FrictionValue())
			if best == nil || days < bestDays {
				best, bestDays = p, days
			}
		}
	}
	return best
}
