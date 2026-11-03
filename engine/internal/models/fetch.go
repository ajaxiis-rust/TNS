package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tns65/internal/journal"
	"tns65/internal/netgate"
)

// События журнала загрузчика моделей (MD-09, MD-08.2). Имена заданы здесь,
// потому что общий реестр journal/events.go задаче T-13-01 не принадлежит.
const (
	eventDownloadStart = "model.download.start"
	eventDownloadDone  = "model.download.done"
	eventDownloadFail  = "model.download.fail"
)

const (
	// defaultWorkers — число параллельных Range-отрезков по умолчанию.
	defaultWorkers = 4
	// maxWorkers — потолок отрезков: 4–8 соединений к CDN (решение владельца 2026-11-03).
	maxWorkers = 8
	// downloadAttempts — попыток на отрезок, включая первую.
	downloadAttempts = 3
	// defaultRetryPause — пауза перед первым повтором; далее удваивается (1/2/4 с).
	defaultRetryPause = time.Second
	// progressInterval — не чаще четырёх колбэков прогресса в секунду.
	progressInterval = 250 * time.Millisecond
)

// DownloadSpec — задание загрузчика: что качать и куда писать.
// Dest — итоговый файл; частичные отрезки живут рядом в каталоге Dest+".part".
// Client пуст — наружный клиент netgate по хосту URL; в тестах — netgate.Local.
// RetryPause пуст — 1 с; тесты уменьшают паузу.
type DownloadSpec struct {
	URL        string
	Dest       string
	Workers    int
	Client     *http.Client
	Logger     *journal.Logger
	Progress   func(DownloadProgress)
	RetryPause time.Duration
}

// DownloadProgress — снимок прогресса для UI: сколько байт скачано из всего
// и текущая скорость, МБ/с.
type DownloadProgress struct {
	Done  int64
	Total int64
	MBps  float64
}

// DownloadResult — итог загрузки: путь, размер, sha256 и служебные числа.
type DownloadResult struct {
	Path    string
	Size    int64
	SHA256  string
	Workers int
	Retries int
	MBps    float64
}

// segment — отрезок файла в байтах, end включительный; end < 0 — размер
// неизвестен (сервер не дал ни Range, ни Content-Length), качаем потоком.
type segment struct {
	index int
	start int64
	end   int64
}

func (s segment) length() int64 { return s.end - s.start + 1 }

// probe — итог пробы докачки: размер файла и поддержка Range.
type probe struct {
	total  int64
	ranges bool
}

// fatalErr — ошибка, которую повторять бессмысленно (4xx, сервер игнорирует
// Range): отрезок сразу сдаётся вместо трёх попыток.
type fatalErr struct{ err error }

func (e fatalErr) Error() string { return e.err.Error() }
func (e fatalErr) Unwrap() error { return e.err }

// DownloadFile скачивает URL в Dest параллельными Range-отрезками (MD-09,
// T-13-01): проба Range, отрезки по файлам Dest+".part/<i>", докачка от размера
// частичного файла, слияние с потоковым sha256, ретраи 3 попытки на отрезок и
// события model.download.start|done|fail. RAM не копится: байты идут в файлы.
func DownloadFile(ctx context.Context, spec DownloadSpec) (DownloadResult, error) {
	rawURL := strings.TrimSpace(spec.URL)
	if rawURL == "" {
		return DownloadResult{}, errors.New("models: загрузка: пустой url")
	}
	dest := strings.TrimSpace(spec.Dest)
	if dest == "" {
		return DownloadResult{}, errors.New("models: загрузка: пустой путь назначения")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return DownloadResult{}, fmt.Errorf("models: загрузка: url %q", rawURL)
	}
	client := spec.Client
	if client == nil {
		client = netgate.Client(u.Host)
	}
	retryPause := spec.RetryPause
	if retryPause <= 0 {
		retryPause = defaultRetryPause
	}

	fail := func(err error) (DownloadResult, error) {
		emitEvent(spec.Logger, eventDownloadFail,
			journal.Field{Key: "url", Value: rawURL},
			journal.Field{Key: "cdn_host", Value: u.Host},
			journal.Field{Key: "error", Value: err.Error()},
		)
		return DownloadResult{}, err
	}

	pr, err := probeDownload(ctx, client, rawURL)
	if err != nil {
		return fail(err)
	}
	workers := spec.Workers
	if workers <= 0 {
		workers = defaultWorkers
	}
	if workers > maxWorkers {
		workers = maxWorkers
	}
	segs := splitSegments(pr.total, workers)
	if !pr.ranges {
		segs = []segment{{index: 0, start: 0, end: pr.total - 1}}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fail(fmt.Errorf("models: загрузка: каталог назначения: %w", err))
	}
	partDir := dest + ".part"
	if err := os.MkdirAll(partDir, 0o755); err != nil {
		return fail(fmt.Errorf("models: загрузка: каталог отрезков: %w", err))
	}

	tracker := newProgressTracker(pr.total, spec.Progress)
	dl := &downloader{
		client:     client,
		url:        rawURL,
		partDir:    partDir,
		ranged:     pr.ranges,
		total:      pr.total,
		retryPause: retryPause,
		progress:   tracker,
	}
	emitEvent(spec.Logger, eventDownloadStart,
		journal.Field{Key: "url", Value: rawURL},
		journal.Field{Key: "cdn_host", Value: u.Host},
		journal.Field{Key: "bytes", Value: strconv.FormatInt(pr.total, 10)},
		journal.Field{Key: "workers", Value: strconv.Itoa(len(segs))},
		journal.Field{Key: "resume", Value: strconv.FormatBool(pr.ranges)},
	)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, len(segs))
	for _, seg := range segs {
		wg.Add(1)
		go func(seg segment) {
			defer wg.Done()
			if err := dl.segment(runCtx, seg); err != nil {
				errCh <- fmt.Errorf("models: отрезок %d/%d: %w", seg.index+1, len(segs), err)
				cancel()
			}
		}(seg)
	}
	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return fail(err)
	}

	sha, size, err := mergeParts(dest, partDir, segs)
	if err != nil {
		return fail(err)
	}
	if err := os.RemoveAll(partDir); err != nil {
		return fail(fmt.Errorf("models: загрузка: удаление отрезков: %w", err))
	}
	mbps := tracker.rate()
	tracker.finish()
	emitEvent(spec.Logger, eventDownloadDone,
		journal.Field{Key: "url", Value: rawURL},
		journal.Field{Key: "cdn_host", Value: u.Host},
		journal.Field{Key: "bytes", Value: strconv.FormatInt(size, 10)},
		journal.Field{Key: "mbps", Value: fmt.Sprintf("%.2f", mbps)},
		journal.Field{Key: "workers", Value: strconv.Itoa(len(segs))},
		journal.Field{Key: "retries", Value: strconv.Itoa(int(dl.retries.Load()))},
	)
	return DownloadResult{
		Path:    dest,
		Size:    size,
		SHA256:  sha,
		Workers: len(segs),
		Retries: int(dl.retries.Load()),
		MBps:    mbps,
	}, nil
}

// probeDownload пробует докачку запросом Range: bytes=0-0 и разбирает
// Accept-Ranges/Content-Range; без Range размер берётся из Content-Length.
func probeDownload(ctx context.Context, client *http.Client, rawURL string) (probe, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return probe{}, fmt.Errorf("models: загрузка: запрос пробы: %w", err)
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(req)
	if err != nil {
		return probe{}, fmt.Errorf("models: загрузка: проба Range: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))

	switch resp.StatusCode {
	case http.StatusPartialContent:
		_, _, total, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil || total <= 0 {
			return probe{}, fmt.Errorf("models: загрузка: Content-Range %q", resp.Header.Get("Content-Range"))
		}
		return probe{total: total, ranges: true}, nil
	case http.StatusOK:
		if resp.ContentLength <= 0 {
			return probe{total: 0, ranges: false}, nil
		}
		return probe{total: resp.ContentLength, ranges: false}, nil
	default:
		return probe{}, fmt.Errorf("models: загрузка: проба Range: статус %d", resp.StatusCode)
	}
}

// parseContentRange разбирает "bytes start-end/total"; total < 0 — "*".
func parseContentRange(v string) (start, end, total int64, err error) {
	if !strings.HasPrefix(v, "bytes ") {
		return 0, 0, 0, fmt.Errorf("models: формат Content-Range %q", v)
	}
	rest := strings.TrimPrefix(v, "bytes ")
	dash := strings.Index(rest, "-")
	slash := strings.Index(rest, "/")
	if dash <= 0 || slash <= dash {
		return 0, 0, 0, fmt.Errorf("models: формат Content-Range %q", v)
	}
	start, err = strconv.ParseInt(strings.TrimSpace(rest[:dash]), 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("models: формат Content-Range %q", v)
	}
	end, err = strconv.ParseInt(strings.TrimSpace(rest[dash+1:slash]), 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("models: формат Content-Range %q", v)
	}
	total = -1
	if s := strings.TrimSpace(rest[slash+1:]); s != "*" {
		total, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("models: формат Content-Range %q", v)
		}
	}
	return start, end, total, nil
}

// splitSegments делит файл на workers отрезков; размер неизвестен — один поток.
func splitSegments(total int64, workers int) []segment {
	if total <= 0 {
		return []segment{{index: 0, start: 0, end: -1}}
	}
	if workers < 1 {
		workers = 1
	}
	if int64(workers) > total {
		workers = int(total)
	}
	base := total / int64(workers)
	rem := total % int64(workers)
	segs := make([]segment, 0, workers)
	var start int64
	for i := 0; i < workers; i++ {
		n := base
		if int64(i) < rem {
			n++
		}
		segs = append(segs, segment{index: i, start: start, end: start + n - 1})
		start += n
	}
	return segs
}

// downloader — состояние одной загрузки: клиент, каталог отрезков, прогресс.
type downloader struct {
	client     *http.Client
	url        string
	partDir    string
	ranged     bool
	total      int64
	retryPause time.Duration
	progress   *progressTracker
	retries    atomic.Int64
}

// segment качает один отрезок в свой частичный файл. Докачка идёт от текущего
// размера частичного файла (только при поддержке Range), всего 3 попытки с
// паузой 1/2/4 с, после исчерпания — ошибка.
func (d *downloader) segment(ctx context.Context, seg segment) error {
	path := filepath.Join(d.partDir, strconv.Itoa(seg.index))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("models: частичный файл: %w", err)
	}
	defer f.Close()

	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		if attempt > 1 {
			d.retries.Add(1)
			if err := sleepCtx(ctx, d.retryPause<<uint(attempt-2)); err != nil {
				return err
			}
		}
		offset, err := d.syncFile(f, seg)
		if err != nil {
			return err
		}
		if seg.end >= 0 && offset >= seg.length() {
			return nil
		}
		err = d.attempt(ctx, f, seg, offset)
		if err == nil {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var fatal fatalErr
		if errors.As(err, &fatal) {
			return err
		}
	}
	return lastErr
}

// syncFile ставит файл на позицию продолжения: при поддержке Range — размер
// частичного файла, иначе отрезок начинается заново (докачки нет).
func (d *downloader) syncFile(f *os.File, seg segment) (int64, error) {
	offset := int64(0)
	if d.ranged {
		st, err := f.Stat()
		if err != nil {
			return 0, fmt.Errorf("models: размер отрезка: %w", err)
		}
		offset = st.Size()
		if seg.end >= 0 && offset > seg.length() {
			offset = seg.length()
		}
	}
	if err := f.Truncate(offset); err != nil {
		return 0, fmt.Errorf("models: обрезка отрезка: %w", err)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("models: позиция отрезка: %w", err)
	}
	return offset, nil
}

// attempt — одна попытка отрезка: GET с Range от offset, потоковая запись.
func (d *downloader) attempt(ctx context.Context, f *os.File, seg segment, offset int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return fatalErr{fmt.Errorf("models: запрос отрезка: %w", err)}
	}
	remaining := int64(-1)
	if seg.end >= 0 {
		remaining = seg.length() - offset
		if remaining <= 0 {
			return nil
		}
		if d.ranged {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", seg.start+offset, seg.end))
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("models: запрос отрезка: %w", err)
	}
	defer resp.Body.Close()

	want := http.StatusOK
	if d.ranged {
		want = http.StatusPartialContent
	}
	if resp.StatusCode != want {
		err := fmt.Errorf("models: отрезок: статус %d", resp.StatusCode)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fatalErr{err}
		}
		return err
	}
	if d.ranged {
		start, _, total, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return fmt.Errorf("models: отрезок: %w", err)
		}
		if start != seg.start+offset {
			return fatalErr{fmt.Errorf("models: отрезок: начало %d вместо %d", start, seg.start+offset)}
		}
		if d.total > 0 && total >= 0 && total != d.total {
			return fatalErr{fmt.Errorf("models: отрезок: размер файла %d вместо %d", total, d.total)}
		}
	}

	var src io.Reader = resp.Body
	if remaining > 0 {
		src = io.LimitReader(resp.Body, remaining)
	}
	n, err := io.Copy(&countWriter{w: f, progress: d.progress}, src)
	if syncErr := f.Sync(); syncErr != nil && err == nil {
		err = syncErr
	}
	if err != nil {
		return fmt.Errorf("models: чтение отрезка: %w", err)
	}
	if remaining > 0 && n < remaining {
		return fmt.Errorf("models: отрезок оборван: %d из %d байт", n, remaining)
	}
	return nil
}

// countWriter пишет байты в файл и двигает общий атомарный счётчик прогресса.
type countWriter struct {
	w        io.Writer
	progress *progressTracker
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 && c.progress != nil {
		c.progress.add(int64(n))
	}
	return n, err
}

// progressTracker — атомарный счётчик байт и колбэк не чаще четырёх раз в секунду.
type progressTracker struct {
	done  atomic.Int64
	total int64
	start time.Time
	mu    sync.Mutex
	last  time.Time
	fn    func(DownloadProgress)
}

func newProgressTracker(total int64, fn func(DownloadProgress)) *progressTracker {
	now := time.Now()
	return &progressTracker{total: total, start: now, last: now, fn: fn}
}

func (p *progressTracker) add(n int64) {
	d := p.done.Add(n)
	if p.fn == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	if now.Sub(p.last) < progressInterval {
		p.mu.Unlock()
		return
	}
	p.last = now
	p.mu.Unlock()
	p.fn(DownloadProgress{Done: d, Total: p.total, MBps: rateMBps(d, now.Sub(p.start))})
}

// rate — средняя скорость всей загрузки, МБ/с.
func (p *progressTracker) rate() float64 {
	return rateMBps(p.done.Load(), time.Since(p.start))
}

// finish — финальный колбэк с полным счётчиком байт.
func (p *progressTracker) finish() {
	if p.fn == nil {
		return
	}
	p.fn(DownloadProgress{Done: p.done.Load(), Total: p.total, MBps: p.rate()})
}

// rateMBps — скорость за прошедшее время, МБ/с.
func rateMBps(bytes int64, elapsed time.Duration) float64 {
	if elapsed <= 0 || bytes <= 0 {
		return 0
	}
	return float64(bytes) / elapsed.Seconds() / (1 << 20)
}

// mergeParts сливает частичные файлы в Dest одним проходом, считая sha256;
// частичный файл отрезка обязан быть ровно его длины.
func mergeParts(dest, partDir string, segs []segment) (string, int64, error) {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", 0, fmt.Errorf("models: слияние: %w", err)
	}
	h := sha256.New()
	w := io.MultiWriter(out, h)
	var size int64
	for _, seg := range segs {
		path := filepath.Join(partDir, strconv.Itoa(seg.index))
		f, err := os.Open(path)
		if err != nil {
			_ = out.Close()
			return "", 0, fmt.Errorf("models: слияние: %w", err)
		}
		n, copyErr := io.Copy(w, f)
		closeErr := f.Close()
		if copyErr != nil {
			_ = out.Close()
			return "", 0, fmt.Errorf("models: слияние: %w", copyErr)
		}
		if closeErr != nil {
			_ = out.Close()
			return "", 0, fmt.Errorf("models: слияние: %w", closeErr)
		}
		if seg.end >= 0 && n != seg.length() {
			_ = out.Close()
			return "", 0, fmt.Errorf("models: слияние: отрезок %d: %d из %d байт", seg.index, n, seg.length())
		}
		size += n
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return "", 0, fmt.Errorf("models: слияние: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", 0, fmt.Errorf("models: слияние: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

// sleepCtx — пауза между попытками с уважением к отмене контекста.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// emitEvent пишет событие журнала движка, если журнал задан.
func emitEvent(l *journal.Logger, event string, fields ...journal.Field) {
	if l == nil {
		return
	}
	_ = l.Event("models", event, fields...)
}
