package workshop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"tns65/internal/db"
)

// RunnerDeps — зависимости исполнителей заказов (T-21-04): хэндлы мира и
// литературной базы, слот мастерской и источник материала эпохи.
type RunnerDeps struct {
	World    *db.Handle
	Literary *db.Handle
	Slot     Slot
	Source   ResearchSource
}

// OrderRunner — исполнитель заказа вида (T-21-04): возвращает чекпойнт ответа
// заказа; ошибка закрывает заказ failed.
type OrderRunner func(ctx context.Context, deps RunnerDeps, o *Order) (string, error)

// NewOrderRunners — реестр исполнителей по виду заказа (T-21-04). Виды вне
// реестра (analysis, forge_plan) исполняет слот мастерской.
func NewOrderRunners() map[Kind]OrderRunner {
	return map[Kind]OrderRunner{
		KindHoroscope:  RunHoroscopeOrder,
		KindPythia:     RunPythiaOrder,
		KindEpoch:      RunEpochOrder,
		KindStyle:      RunStyleOrder,
		KindComplement: RunComplementOrder,
		KindEdit:       RunEditorOrder,
	}
}

// PythiaOrderPayload — payload заказа pythia: день ночного прогона.
type PythiaOrderPayload struct {
	Day int `json:"day"`
}

// StyleOrderPayload — payload заказа style: черновик фоновой стилизации.
type StyleOrderPayload struct {
	Draft string `json:"draft"`
}

// orderPayload разбирает payload заказа в dst; пустой payload — нулевая структура.
func orderPayload(o *Order, dst any) error {
	if strings.TrimSpace(o.Payload) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(o.Payload), dst); err != nil {
		return fmt.Errorf("workshop: payload заказа %s: %w", o.Kind, err)
	}
	return nil
}

// RunHoroscopeOrder — заказ horoscope: ковка гороскопа слотом (W-13).
func RunHoroscopeOrder(_ context.Context, deps RunnerDeps, o *Order) (string, error) {
	if deps.World == nil || deps.Literary == nil {
		return "", errors.New("workshop: заказ horoscope без хэндлов мира и литературной базы")
	}
	var p HoroscopePayload
	if err := orderPayload(o, &p); err != nil {
		return "", err
	}
	id, err := RunHoroscopeForge(deps.World, deps.Literary, p)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}

// RunPythiaOrder — заказ pythia: ночной прогон карты игрока (W-12).
func RunPythiaOrder(_ context.Context, deps RunnerDeps, o *Order) (string, error) {
	if deps.World == nil || deps.Literary == nil {
		return "", errors.New("workshop: заказ pythia без хэндлов мира и литературной базы")
	}
	var p PythiaOrderPayload
	if err := orderPayload(o, &p); err != nil {
		return "", err
	}
	if p.Day <= 0 {
		return "", errors.New("workshop: заказ pythia без дня прогона")
	}
	return "", RunPythia(deps.World, deps.Literary, p.Day)
}

// RunEpochOrder — заказ epoch: ковка справочника эпохи (W-19); эпоха выводится
// из описания мира, payload заказа не нужен.
func RunEpochOrder(ctx context.Context, deps RunnerDeps, _ *Order) (string, error) {
	if deps.World == nil || deps.Literary == nil {
		return "", errors.New("workshop: заказ epoch без хэндлов мира и литературной базы")
	}
	id, err := ForgeEpoch(ctx, deps.World, deps.Literary, deps.Source)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}

// RunStyleOrder — заказ style: фоновая стилизация черновика под классика (W-21.4).
func RunStyleOrder(ctx context.Context, deps RunnerDeps, o *Order) (string, error) {
	if deps.Literary == nil {
		return "", errors.New("workshop: заказ style без литературной базы")
	}
	var p StyleOrderPayload
	if err := orderPayload(o, &p); err != nil {
		return "", err
	}
	id, err := StylizeClassic(ctx, deps.Literary, p.Draft)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}

// RunComplementOrder — заказ complement: ковка карточки арки (T-05-02) и сверка
// спроса: заказы, чей спрос закрыт новой карточкой, снимаются.
func RunComplementOrder(_ context.Context, deps RunnerDeps, o *Order) (string, error) {
	if deps.World == nil || deps.Literary == nil {
		return "", errors.New("workshop: заказ complement без хэндлов мира и литературной базы")
	}
	var p ComplementPayload
	if err := orderPayload(o, &p); err != nil {
		return "", err
	}
	card, err := RunForgeCard(deps.Literary, p)
	if err != nil {
		return "", err
	}
	arcs, err := LoadArcs(DefaultArcsPath)
	if err != nil {
		return card, err
	}
	if err := ReconcileComplement(deps.World, arcs); err != nil {
		return card, err
	}
	return card, nil
}

// RunEditorOrder — заказ edit: пакетная редактура главы (T-05-08).
func RunEditorOrder(ctx context.Context, deps RunnerDeps, o *Order) (string, error) {
	if deps.World == nil || deps.Slot == nil {
		return "", errors.New("workshop: заказ edit без мира и слота мастерской")
	}
	var p EditorQueuePayload
	if err := orderPayload(o, &p); err != nil {
		return "", err
	}
	if p.ChapterID <= 0 {
		return "", errors.New("workshop: заказ edit без id главы")
	}
	res, err := RunEditorTask(ctx, deps.World, deps.Slot, &EditorQueueTask{ID: o.ID, ChapterID: p.ChapterID})
	if err != nil {
		return "", err
	}
	return res.Summary, nil
}
