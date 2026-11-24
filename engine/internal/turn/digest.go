package turn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tns65/internal/db"
)

// Разделение сводки и вести (T-03-14, LT-13): при возврате игрока мир доставляет
// сводку «пока тебя не было», если сработало отложенное событие или отсутствие
// превысило порог ночи; остальное, случившееся в отсутствие, приходит
// запаздывающими вестями. Расхождения кругов от скачков или паузы не маскируются,
// вести не ускоряются — в этом мире они могут идти неделями.

// События сводки и вестей в оперативной базе мира (events, писатель core).
const (
	digestEventKind = "digest"
	newsEventKind   = "news"
)

// NewsEventKind — вид события запаздывающих вестей (T-03-14, шаг 2). Один
// возврат пишет одно событие со списком вестей: последнее событие news — вести
// последнего возврата, его читает GET /api/state в поле return_news.
const NewsEventKind = newsEventKind

// digestSummaryText — сводка при возврате (T-03-14, шаг 1): непустая строка,
// которую игрок видит первой после отсутствия.
const digestSummaryText = "Пока тебя не было, мир изменился."

// newsLineFmt — запаздывающая весть о продвижении дальнего круга (шаг 2).
const newsLineFmt = "Весть: дальний круг мира продвинулся (%d)."

// Digest — сводка и вести, накопленные за отсутствие (T-03-14).
type Digest struct {
	Summary       string   // сводка «пока тебя не было»; пусто — сводки нет.
	News          []string // запаздывающие вести об остальном.
	AbsentMin     int64    // длительность отсутствия в целых минутах.
	DeferredFired int      // сколько отложенных событий сработало.
}

// buildDigest строит сводку и вести по итогу паузы. Сводка формируется в двух
// случаях (LT-13.1): сработало отложенное событие или отсутствие превысило порог
// ночи. Остальное, случившееся в отсутствие, доставляется запаздывающими вестями.
func buildDigest(report AbsenceReport, absentFor time.Duration, cfg TimeConfig) Digest {
	d := Digest{
		AbsentMin:     int64(absentFor / time.Minute),
		DeferredFired: report.DeferredFired,
	}
	night := cfg.NightThresholdMin
	if night <= 0 {
		night = NightThresholdMin
	}
	if report.DeferredFired > 0 || d.AbsentMin >= night {
		d.Summary = digestSummaryText
	}
	for i := 0; i < report.FatesAdvanced; i++ {
		d.News = append(d.News, fmt.Sprintf(newsLineFmt, i+1))
	}
	return d
}

// writeDigestEvents пишет сводку и вести в events оперативной базы мира хэндлом
// core: сводка — одно событие digest, вести возврата — одно событие news со
// списком текстов.
func writeDigestEvents(h *db.Handle, d Digest) error {
	if h == nil {
		return nil
	}
	if d.Summary != "" {
		payload, err := json.Marshal(map[string]any{
			"summary":        d.Summary,
			"absent_min":     d.AbsentMin,
			"deferred_fired": d.DeferredFired,
		})
		if err != nil {
			return err
		}
		if _, err := h.Exec("events",
			`INSERT INTO events (day, kind, payload_json, source) VALUES (0, ?, ?, ?)`,
			digestEventKind, string(payload), eventSourceCore); err != nil {
			return err
		}
	}
	if len(d.News) > 0 {
		payload, err := json.Marshal(map[string][]string{"texts": d.News})
		if err != nil {
			return err
		}
		if _, err := h.Exec("events",
			`INSERT INTO events (day, kind, payload_json, source) VALUES (0, ?, ?, ?)`,
			newsEventKind, string(payload), eventSourceCore); err != nil {
			return err
		}
	}
	return nil
}

// digestStage — разделение сводки и вести (T-03-14, «Правится» stages.go): на
// возврате игрока сводка ложится перед ответом хода, вести — следом. Сводка
// формируется при сработавшем отложенном событии или отсутствии дольше порога
// ночи. Выбранная весть может подаваться сценой с диалогом НПС (IMP-5); сцена
// недоступна — весть остаётся текстом.
type digestStage struct {
	db  *db.Handle
	cfg TimeConfig // порог ночи; нулевое значение заменяется дефолтом.
	// scene — слот мастерской для сцены вести (IMP-5); nil — слот не подключён,
	// вести подаются только текстом.
	scene NewsSceneSlot
	// sceneCfg — настройка сценовой подачи вестей; нулевое значение — сцена
	// выключена.
	sceneCfg NewsSceneConfig
}

func (*digestStage) Name() string { return "digest" }

func (s *digestStage) Run(ctx context.Context, st *TurnState) error {
	st.DigestScene = NewsScene{NewsIndex: -1}
	d := buildDigest(st.Absence, st.AbsentFor, s.cfg)
	if d.Summary == "" && len(d.News) == 0 {
		return nil
	}
	if err := writeDigestEvents(s.db, d); err != nil {
		return err
	}
	st.DigestSummary = d.Summary
	st.DigestNews = d.News
	reply := st.Reply
	if d.Summary != "" {
		reply = d.Summary + "\n" + reply
	}
	if len(d.News) > 0 {
		reply += "\n" + strings.Join(d.News, "\n")
	}
	// Сценовая подача (IMP-5): брак или недоступность слота — весть текстом,
	// ход не валится.
	if idx := sceneNewsIndex(d.News, s.sceneCfg); idx >= 0 {
		if scene, ok := s.makeNewsScene(ctx, st, d.News[idx], idx); ok {
			st.DigestScene = scene
			reply = strings.Replace(reply, d.News[idx], scene.Prose, 1)
		}
	}
	st.Reply = reply
	return nil
}

// makeNewsScene пишет сцену вести штатным путём в scenes хэндлом core; false —
// сцена не удалась (слот не подключён, отказ, брак или ошибка записи), весть
// остаётся текстовой.
func (s *digestStage) makeNewsScene(ctx context.Context, st *TurnState, news string, idx int) (NewsScene, bool) {
	if s.scene == nil || s.db == nil {
		return NewsScene{}, false
	}
	out, err := s.scene.Generate(ctx, newsSceneRole, newsScenePrompt(news, st.Roster))
	if err != nil {
		return NewsScene{}, false
	}
	prose := strings.TrimSpace(out)
	// Сцена вести — прямая речь названного лица: табу его идиолекта в прозу
	// хода не идёт (B-15, правило 5). Реплика убрана — сцена остаётся без неё;
	// опустевшая сцена — брак, весть подаётся текстом.
	if actor := newsSceneActor(st.Roster); actor != nil {
		prose = strings.TrimSpace(speechWithoutTabooBy(actor.Tags, prose))
	}
	if !sceneProseUsable(prose) {
		return NewsScene{}, false
	}
	res, err := s.db.Exec("scenes",
		`INSERT INTO scenes (session_id, day, place_id, status, prose, turn_id) VALUES (?, 0, ?, ?, ?, NULL)`,
		st.Input.SessionID, st.Prepath.Frame.HeroPlaceID, sceneStatusOpen, prose)
	if err != nil {
		return NewsScene{}, false
	}
	id, _ := res.LastInsertId()
	return NewsScene{NewsIndex: idx, SceneID: id, Prose: prose}, true
}
