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
// core: сводка — одно событие digest, каждая весть — событие news.
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
	for _, n := range d.News {
		payload, err := json.Marshal(map[string]string{"text": n})
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
// ночи; LLM не вызывается.
type digestStage struct {
	db  *db.Handle
	cfg TimeConfig // порог ночи; нулевое значение заменяется дефолтом.
}

func (*digestStage) Name() string { return "digest" }

func (s *digestStage) Run(_ context.Context, st *TurnState) error {
	d := buildDigest(st.Absence, st.AbsentFor, s.cfg)
	if d.Summary == "" && len(d.News) == 0 {
		return nil
	}
	if err := writeDigestEvents(s.db, d); err != nil {
		return err
	}
	st.DigestSummary = d.Summary
	st.DigestNews = d.News
	if d.Summary != "" {
		st.Reply = d.Summary + "\n" + st.Reply
	}
	if len(d.News) > 0 {
		st.Reply += "\n" + strings.Join(d.News, "\n")
	}
	return nil
}
