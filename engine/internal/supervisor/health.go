package supervisor

import (
	"fmt"
	"io"

	"tns65/internal/netgate"
)

// ReadyFunc проверяет готовность ребёнка. Реальная — health-запрос к
// 127.0.0.1:<port><health_path> через netgate.Local; тесты подменяют функцией
// на подставных часах.
type ReadyFunc func(spec Config) error

// defaultReady — health-запрос loopback-резидента (шаг 2). Ребёнок без порта или
// пути готов сразу: stdio-резидентов (лемматизатор) health-эндпоинт не касается.
func defaultReady(spec Config) error {
	if spec.Port == 0 || spec.HealthPath == "" {
		return nil
	}
	client := netgate.Local(fmt.Sprintf("127.0.0.1:%d", spec.Port))
	defer client.CloseIdleConnections()

	url := fmt.Sprintf("http://127.0.0.1:%d%s", spec.Port, spec.HealthPath)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("supervisor: health %s: status %d", url, resp.StatusCode)
	}
	return nil
}
