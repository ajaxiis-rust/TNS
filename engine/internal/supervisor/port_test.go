package supervisor

import (
	"net"
	"strings"
	"testing"
)

// TestCheckPortFree — «Проверка» T-12-19: занятый порт даёт понятную ошибку.
func TestCheckPortFree(t *testing.T) {
	if err := CheckPortFree(0); err != nil {
		t.Fatalf("CheckPortFree(0) = %v, want nil", err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	err = CheckPortFree(port)
	if err == nil {
		t.Fatalf("CheckPortFree(%d) = nil, want busy error", port)
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("CheckPortFree(%d) error = %q, want mention 'already in use'", port, err.Error())
	}
}
