package webapi

import (
	"strings"
	"testing"
)

func TestNoAuth(t *testing.T) {
	if authModeNone != "none" {
		t.Fatalf("auth mode = %q, want none", authModeNone)
	}
	if len(accessLevels) != 0 {
		t.Fatalf("access levels = %v, want none", accessLevels)
	}
	if len(adminZonePaths) != 0 {
		t.Fatalf("admin zone paths = %v, want none", adminZonePaths)
	}

	for _, s := range routeSpecs(NewHub()) {
		if routeRequiresAuth(s.method, s.path) {
			t.Fatalf("route %s %s requires auth", s.method, s.path)
		}
		if strings.Contains(s.path, "login") || strings.Contains(s.path, "logout") ||
			strings.Contains(s.path, "/admin") || strings.Contains(s.path, "/auth") {
			t.Fatalf("route %s %s implies auth/admin zone", s.method, s.path)
		}
	}
}
