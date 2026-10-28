package webapi

import (
	"net/http"
	"strings"

	"tns65/internal/db"
	"tns65/internal/paganel"
)

// Прорисовка карты и граф отношений (T-08-06, UI-05.5, UI-05.6, W-20):
// JSON-эндпоинты только чтение над world(<slug>).db. Рендер карты и графа — в
// браузере (map.js/graph.js); webapi ничего не пишет.

const (
	mapViewportPath   = "/api/atlas/worlds/map"
	relationGraphPath = "/api/atlas/worlds/graph"
)

// mapViewportResponse — ответ окна «Прорисовка карты»: данные вьюпорта Атласа
// (T-06-13) плюс слаг мира. Координаты передаются как есть, расстояния и дни —
// готовыми числами от кода (W-20.1).
type mapViewportResponse struct {
	World  string               `json:"world"`
	Places []paganel.AtlasPlace `json:"places"`
	Paths  []paganel.AtlasPath  `json:"paths"`
	Hero   paganel.AtlasHero    `json:"hero"`
	Rings  []paganel.AtlasRing  `json:"rings"`
}

// relationGraphNode — персонаж в графе отношений.
type relationGraphNode struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// relationGraphEdge — связь между двумя персонажами из social_graph.
type relationGraphEdge struct {
	ID       int64   `json:"id"`
	A        int64   `json:"a_character_id"`
	B        int64   `json:"b_character_id"`
	Relation string  `json:"relation"`
	Weight   float64 `json:"weight"`
}

// relationGraphResponse — ответ окна «Граф отношений».
type relationGraphResponse struct {
	World string              `json:"world"`
	Nodes []relationGraphNode `json:"nodes"`
	Edges []relationGraphEdge `json:"edges"`
}

// openMapReader открывает оперативную базу мира только на чтение. Тег писателя
// не важен: хэндл используется только для Query.
func openMapReader(slug string) (*db.Handle, error) {
	return db.OpenReaderAs(worldDBPath(slug), db.DBWorld, db.WriterCore, nil)
}

// handleMapViewport — GET /api/atlas/worlds/map?world=<slug>: места, пути,
// положение героя и кольца из вьюпорта Атласа (T-06-13). Только чтение.
func handleMapViewport(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}

	h, err := openMapReader(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	v, err := paganel.BuildAtlasView(h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "map viewport failed")
		return
	}

	places := v.Places
	if places == nil {
		places = []paganel.AtlasPlace{}
	}
	paths := v.Paths
	if paths == nil {
		paths = []paganel.AtlasPath{}
	}
	rings := v.Rings
	if rings == nil {
		rings = []paganel.AtlasRing{}
	}

	writeJSON(w, http.StatusOK, mapViewportResponse{
		World:  slug,
		Places: places,
		Paths:  paths,
		Hero:   v.Hero,
		Rings:  rings,
	})
}

// handleRelationGraph — GET /api/atlas/worlds/graph?world=<slug>: персонажи и
// связи из social_graph. Только чтение.
func handleRelationGraph(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("world"))
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeBadRequest, "world slug required")
		return
	}

	h, err := openMapReader(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "world not found")
		return
	}
	defer h.Close()

	nodes, err := readRelationGraphNodes(h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "relation graph failed")
		return
	}
	edges, err := readRelationGraphEdges(h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, codeInternal, "relation graph failed")
		return
	}

	writeJSON(w, http.StatusOK, relationGraphResponse{
		World: slug,
		Nodes: nodes,
		Edges: edges,
	})
}

// readRelationGraphNodes читает персонажей из characters (только чтение).
func readRelationGraphNodes(h *db.Handle) ([]relationGraphNode, error) {
	rows, err := h.Query(`SELECT id, name FROM characters ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make([]relationGraphNode, 0)
	for rows.Next() {
		var n relationGraphNode
		if err := rows.Scan(&n.ID, &n.Name); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// readRelationGraphEdges читает связи из social_graph (только чтение).
func readRelationGraphEdges(h *db.Handle) ([]relationGraphEdge, error) {
	rows, err := h.Query(
		`SELECT id, a_character_id, b_character_id, relation, weight
		 FROM social_graph ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	edges := make([]relationGraphEdge, 0)
	for rows.Next() {
		var e relationGraphEdge
		if err := rows.Scan(&e.ID, &e.A, &e.B, &e.Relation, &e.Weight); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}
