// TestVerne — симуляционный harness приёмки (T-19-11). Файл без тега e2e:
// проверка задачи гоняет `go test ./e2e/ -run TestVerne` без тега, а сценарии
// INT-* под тегом e2e собираются отдельно. Харнесс самодостаточен: бинарник
// ядра собирается с тегом e2e (хуки --fake-*), helpers здесь свои, с префиксом
// verne, чтобы не конфликтовать с harness_test.go при сборке под тегом.

package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"tns65/internal/db"
	"tns65/internal/netgate"
	"tns65/internal/turn"
)

// Пороги метрик harness (шаг 2): прогон зелёный, пока метрики в допуске.
// Пороги — константы harness, а не настройка движка.
const (
	verneTurns               = 10   // N ходов прогона: по абзацу книжки
	verneRefuseEvery         = 3    // каждый третий вызов слота отвечает отказом
	verneMaxSlotRefusalShare = 0.5  // доля отказов слота в вызовах
	verneMinWeaverCalls      = 3    // слот прозы обязан участвовать в прогоне
	verneMinTonalityShare    = 0.3  // попадания детектора тональности
	verneMinSarcasmHits      = 1    // попадания детектора сарказма
	verneMinEroticaHits      = 1    // попадания детектора эротики
	verneMinMemoryShare      = 0.75 // связность по памяти NPC
	verneMinReinforce        = 2    // повторные встречи одного лица в памяти
)

// vernePortAttempts — сколько раз harness поднимает ядро на новом свободном
// порту: порт занят чужим процессом — старт повторяется (T-21-02). Фиксированного
// порта у harness нет: ядро слушает свободный порт прогона.
const vernePortAttempts = 3

// verneNarrator, verneCompanion — имена лиц мира, упоминаемых в книжке: труппа
// узнаёт имя в реплике и пишет встречу в память реакций (memory_topics).
const (
	verneNarrator  = "Жюль"
	verneCompanion = "Консель"
)

// verneSlotProse — штатная проза подставного слота: повествование без прямой
// речи, чтобы словарный гейт табу не отбрасывал ответ.
const verneSlotProse = "Ветер трогает паруса, и палуба отвечает тихим скрипом. Мир держит ответ, пока ты идёшь вперёд."

// verneBook — книжка-фикстура harness (шаг 1): десять абзацев связной прозы
// приключенческого романа. Ими подаётся вход игрока вместо живого ввода; в
// абзацах есть тональность, сарказм, эротика и повторные упоминания лиц.
// Каждый абзац называет движение формами словаря намерений пакета (идти, плыть,
// ехать, бежать): смысл реплики подтверждается двухголосием, и ход коммитится.
var verneBook = []string{
	"Мы вышли из порта на рассвете, и берег растворился в серой дымке. Я иду по палубе, и мне грустно покидать дом: нам ещё плыть на юг, а после — ехать и бежать по чужой земле.",
	"К полудню ветер усилился, и палуба ходила под ногами. Мне было страшно, и я бегу к мачте, чтобы не идти по мокрым доскам; капитан велел плыть дальше и не ехать берегом.",
	"Ну да, конечно, капитан знал эти воды лучше всех — как же, тридцать лет плаваний! Он мог идти, плыть, ехать и бежать в любую погоду, и ни одна ошибка его не пугала.",
	"В трюме мы нашли ящик с припасами и старую карту. Жюль развернул её на бочке и провёл пальцем по берегу: нам предстояло плыть до устья, идти вверх по реке, ехать через перевал и бежать от погони.",
	"Ночью я не мог уснуть: за бортом плескалось что-то тяжёлое, и тревога не отпускала меня до утра. К рассвету надо было идти на палубу, плыть на юг и ехать на восток, а потом бежать, если ветер переменится.",
	"К утру шторм стих. Солнце вышло из-за туч, и утро было отлично: паруса сохли. Нам оставалось плыть день, идти ночь и ехать до первой станции; бежать больше не было нужды.",
	"— Ты боишься, — сказал Консель и положил мне руку на плечо. — Это пройдёт, когда увидишь землю. Нам ещё идти и плыть, а там можно будет ехать или бежать, как велит дорога.",
	"Я обнимаю её на прощание, и в этом объятье — нежность и сдержанная страсть, о которой не говорят вслух. Потом пора идти к лодке, и я обещаю плыть, ехать и бежать, пока не вернусь.",
	"Перед нами открылся берег: скалы, пальмы и белый песок. Уф, как же здесь душно после моря. Идти до жилья далеко, плыть вдоль скал долго, а ехать не на чем; остаётся бежать по тропе.",
	"Жюль вёл нас вверх по склону и говорил о том, как много значит для него эта земля. «Отсюда легко идти, плыть и ехать, — повторял он, — а бежать отсюда никто не станет».",
}

// verneTurnRow — строка turns прогона: вход игрока, ответ мира, путь и признак
// отказа слота.
type verneTurnRow struct {
	PlayerText string
	ReplyText  string
	Path       string
	Degraded   int
}

// TestVerne — T-19-11. Мир с фейковыми слотами, реплики игрока — абзацы книжки
// (шаг 1), метрики прогона (шаг 2): доля отказов слота, попадания словарных
// детекторов тональности/сарказма/эротики, связность по памяти NPC. Отчёт идёт
// в лог теста и в reports/verne.json (шаг 3); прогон зелёный при метриках в
// допуске.
func TestVerne(t *testing.T) {
	bin := verneBuild(t)
	dir := verneWorldDir(t)

	// Подставной слот прозы: каждый третий вызов отвечает отказом (HTTP 503),
	// прочие — штатной прозой. Счётчики — метрика отказов слота.
	var slotCalls, slotRefusals int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&slotCalls, 1)%verneRefuseEvery == 0 {
			atomic.AddInt64(&slotRefusals, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(verneSlotProse))
	}))
	defer srv.Close()

	slug := "test-verna"
	verneBirth(t, dir, bin, slug)
	worldDB := filepath.Join(dir, "world_db", "worlds", slug, slug+".db")

	// Лица мира получают имена: после рождения characters.name пуст, а труппа
	// узнаёт в реплике только непустое имя.
	verneNameFaces(t, worldDB, verneNarrator, verneCompanion)

	c := verneStartCore(t, dir, bin, srv.URL)
	wdb := verneOpenDB(t, worldDB)
	defer wdb.Close()

	lex, err := turn.LoadLexicon(filepath.Join(verneEngineRoot(t), "packs", "ru"))
	if err != nil {
		t.Fatalf("лексикон пакета ru: %v", err)
	}
	sarcasmHits, eroticaHits := verneDetectorHits(lex)

	// Шаг 1: прогон N ходов абзацами книжки; настроение героя читается после
	// каждого хода — это исход детектора тональности предпути.
	moods := make([]string, 0, verneTurns)
	for i, paragraph := range verneBook {
		c.postTurn(t, slug, paragraph)
		mood := verneHeroMood(t, wdb)
		moods = append(moods, mood)
		t.Logf("verne ход %02d: настроение=%q вход=%q", i+1, mood, paragraph)
	}

	turns := verneTurnsFromDB(t, wdb)
	if len(turns) != verneTurns {
		t.Fatalf("ходов в turns = %d, want %d", len(turns), verneTurns)
	}

	slotCallCount := atomic.LoadInt64(&slotCalls)
	refusalCount := int(atomic.LoadInt64(&slotRefusals))
	slotRefusalShare := 0.0
	if slotCallCount > 0 {
		slotRefusalShare = float64(refusalCount) / float64(slotCallCount)
	}

	moodTurns, degradedTurns := 0, 0
	for i, tr := range turns {
		if strings.TrimSpace(moods[i]) != "" {
			moodTurns++
		}
		if tr.Degraded != 0 {
			degradedTurns++
		}
		t.Logf("verne чат %02d: путь=%s деградация=%d мир=%q", i+1, tr.Path, tr.Degraded, tr.ReplyText)
	}
	tonalityShare := float64(moodTurns) / float64(verneTurns)

	memory := verneMemory(t, wdb)
	memoryRows, memoryReinforce := 0, 0
	for _, n := range memory {
		memoryRows += n
		if n > memoryReinforce {
			memoryReinforce = n
		}
	}
	mentionTurns := verneMentionTurns(verneNarrator, verneCompanion)
	memoryShare := 0.0
	if mentionTurns > 0 {
		memoryShare = float64(memoryRows) / float64(mentionTurns)
	}

	report := map[string]any{
		"turns":              len(turns),
		"slot_calls":         slotCallCount,
		"slot_refusals":      refusalCount,
		"slot_refusal_share": slotRefusalShare,
		"degraded_turns":     degradedTurns,
		"tonality_share":     tonalityShare,
		"tonality_hits":      moodTurns,
		"sarcasm_hits":       sarcasmHits,
		"erotica_hits":       eroticaHits,
		"memory_share":       memoryShare,
		"memory_rows":        memoryRows,
		"memory_reinforce":   memoryReinforce,
		"thresholds": map[string]any{
			"slot_refusal_share": verneMaxSlotRefusalShare,
			"weaver_calls":       verneMinWeaverCalls,
			"tonality_share":     verneMinTonalityShare,
			"sarcasm_hits":       verneMinSarcasmHits,
			"erotica_hits":       verneMinEroticaHits,
			"memory_share":       verneMinMemoryShare,
			"memory_reinforce":   verneMinReinforce,
		},
	}
	verneWriteReport(t, report)
	t.Logf("verne отчёт: ходов=%d вызовов_слота=%d отказов=%d доля_отказов=%.2f деградаций=%d тональность=%.2f сарказм=%d эротика=%d память=%.2f повторов_памяти=%d",
		len(turns), slotCallCount, refusalCount, slotRefusalShare, degradedTurns,
		tonalityShare, sarcasmHits, eroticaHits, memoryShare, memoryReinforce)

	// Ход доставлен всегда: книга подана вместо игрока, ответ мира непуст, отказ
	// слота доставлен запаской.
	for i, tr := range turns {
		if tr.PlayerText != verneBook[i] {
			t.Fatalf("ход %d: вход %q, want абзац книжки %q", i+1, tr.PlayerText, verneBook[i])
		}
		if strings.TrimSpace(tr.ReplyText) == "" {
			t.Fatalf("ход %d: пустой ответ мира", i+1)
		}
		if tr.Degraded != 0 && tr.Path != "fallback" {
			t.Fatalf("ход %d: деградация с путём %q, want fallback", i+1, tr.Path)
		}
	}

	if slotCallCount < verneMinWeaverCalls {
		t.Fatalf("вызовов слота = %d, want ≥ %d: метрики слотов не на чем считать", slotCallCount, verneMinWeaverCalls)
	}
	if refusalCount < 1 {
		t.Fatal("отказов слота 0: подставной слот не отказал ни разу")
	}
	if slotRefusalShare > verneMaxSlotRefusalShare {
		t.Fatalf("доля отказов слота = %.2f, want ≤ %.2f", slotRefusalShare, verneMaxSlotRefusalShare)
	}
	if degradedTurns < 1 {
		t.Fatalf("деградаций хода 0 при отказах слота %d: отказ не доставлен запаской", refusalCount)
	}
	if tonalityShare < verneMinTonalityShare {
		t.Fatalf("попадания тональности = %.2f, want ≥ %.2f", tonalityShare, verneMinTonalityShare)
	}
	if sarcasmHits < verneMinSarcasmHits {
		t.Fatalf("попадания сарказма = %d, want ≥ %d", sarcasmHits, verneMinSarcasmHits)
	}
	if eroticaHits < verneMinEroticaHits {
		t.Fatalf("попадания эротики = %d, want ≥ %d", eroticaHits, verneMinEroticaHits)
	}
	if memoryShare < verneMinMemoryShare {
		t.Fatalf("связность по памяти NPC = %.2f, want ≥ %.2f", memoryShare, verneMinMemoryShare)
	}
	if memoryReinforce < verneMinReinforce {
		t.Fatalf("повторных встреч в памяти = %d, want ≥ %d", memoryReinforce, verneMinReinforce)
	}
}

// verneEngineRoot — корень движка: тест исполняется в каталоге engine/e2e.
func verneEngineRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("корень движка: %v", err)
	}
	return root
}

// verneBuild собирает ./cmd/tns65 с тегом e2e — только такая сборка принимает
// хуки --fake-clock/--fake-embed/--fake-slot.
func verneBuild(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "tns65")
	cmd := exec.Command("go", "build", "-tags", "e2e", "-o", out, "./cmd/tns65")
	cmd.Dir = verneEngineRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if buf, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build -tags e2e: %v\n%s", err, buf)
	}
	return out
}

// verneWorldDir готовит рабочий каталог прогона: базы, газетир, пакет и конфиг
// видны по прежним путям (конфиг — копия, в него пишется порт прогона), world_db/
// рождается во временном каталоге.
func verneWorldDir(t *testing.T) string {
	t.Helper()
	root := verneEngineRoot(t)
	dir := t.TempDir()
	for _, name := range []string{"base", "seed", "packs"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s отсутствует в %s: %v", name, root, err)
		}
		if err := os.Symlink(filepath.Join(root, name), filepath.Join(dir, name)); err != nil {
			t.Fatalf("symlink %s: %v", name, err)
		}
	}
	// config.yaml копируется, а не связывается: harness прописывает в секцию web
	// свободный порт прогона (T-21-02), правка симлинка изменила бы конфиг движка.
	raw, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("config.yaml в %s: %v", root, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), raw, 0o644); err != nil {
		t.Fatalf("config.yaml прогона: %v", err)
	}
	return dir
}

// verneRun запускает бинарник в каталоге dir и возвращает stdout, stderr и код.
func verneRun(t *testing.T, dir, bin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", append([]string{bin}, args...), err)
		}
		code = ee.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

// verneBirth рождает мир по анкете из 14 полей с фейковыми эмбеддером и слотом.
func verneBirth(t *testing.T, dir, bin, slug string) {
	t.Helper()
	ancketa := map[string]any{
		"slug":         slug,
		"title":        "Тёмный мир",
		"description":  "Мир, где свет угас навсегда.",
		"synopsis":     "Герой приходит в мир без света. Он ищет источник тьмы. Финал решает всё.",
		"prologue":     "Всё началось с тишины.",
		"lang":         "ru",
		"travel_mode":  "вымышленная",
		"genres":       []string{"фэнтези", "хоррор"},
		"world_rules":  []string{"магия: редкая и опасная"},
		"social_order": "феодализм",
		"economy_mods": []string{"бартер"},
		"magic":        "редкая",
		"hero_birth":   "простолюдин",
		"censorship":   "16+",
	}
	raw, err := json.MarshalIndent(ancketa, "", "  ")
	if err != nil {
		t.Fatalf("анкета: %v", err)
	}
	path := filepath.Join(t.TempDir(), slug+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("анкета: %v", err)
	}
	stdout, stderr, code := verneRun(t, dir, bin, "world", "birth", "--ancketa", path, "--fake-embed", "--fake-slot")
	if code != 0 {
		t.Fatalf("world birth exit=%d, want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
}

// verneCore — запущенный процесс ядра.
type verneCore struct {
	cmd     *exec.Cmd
	log     *os.File
	url     string
	client  *http.Client
	exited  chan struct{} // закрыт после cmd.Wait: процесс ядра завершился
	waitErr error         // итог cmd.Wait; читается после закрытия exited
	logPath string
}

// verneStartCore поднимает ядро в dir с фейковыми часами и адресом подставного
// слота и ждёт /api/health. Порт — свободный порт прогона (T-21-02): фиксированный
// 8720 занимал чужой процесс, и harness принимал его ответы за ядро прогона.
func verneStartCore(t *testing.T, dir, bin, slotURL string) *verneCore {
	t.Helper()
	var lastErr error
	for attempt := 1; attempt <= vernePortAttempts; attempt++ {
		port := verneFreePort(t)
		verneSetWebPort(t, dir, port)
		c, err := verneLaunchCore(t, dir, bin, slotURL, port)
		if err == nil {
			t.Cleanup(c.stop)
			return c
		}
		lastErr = err
		t.Logf("verne: старт ядра на порту %d (%d/%d): %v", port, attempt, vernePortAttempts, err)
	}
	t.Fatalf("ядро не поднялось ни на одном свободном порту за %d попыток: %v", vernePortAttempts, lastErr)
	return nil
}

// verneLaunchCore запускает ядро на порту port и ждёт /api/health. Выход процесса
// до готовности — ошибка с журналом ядра: порт перехвачен или конфиг не прочитан.
func verneLaunchCore(t *testing.T, dir, bin, slotURL string, port int) (*verneCore, error) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "core.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("лог ядра: %w", err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cmd := exec.Command(bin, "--fake-clock", "--fake-slot", slotURL)
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start core: %w", err)
	}
	c := &verneCore{
		cmd:     cmd,
		log:     logFile,
		url:     "http://" + addr,
		client:  netgate.Local(addr),
		exited:  make(chan struct{}),
		logPath: logPath,
	}
	go func() {
		c.waitErr = c.cmd.Wait()
		close(c.exited)
	}()
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-c.exited:
			c.closeLog()
			return nil, fmt.Errorf("ядро на %s завершилось (код %v) до готовности\n%s", addr, c.waitErr, verneLogTail(logPath))
		default:
		}
		resp, err := c.client.Get(c.url + "/api/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// Чужой процесс на этом порту отвечает сразу: свой обязан быть жив
				// и после короткой выдержки — иначе старт повторяется на новом порту.
				time.Sleep(100 * time.Millisecond)
				select {
				case <-c.exited:
					c.closeLog()
					return nil, fmt.Errorf("ядро на %s завершилось после /api/health: порт занят чужим процессом", addr)
				default:
				}
				return c, nil
			}
		}
		if time.Now().After(deadline) {
			_ = c.cmd.Process.Kill()
			<-c.exited
			c.closeLog()
			return nil, fmt.Errorf("ядро не поднялось на %s\n%s", addr, verneLogTail(logPath))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// verneFreePort берёт свободный порт loopback: пробный слушатель на :0 сразу
// закрывается, ядро биндится на занятый им порт.
func verneFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("свободный порт ядра: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("закрыть пробный слушатель: %v", err)
	}
	return port
}

// verneSetWebPort прописывает порт в секцию web копии config.yaml рабочего
// каталога: ядро читает конфиг из своего cwd.
func verneSetWebPort(t *testing.T, dir string, port int) {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config.yaml прогона: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	inWeb, done := false, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if line == trimmed {
			inWeb = trimmed == "web:"
			continue
		}
		if inWeb && strings.HasPrefix(trimmed, "port:") {
			lines[i] = "  port: " + strconv.Itoa(port)
			done = true
		}
	}
	if !done {
		t.Fatalf("config.yaml: секция web без поля port")
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("config.yaml прогона: %v", err)
	}
}

// verneLogTail возвращает хвост журнала ядра для сообщения об отказе старта.
func verneLogTail(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	const limit = 4000
	if len(raw) > limit {
		raw = raw[len(raw)-limit:]
	}
	return strings.TrimSpace(string(raw))
}

// postTurn подаёт реплику игрока в ядро.
func (c *verneCore) postTurn(t *testing.T, world, text string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"world": world, "text": text})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.client.Post(c.url+"/api/turn", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post turn: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post turn status %d", resp.StatusCode)
	}
}

// stop гасит ядро по SIGTERM и ждёт завершения процесса.
func (c *verneCore) stop() {
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	<-c.exited
	c.closeLog()
}

// closeLog закрывает журнал ядра ровно один раз.
func (c *verneCore) closeLog() {
	if c.log != nil {
		_ = c.log.Close()
		c.log = nil
	}
}

// verneOpenDB открывает оперативную базу мира на чтение.
func verneOpenDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	conn, err := db.OpenReader(path)
	if err != nil {
		t.Fatalf("открыть базу мира %s: %v", path, err)
	}
	return conn
}

// verneNameFaces даёт лицам мира имена: пустое имя труппа в реплике не узнаёт,
// и память реакций остаётся пустой.
func verneNameFaces(t *testing.T, worldDB string, names ...string) {
	t.Helper()
	h, err := db.OpenWriterAs(worldDB, db.DBWorld, db.WriterCore, nil)
	if err != nil {
		t.Fatalf("открыть базу мира для имён: %v", err)
	}
	defer h.Close()
	for i, name := range names {
		var id int64
		if err := h.QueryRow(`SELECT id FROM characters ORDER BY id LIMIT 1 OFFSET ?`, i).Scan(&id); err != nil {
			t.Fatalf("лицо %d: %v", i, err)
		}
		if _, err := h.Exec("characters", `UPDATE characters SET name = ? WHERE id = ?`, name, id); err != nil {
			t.Fatalf("имя %q: %v", name, err)
		}
	}
}

// verneHeroMood — настроение героя из character_state: предпуть пишет его строкой
// героя (character_id = 0) на каждом ходе; пусто — реплика нейтральна.
func verneHeroMood(t *testing.T, wdb *sql.DB) string {
	t.Helper()
	var mood sql.NullString
	err := wdb.QueryRow(`SELECT mood FROM character_state WHERE character_id = 0`).Scan(&mood)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		t.Fatalf("настроение героя: %v", err)
	}
	return mood.String
}

// verneTurnsFromDB возвращает строки ходов прогона в хронологическом порядке.
func verneTurnsFromDB(t *testing.T, wdb *sql.DB) []verneTurnRow {
	t.Helper()
	rows, err := wdb.Query(`SELECT player_text, reply_text, path, degraded FROM turns ORDER BY id`)
	if err != nil {
		t.Fatalf("ходы прогона: %v", err)
	}
	defer rows.Close()
	var out []verneTurnRow
	for rows.Next() {
		var tr verneTurnRow
		if err := rows.Scan(&tr.PlayerText, &tr.ReplyText, &tr.Path, &tr.Degraded); err != nil {
			t.Fatalf("строка хода: %v", err)
		}
		out = append(out, tr)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("ходы прогона: %v", err)
	}
	return out
}

// verneMemory — память реакций по лицам: сколько встреч помнит каждое лицо.
func verneMemory(t *testing.T, wdb *sql.DB) map[int64]int {
	t.Helper()
	rows, err := wdb.Query(`SELECT character_id, COUNT(*) FROM memory_topics GROUP BY character_id`)
	if err != nil {
		t.Fatalf("память NPC: %v", err)
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatalf("строка памяти: %v", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("память NPC: %v", err)
	}
	return out
}

// verneMentionTurns считает абзацы книжки, где названо хотя бы одно лицо: по
// одному упоминанию на ход пишется одна запись памяти.
func verneMentionTurns(names ...string) int {
	n := 0
	for _, paragraph := range verneBook {
		lower := strings.ToLower(paragraph)
		for _, name := range names {
			if strings.Contains(lower, strings.ToLower(name)) {
				n++
				break
			}
		}
	}
	return n
}

// verneDetectorHits считает попадания словарных детекторов пакета по репликам
// книжки: сарказм и эротика (тональность читается из ходов, см. verneHeroMood).
func verneDetectorHits(lex *turn.Lexicon) (sarcasm, erotica int) {
	for _, paragraph := range verneBook {
		if lex.DetectSarcasm(paragraph).Sarcastic {
			sarcasm++
		}
		if lex.DetectFlirt(paragraph).Erotica {
			erotica++
		}
	}
	return sarcasm, erotica
}

// verneWriteReport пишет отчёт прогона в engine/e2e/reports/verne.json.
func verneWriteReport(t *testing.T, fields map[string]any) {
	t.Helper()
	dir := filepath.Join(verneEngineRoot(t), "e2e", "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir reports: %v", err)
	}
	raw, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatalf("отчёт: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(dir, "verne.json"), raw, 0o644); err != nil {
		t.Fatalf("отчёт: %v", err)
	}
}
