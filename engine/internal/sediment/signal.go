package sediment

import (
	"errors"

	"tns65/internal/db"
)

// SignalKind — вид сигнала игрока о сцене (CHECK scene_signals.kind, T-04-06).
type SignalKind string

// Виды сигналов игрока: like поднимает приоритет оседания, dislike отклоняет.
const (
	SignalLike    SignalKind = "like"
	SignalDislike SignalKind = "dislike"
)

// Signal — строка сигнала игрока о сцене из scene_signals.
type Signal struct {
	SceneID   int64
	Kind      SignalKind
	CreatedAt string
}

// Приоритет заказа оседания: базовый 50 (T-02-11, KindPriority), лайк добавляет 20.
const (
	sedimentBasePriority = 50
	likePriorityBoost    = 20
)

// ErrSceneDisliked — дизлайк отклоняет сцену до очистки.
var ErrSceneDisliked = errors.New("sediment: сцена отклонена дизлайком")

// SceneSignals читает сигналы сцены из scene_signals оперативной базы мира.
// Оседание сигнал только читает (пишет его core, точка вызова — чат, T-08-15).
func SceneSignals(h *db.Handle, sceneID int64) ([]Signal, error) {
	rows, err := h.Query(
		`SELECT scene_id, kind, created_at FROM scene_signals WHERE scene_id = ? ORDER BY created_at`,
		sceneID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var signals []Signal
	for rows.Next() {
		var sceneID int64
		var kind string
		var createdAt string
		if err := rows.Scan(&sceneID, &kind, &createdAt); err != nil {
			return nil, err
		}
		signals = append(signals, Signal{SceneID: sceneID, Kind: SignalKind(kind), CreatedAt: createdAt})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return signals, nil
}

// SignalGate — отбор сцены по сигналам игрока (T-04-06, шаг 1): like поднимает
// приоритет заказа оседания в очереди на likePriorityBoost, dislike отклоняет
// сцену до очистки. Возвращает приоритет заказа; при дизлайке — ErrSceneDisliked.
func SignalGate(signals []Signal) (int, error) {
	priority := sedimentBasePriority
	liked := false
	for _, s := range signals {
		switch s.Kind {
		case SignalLike:
			liked = true
		case SignalDislike:
			return 0, ErrSceneDisliked
		}
	}
	if liked {
		priority += likePriorityBoost
	}
	return priority, nil
}
