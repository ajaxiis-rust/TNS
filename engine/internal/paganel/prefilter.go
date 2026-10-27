package paganel

import (
	"errors"
	"fmt"
	"strings"

	"tns65/internal/db"
	"tns65/internal/sediment"
)

// Prefilter — префильтр конвейера оседания (T-06-14, W-04.7): отсеивает сцену,
// чьи места не сверяются с географией мира (places/player_map). Правило отсева —
// только география; смысловую оценку конвейеру дают дедуп и авто-критик.
type Prefilter struct {
	places map[string]struct{}
}

// NewPrefilter собирает префильтр по хэндлу мира: известная география — имена
// мест из places и мест, известных игроку из player_map. Хэндл только читается.
func NewPrefilter(h *db.Handle) (*Prefilter, error) {
	if h == nil {
		return nil, errors.New("paganel: хэндл мира не задан")
	}
	p := &Prefilter{places: map[string]struct{}{}}
	if err := p.loadPlaces(h); err != nil {
		return nil, err
	}
	if err := p.loadPlayerMap(h); err != nil {
		return nil, err
	}
	return p, nil
}

// loadPlaces кладёт имена всех мест places в известную географию.
func (p *Prefilter) loadPlaces(h *db.Handle) error {
	rows, err := h.Query(`SELECT name FROM places`)
	if err != nil {
		return fmt.Errorf("paganel: читать places: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("paganel: разбор места: %w", err)
		}
		p.addPlace(name)
	}
	return rows.Err()
}

// loadPlayerMap добавляет имена мест, известных игроку (player_map → places).
func (p *Prefilter) loadPlayerMap(h *db.Handle) error {
	rows, err := h.Query(`SELECT pl.name FROM player_map pm JOIN places pl ON pl.id = pm.place_id`)
	if err != nil {
		return fmt.Errorf("paganel: читать player_map: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("paganel: разбор player_map: %w", err)
		}
		p.addPlace(name)
	}
	return rows.Err()
}

// addPlace кладёт имя места в множество в нижнем регистре; пустые пропускаются.
func (p *Prefilter) addPlace(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	p.places[strings.ToLower(name)] = struct{}{}
}

// Keep — сцена пропускается, если хотя бы одно её место сверяется с географией
// мира (W-04.7); иначе отсеивается. Смысловую оценку префильтр не делает.
func (p *Prefilter) Keep(scene sediment.Scene) bool {
	if p == nil || p.places == nil {
		return false
	}
	prose := strings.ToLower(scene.Prose)
	for name := range p.places {
		if strings.Contains(prose, name) {
			return true
		}
	}
	return false
}
