// Package geo — общий пакет гео-расчёта мира (T-05-13): геодезическое
// расстояние и дни пути считаются кодом, модель получает готовые числа (W-20.2).
package geo

import "math"

// EarthRadiusKm — радиус Земли для формулы гаверсинусов (километры).
const EarthRadiusKm = 6371.0

// Place — место из places: реальные координаты в градусах (x — долгота,
// y — широта), как в схеме places (T-01-06).
type Place struct {
	X float64 // долгота
	Y float64 // широта
}

// Distance — геодезическое расстояние между местами по формуле гаверсинусов
// (T-05-13, шаг 1), километры.
func Distance(a, b Place) float64 {
	lat1 := a.Y * math.Pi / 180
	lat2 := b.Y * math.Pi / 180
	dLat := (b.Y - a.Y) * math.Pi / 180
	dLon := (b.X - a.X) * math.Pi / 180
	s := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * EarthRadiusKm * math.Asin(math.Sqrt(s))
}
