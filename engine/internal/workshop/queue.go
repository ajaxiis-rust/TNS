package workshop

import (
	"encoding/json"
	"fmt"
	"time"

	"tns65/internal/db"
)

// Kind — вид заказа мастерской (T-02-11, шаг 1).
type Kind string

// Единый реестр видов заказов. Обязан совпадать с CHECK в
// 0003_workshop_queue.sql: новый вид добавляется правкой CHECK вместе с
// задачей-владельцем.
const (
	KindAnalysis   Kind = "analysis"
	KindForgePlan  Kind = "forge_plan"
	KindComplement Kind = "complement"
	KindHoroscope  Kind = "horoscope"
	KindPythia     Kind = "pythia"
	KindEdit       Kind = "edit"
	KindEpoch      Kind = "epoch"
	KindStyle      Kind = "style"
	KindSediment   Kind = "sediment"
	KindRing       Kind = "ring"
	KindWakeup     Kind = "wakeup"
)

// Kinds — виды заказов в порядке реестра CHECK.
var Kinds = []Kind{
	KindAnalysis, KindForgePlan, KindComplement, KindHoroscope, KindPythia,
	KindEdit, KindEpoch, KindStyle, KindSediment, KindRing, KindWakeup,
}

// Status — статус заказа; меняет только ядро.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusDone      Status = "done"
	StatusCancelled Status = "cancelled"
	StatusFailed    Status = "failed"
)

// Step — ступень заказа (T-05-01 сопоставляет ступени паузы весу работ).
type Step string

const (
	StepLight Step = "light"
	StepHeavy Step = "heavy"
)

// Приоритеты видов (T-02-11, шаг 1): 100 пробуждение, 60 гороскоп, 50 оседание,
// 30 доукомплектация, 20 кольца, 10 стиль. Остальные виды (в т.ч. analysis и
// forge_plan) получают defaultPriority.
const defaultPriority = 50

// KindPriority возвращает приоритет вида заказа по реестру T-02-11.
func KindPriority(k Kind) int {
	switch k {
	case KindWakeup:
		return 100
	case KindHoroscope:
		return 60
	case KindSediment:
		return 50
	case KindComplement:
		return 30
	case KindRing:
		return 20
	case KindStyle:
		return 10
	default:
		return defaultPriority
	}
}

// Provider — пресет слота мастерской (T-02-11, шаг 3).
type Provider string

const (
	ProviderLocal       Provider = "local"
	ProviderGeminiFlash Provider = "gemini-flash"
	ProviderGeminiPro   Provider = "gemini-pro"
)

// DefaultProvider — пресет по умолчанию: local.
const DefaultProvider = ProviderLocal

// Адреса локальных слотов: генерация :5001, эмбеддинги :5002 (шаг 3).
const (
	localGenAddr   = ":5001"
	localEmbedAddr = ":5002"
)

// Preset — заводской пресет слота: провайдер и адреса локальных слотов.
type Preset struct {
	Provider  Provider
	GenAddr   string
	EmbedAddr string
}

// Presets — заводской реестр пресетов (шаг 3): local (дефолт), gemini-flash,
// gemini-pro. Ключи внешних провайдеров вводятся в UI и хранятся в settings.toml
// вне world_db/; локальных адресов у внешних пресетов нет.
func Presets() []Preset {
	return []Preset{
		{Provider: ProviderLocal, GenAddr: localGenAddr, EmbedAddr: localEmbedAddr},
		{Provider: ProviderGeminiFlash},
		{Provider: ProviderGeminiPro},
	}
}

// GenSlot — адрес слота генерации для пресета; внешние пресеты локального адреса
// не имеют. Выбор слота по пресету — T-02-14.
func GenSlot(p Provider) string {
	if p == ProviderLocal {
		return localGenAddr
	}
	return ""
}

// Enqueue ставит заказ kind в очередь мира хэндлом core (статус queued) и
// привязывает его к слоту по пресету через model_bindings (шаг 3). Статус заказа
// меняет только ядро; снаружи заказ создаётся только queued.
func Enqueue(h *db.Handle, kind Kind, step Step, provider Provider, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("workshop: payload заказа %s: %w", kind, err)
	}
	if _, err := h.Exec("workshop_queue",
		`INSERT INTO workshop_queue (kind, payload_json, priority, status, step, resumable, attempts, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(kind), string(raw), KindPriority(kind), string(StatusQueued), string(step), 0, 0, nowUTC()); err != nil {
		return err
	}
	return bindSlot(h, kind, provider)
}

// EnqueueBirthOrders — постановка двух заказов рождения (шаг 2): analysis
// (анализ данных первичного этапа) и forge_plan (план ковки всей книги), оба
// через слот мастерской, пресет по умолчанию local. До исполнителя очереди (P4)
// заказы живут со статусом queued. Повторная постановка того же заказа для того
// же мира не создаёт дубль (шаг 4).
func EnqueueBirthOrders(h *db.Handle) error {
	for _, o := range []struct {
		kind Kind
		step Step
	}{
		{KindAnalysis, StepHeavy},
		{KindForgePlan, StepLight},
	} {
		if err := enqueueOnce(h, o.kind, o.step); err != nil {
			return err
		}
	}
	return nil
}

// enqueueOnce ставит заказ, если заказа того же вида для этого мира ещё нет.
func enqueueOnce(h *db.Handle, kind Kind, step Step) error {
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM workshop_queue WHERE kind = ?`, string(kind)).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return Enqueue(h, kind, step, DefaultProvider, map[string]string{"kind": string(kind)})
}

// bindSlot привязывает вид заказа к слоту: строка model_bindings role=kind,
// model_ref=адрес слота, provider=пресет. Повторная привязка не дублирует строку.
func bindSlot(h *db.Handle, kind Kind, provider Provider) error {
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM model_bindings WHERE role = ?`, string(kind)).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := h.Exec("model_bindings",
		`INSERT INTO model_bindings (role, model_ref, provider, updated_at) VALUES (?, ?, ?, ?)`,
		string(kind), GenSlot(provider), string(provider), nowUTC()); err != nil {
		return err
	}
	return nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
