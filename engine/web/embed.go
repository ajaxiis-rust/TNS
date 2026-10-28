// Package web — встроенная статика веба игрока (T-08-02, шаг 2).
package web

import "embed"

// FS — корень engine/web: index.html и static/*. Статика отдаётся из бинарника,
// не с диска.
//
//go:embed index.html atlas.html static
var FS embed.FS
