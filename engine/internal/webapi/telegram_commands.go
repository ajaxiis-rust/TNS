package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tns65/internal/workshop"
)

// Команды Telegram-бота (W2): сообщение со слеша — команда, остальное — ход.
// Каждая команда — loopback HTTP-вызов к веб-интерфейсу (тот же клиент, что
// ForwardChat) с коротким тайм-аутом; промах команды — короткое человеческое
// сообщение, без стека.

// commandTimeout — короткий тайм-аут loopback-команды: зависший вызов не
// блокирует long poll.
const commandTimeout = 5 * time.Second

// providersPath — маршрут карточек внешних провайдеров (GET /api/providers).
const providersPath = "/api/providers"

// Тексты ответов команд.
const (
	commandHelp = "Команды:\n" +
		"/help — список команд\n" +
		"/worlds — список миров\n" +
		"/world [слаг] — текущий мир или переключение\n" +
		"/stats — состояние героя\n" +
		"/model — провайдеры и роли\n" +
		"/model <роль> <провайдер> — привязка роли к провайдеру"

	commandFailureText = "Не получилось выполнить команду, попробуйте ещё раз."
	commandUnknownText = "Не знаю такой команды. Список — /help."
	commandNoWorldText = "Активный мир не выбран."
)

// knownRole сообщает, что роль показывается экраном Настроек (виды заказов
// мастерской T-02-11) и может быть привязана к провайдеру.
func knownRole(role string) bool {
	for _, k := range workshop.Kinds {
		if string(k) == role {
			return true
		}
	}
	return false
}

// parseCommand разбирает сообщение-команду: первый токен — имя команды (без
// @BotName, ASCII приводится к нижнему регистру), остаток — аргумент.
func parseCommand(text string) (name, arg string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}
	cmd := fields[0]
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	name = strings.ToLower(cmd)
	arg = strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	return name, arg
}

// handleCommand разбирает и выполняет одну команду Telegram, а ответ шлёт
// sendMessage. Ошибка команды превращается в короткое человеческое сообщение.
func (b *TelegramBot) handleCommand(ctx context.Context, text string) {
	name, arg := parseCommand(text)
	if name == "" {
		return
	}
	cmdCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	reply, err := b.runCommand(cmdCtx, name, arg)
	if err != nil {
		reply = commandFailureText
	}
	if reply == "" {
		return
	}
	_ = b.SendChat(ctx, reply)
}

// runCommand маршрутизирует команду по имени. Возвращённая ошибка — промах
// loopback-вызова; на уровне handleCommand она превращается в человеческое
// сообщение без стека.
func (b *TelegramBot) runCommand(ctx context.Context, name, arg string) (string, error) {
	switch name {
	case "/help":
		return commandHelp, nil
	case "/worlds":
		return b.cmdWorlds(ctx)
	case "/world":
		return b.cmdWorld(ctx, arg)
	case "/stats":
		return b.cmdStats(ctx)
	case "/model":
		return b.cmdModel(ctx, arg)
	default:
		return commandUnknownText, nil
	}
}

// loopbackJSON выполняет JSON-запрос к loopback-серверу тем же клиентом, что
// ForwardChat. Тело запроса — body (кодируется в JSON), ответ декодируется в out.
func (b *TelegramBot) loopbackJSON(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(b.localBase, "/")+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.local.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webapi: loopback %s %s: status %d", method, path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// cmdWorlds — /worlds: список миров из GET /api/atlas/worlds, активный помечен.
func (b *TelegramBot) cmdWorlds(ctx context.Context) (string, error) {
	var out atlasListResponse
	if err := b.loopbackJSON(ctx, http.MethodGet, atlasWorldsPath, nil, &out); err != nil {
		return "", err
	}
	if len(out.Worlds) == 0 {
		return "Миров нет.", nil
	}
	var sb strings.Builder
	for _, w := range out.Worlds {
		mark := ""
		if w.Active {
			mark = " ← активный"
		}
		fmt.Fprintf(&sb, "%s — %s%s\n", w.Slug, w.Title, mark)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// cmdWorld — /world: без аргумента показывает текущий мир, с аргументом
// переключает активный мир POST /api/atlas/worlds/active (хэндлер сам поднимает
// in-process активный слаг, и ForwardChat перестаёт отвечать «выберите мир»).
func (b *TelegramBot) cmdWorld(ctx context.Context, arg string) (string, error) {
	slug := strings.TrimSpace(arg)
	if slug == "" {
		var out atlasListResponse
		if err := b.loopbackJSON(ctx, http.MethodGet, atlasWorldsPath, nil, &out); err != nil {
			return "", err
		}
		if out.Active == "" {
			return commandNoWorldText, nil
		}
		for _, w := range out.Worlds {
			if w.Slug == out.Active {
				return fmt.Sprintf("Текущий мир: %s — %s", w.Slug, w.Title), nil
			}
		}
		return fmt.Sprintf("Текущий мир: %s", out.Active), nil
	}

	var res struct {
		Active string `json:"active"`
	}
	if err := b.loopbackJSON(ctx, http.MethodPost, atlasActivePath, map[string]string{"world": slug}, &res); err != nil {
		return "", err
	}
	return fmt.Sprintf("Активный мир: %s", res.Active), nil
}

// cmdStats — /stats: состояние героя из GET /api/state. Здоровье печатается
// одним словом семибалльной шкалы, проценты не показываются.
func (b *TelegramBot) cmdStats(ctx context.Context) (string, error) {
	slug := getActiveSlug()
	if slug == "" {
		return commandNoWorldText, nil
	}
	var st chatStateResponse
	if err := b.loopbackJSON(ctx, http.MethodGet, chatStatePath+"?world="+url.QueryEscape(slug), nil, &st); err != nil {
		return "", err
	}

	var sb strings.Builder
	title := st.WorldState["title"]
	if title == "" {
		title = st.World
	}
	sb.WriteString("Мир: " + title)
	if mode := st.WorldState["travel_mode"]; mode != "" {
		sb.WriteString(" (" + mode + ")")
	}
	sb.WriteString("\n")

	if st.Hero.HealthKnown {
		fmt.Fprintf(&sb, "Здоровье: %s\n", healthWord(st.Hero.Health))
	} else {
		sb.WriteString("Здоровье: неизвестно\n")
	}
	if st.Hero.Mood != "" {
		fmt.Fprintf(&sb, "Настроение: %s\n", st.Hero.Mood)
	}
	if st.Hero.Place != "" {
		if st.Hero.PlaceKind != "" {
			fmt.Fprintf(&sb, "Место: %s (%s)\n", st.Hero.Place, st.Hero.PlaceKind)
		} else {
			fmt.Fprintf(&sb, "Место: %s\n", st.Hero.Place)
		}
	}
	if st.System.SessionID != 0 {
		fmt.Fprintf(&sb, "Сессия: %d\n", st.System.SessionID)
	}
	if len(st.Turns) > 0 {
		last := st.Turns[len(st.Turns)-1]
		line := fmt.Sprintf("Последний ход: %d", last.TurnID)
		if st.System.StartedAt != "" {
			line += " (время: " + st.System.StartedAt + ")"
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// healthWord переводит здоровье героя (проценты) в одно слово семибалльной
// шкалы. Слой состояния отсутствует — вызывающий печатает «Здоровье: неизвестно».
func healthWord(health int) string {
	switch {
	case health >= 86:
		return "Великолепное"
	case health >= 71:
		return "Отличное"
	case health >= 56:
		return "Хорошее"
	case health >= 41:
		return "Среднее"
	case health >= 26:
		return "Не хорошее"
	case health >= 11:
		return "Плохое"
	default:
		return "Критическое"
	}
}

// cmdModel — /model: без аргумента список провайдеров с ролями, с аргументами
// /model <роль> <провайдер> — привязка роли к провайдеру.
func (b *TelegramBot) cmdModel(ctx context.Context, arg string) (string, error) {
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		return b.modelList(ctx)
	}
	if len(fields) != 2 {
		return "Использование: /model <роль> <провайдер>. Список команд — /help.", nil
	}
	return b.modelSwitch(ctx, fields[0], fields[1])
}

// modelList — /model: карточки провайдеров из GET /api/providers с пометкой,
// какие роли использует каждый провайдер (из GET /api/settings bindings).
func (b *TelegramBot) modelList(ctx context.Context) (string, error) {
	slug := getActiveSlug()
	if slug == "" {
		return commandNoWorldText, nil
	}
	var prov struct {
		Providers []providerCard `json:"providers"`
	}
	if err := b.loopbackJSON(ctx, http.MethodGet, providersPath, nil, &prov); err != nil {
		return "", err
	}
	var set struct {
		Bindings []slotBinding `json:"bindings"`
	}
	if err := b.loopbackJSON(ctx, http.MethodGet, settingsPath+"?world="+url.QueryEscape(slug), nil, &set); err != nil {
		return "", err
	}

	used := map[string][]string{}
	for _, bd := range set.Bindings {
		used[bd.Provider] = append(used[bd.Provider], bd.Role)
	}

	var sb strings.Builder
	for _, p := range prov.Providers {
		key := "ключа нет"
		if p.KeySet {
			key = "ключ есть"
		}
		line := fmt.Sprintf("%s — %s, модель %s, квота %s, стоимость %d",
			p.Name, key, p.Model, p.Quota, p.CostMillis)
		if roles := used[p.Name]; len(roles) > 0 {
			line += " (роли: " + strings.Join(roles, ", ") + ")"
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// modelSwitch — /model <роль> <провайдер>: привязка роли к провайдеру
// PUT /api/settings/bindings с model_ref провайдера из /api/providers.
// Неизвестные роль или провайдер — понятное русское сообщение, ничего не меняется.
func (b *TelegramBot) modelSwitch(ctx context.Context, role, provider string) (string, error) {
	slug := getActiveSlug()
	if slug == "" {
		return commandNoWorldText, nil
	}
	if !knownRole(role) {
		return "Не знаю такой роли: " + role + ". Список ролей — /model.", nil
	}

	var prov struct {
		Providers []providerCard `json:"providers"`
	}
	if err := b.loopbackJSON(ctx, http.MethodGet, providersPath, nil, &prov); err != nil {
		return "", err
	}
	modelRef := ""
	found := false
	for _, p := range prov.Providers {
		if p.Name == provider {
			modelRef = p.Model
			found = true
			break
		}
	}
	if !found {
		return "Не знаю такого провайдера: " + provider + ". Список — /model.", nil
	}

	body := modelSwitchRequest{
		World:    slug,
		Bindings: []slotBinding{{Role: role, ModelRef: modelRef, Provider: provider}},
	}
	if err := b.loopbackJSON(ctx, http.MethodPut, settingsBindingsPath, body, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("Роль %s → %s", role, provider), nil
}

// modelSwitchRequest — тело PUT /api/settings/bindings для /model <роль>.
type modelSwitchRequest struct {
	World    string        `json:"world"`
	Bindings []slotBinding `json:"bindings"`
}
