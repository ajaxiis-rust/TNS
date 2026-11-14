package webapi

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tns65/internal/journal"
	"tns65/internal/models"
	"tns65/internal/providercfg"
)

// Мониторинг (T-08-11, UI-09, F-09): раздел «Настройки», только чтение
// пост-фактум. Снимок станка собирается из журнала станка и станочного конфига;
// управляющих маршрутов (POST/PUT/DELETE) в этом разделе нет.

const monitorPath = "/api/monitor"

// monitorJournalPath — путь журнала станка; подменяется в тестах.
var monitorJournalPath = "state/journal/engine.log"

// monitorTailLines — число последних строк журнала на первом экране.
const monitorTailLines = 20

// monitorHub — WS-хаб для живой рассылки событий мониторинга (UI-09.3).
// Подключается при сборке роутера в routes.go, как mcpHub.
var monitorHub *Hub

// monitorTopic — WS-топик экрана мониторинга; клиент подписывается на него в app.js.
const monitorTopic = "monitor"

// broadcastMonitor рассылает событие мониторинга подписчикам топика monitor.
func broadcastMonitor(eventType string) {
	if monitorHub == nil {
		return
	}
	monitorHub.Broadcast(WSMessage{
		Topic:     monitorTopic,
		EventType: eventType,
		Payload:   json.RawMessage("{}"),
	})
}

// bindMonitorJournal подключает наблюдателя журнала станка к WS-хабу: события
// деградации хода (напр. turn.degraded) рассылаются подписчикам мониторинга.
// Журнал только читается экраном, сам экран управляющих маршрутов не имеет.
func bindMonitorJournal(logger *journal.Logger) {
	if logger == nil {
		return
	}
	logger.OnEvent(func(event string) {
		if event == journal.EventTurnDegraded {
			broadcastMonitor(event)
		}
	})
}

// monitorSlot — карточка слота/провайдера первого блока (UI-09.2).
type monitorSlot struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Online         bool   `json:"online"`
	KeyValid       bool   `json:"key_valid"`
	LimitRemaining string `json:"limit_remaining,omitempty"`
	Addr           string `json:"addr,omitempty"`
}

// monitorRole — метрики роли из журнала вызовов (UI-09.2).
type monitorRole struct {
	Name          string  `json:"name"`
	Calls         int64   `json:"calls"`
	Errors        int64   `json:"errors"`
	ErrorPercent  float64 `json:"error_percent"`
	AvgResponseMS float64 `json:"avg_response_ms"`
	LastCallAt    string  `json:"last_call_at,omitempty"`
}

// monitorMachine — карточка локальной машины :5001/:5002 (UI-09.2).
type monitorMachine struct {
	Addr  string `json:"addr"`
	Alive bool   `json:"alive"`
	Queue int64  `json:"queue"`
}

// monitorResponse — пять блоков первого экрана в заданном порядке (UI-09.2).
type monitorResponse struct {
	Slots        []monitorSlot    `json:"slots"`
	Roles        []monitorRole    `json:"roles"`
	Machines     []monitorMachine `json:"machines"`
	Degradations int64            `json:"degradations"`
	JournalTail  []string         `json:"journal_tail"`
}

// handleMonitor — GET /api/monitor: снимок станка, только чтение.
func handleMonitor(w http.ResponseWriter, _ *http.Request) {
	degradations, tail := readMonitorJournal()
	writeJSON(w, http.StatusOK, monitorResponse{
		Slots:        monitorSlots(),
		Roles:        monitorRoles(),
		Machines:     monitorMachines(),
		Degradations: degradations,
		JournalTail:  tail,
	})
}

// monitorSlots — слоты генерации/эмбеддинга и внешние провайдеры. Ключи и адреса
// берутся из конфига; наружу ключи не уходят — только признак валидности.
func monitorSlots() []monitorSlot {
	keys := readSettingsKeys(settingsTOMLPath)
	llm := readLLMSettings(settingsConfigPath, keys)
	limits := latestLimits()
	slots := []monitorSlot{
		{
			Name:           "generation",
			Kind:           "slot",
			Online:         machineAlive(llm.Gen.Addr),
			KeyValid:       llm.Gen.KeySet,
			LimitRemaining: limits[generationProvider()],
			Addr:           llm.Gen.Addr,
		},
		{
			Name:     "embedding",
			Kind:     "slot",
			Online:   machineAlive(llm.Embed.Addr),
			KeyValid: llm.Embed.KeySet,
			Addr:     llm.Embed.Addr,
		},
	}
	for _, def := range readProviderCatalog(providersTOMLPath).Providers() {
		if def.Protocol == providercfg.ProtocolLocal || def.Name == "local" {
			continue
		}
		slots = append(slots, monitorSlot{
			Name:           def.Name,
			Kind:           "provider",
			KeyValid:       keys[def.Name] != "",
			LimitRemaining: limits[def.Name],
		})
	}
	return slots
}

// generationProvider — имя провайдера слота генерации из секции slots
// config.yaml; пусто — конфигурация не прочитана.
func generationProvider() string {
	cfg, err := models.LoadSlotsConfig(filepath.Join(instanceRoot(), "config.yaml"))
	if err != nil {
		return ""
	}
	return string(cfg.Default)
}

// latestLimits — остаток лимита по провайдеру из последней записи журнала
// вызовов (поле limit_remaining ответа провайдера, FIX-4). Провайдер без
// остатка в журнале в карту не попадает.
func latestLimits() map[string]string {
	out := map[string]string{}
	for _, rec := range models.ReadCalls(providerUsagePath) {
		if rec.LimitRemaining == "" {
			continue
		}
		if rec.Provider != "" {
			out[rec.Provider] = rec.LimitRemaining
		}
		out[rec.Role] = rec.LimitRemaining
	}
	return out
}

// monitorRoles — метрики ролей из журнала вызовов (UI-09.6, FIX-4): вызовы,
// ошибки, их доля, среднее время ответа и момент последнего вызова. Роль без
// записей в журнале в блок не попадает.
func monitorRoles() []monitorRole {
	type roleAgg struct {
		calls       int64
		errors      int64
		durationSum int64
		durationN   int64
		last        time.Time
	}
	order := []string{}
	aggs := map[string]*roleAgg{}
	for _, rec := range models.ReadCalls(providerUsagePath) {
		a, ok := aggs[rec.Role]
		if !ok {
			a = &roleAgg{}
			aggs[rec.Role] = a
			order = append(order, rec.Role)
		}
		if rec.Calls > 0 {
			a.calls += rec.Calls
		} else {
			a.calls++
		}
		if rec.Error != "" {
			a.errors++
		}
		if rec.DurationMS > 0 {
			a.durationSum += rec.DurationMS
			a.durationN++
		}
		if ts, err := time.Parse(time.RFC3339, rec.TS); err == nil && ts.After(a.last) {
			a.last = ts
		}
	}
	out := make([]monitorRole, 0, len(order))
	for _, name := range order {
		a := aggs[name]
		role := monitorRole{
			Name:   name,
			Calls:  a.calls,
			Errors: a.errors,
		}
		if a.calls > 0 {
			role.ErrorPercent = math.Round(float64(a.errors)*1000/float64(a.calls)) / 10
		}
		if a.durationN > 0 {
			role.AvgResponseMS = math.Round(float64(a.durationSum)*10/float64(a.durationN)) / 10
		}
		if !a.last.IsZero() {
			role.LastCallAt = a.last.UTC().Format(time.RFC3339)
		}
		out = append(out, role)
	}
	return out
}

// monitorMachines — локальные машины генерации и эмбеддингов: жива/мертва по
// TCP-доступу; длина очереди пока не измеряется (источника в станке нет).
func monitorMachines() []monitorMachine {
	keys := readSettingsKeys(settingsTOMLPath)
	llm := readLLMSettings(settingsConfigPath, keys)
	return []monitorMachine{
		{Addr: llm.Gen.Addr, Alive: machineAlive(llm.Gen.Addr), Queue: 0},
		{Addr: llm.Embed.Addr, Alive: machineAlive(llm.Embed.Addr), Queue: 0},
	}
}

// machineAlive — доступен ли TCP-адрес локальной машины.
func machineAlive(addr string) bool {
	addr = normalizeAddr(addr)
	if addr == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// normalizeAddr приводит ":port" к "127.0.0.1:port".
func normalizeAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// journalRecord — одна JSON-строка журнала станка (подмножество journal.Logger).
type journalRecord struct {
	TS     string          `json:"ts"`
	Level  string          `json:"level"`
	Source string          `json:"source"`
	Event  string          `json:"event"`
	Fields []journal.Field `json:"fields"`
}

// readMonitorJournal читает журнал станка и возвращает счётчик деградаций хода
// (события turn.degraded, F-09.1) и хвост последних строк.
func readMonitorJournal() (int64, []string) {
	data, err := os.ReadFile(monitorJournalPath)
	if err != nil {
		return 0, []string{}
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var degradations int64
	for _, line := range lines {
		if line == "" {
			continue
		}
		var rec journalRecord
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec.Event == journal.EventTurnDegraded {
			degradations++
		}
	}
	if len(lines) > monitorTailLines {
		lines = lines[len(lines)-monitorTailLines:]
	}
	tail := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			tail = append(tail, line)
		}
	}
	return degradations, tail
}
