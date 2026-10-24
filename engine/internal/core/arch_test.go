package core

import (
	"os/exec"
	"strings"
	"testing"
)

// infraPackages — инфраструктурные пакеты, которым запрещено импортировать
// доменные пакеты (F-12.6): всё из tns65/internal/, кроме самих инфраструктурных
// четырёх и низлежащего version.
var infraPackages = []string{
	"db",
	"journal",
	"netgate",
	"portgate",
}

// allowedInternal — разрешённый набор импортов под префиксом tns65/internal/.
var allowedInternal = map[string]bool{
	"tns65/internal/db":       true,
	"tns65/internal/journal":  true,
	"tns65/internal/netgate":  true,
	"tns65/internal/portgate": true,
	"tns65/internal/version":  true,
}

func TestNoImportCycles(t *testing.T) {
	for _, p := range infraPackages {
		pkg := "tns65/internal/" + p
		out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v", pkg, err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "tns65/internal/") {
				continue
			}
			if allowedInternal[line] {
				continue
			}
			t.Errorf("%s imports forbidden package %s", pkg, line)
		}
	}
}
