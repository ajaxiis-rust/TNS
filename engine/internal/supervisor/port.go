package supervisor

import (
	"fmt"
	"net"
)

// CheckPortFree проверяет, что loopback-порт свободен, до старта ребёнка.
// Порт 0 — без проверки (ребёнок без сетевого health-эндпоинта). Занятый порт —
// понятная ошибка (T-12-19, «порт занят → понятная ошибка»).
func CheckPortFree(port int) error {
	if port == 0 {
		return nil
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("supervisor: port %d is already in use: %w", port, err)
	}
	_ = l.Close()
	return nil
}
