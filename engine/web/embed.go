// Package web — встроенная статика веба игрока (T-08-02, шаг 2).
package web

import "embed"

// FS — корень engine/web: index.html и static/*. Встроенный фронт — фолбэк:
// если рядом с бинарником лежит полный каталог web/, webapi раздаёт статику с
// диска и правка UI не требует пересборки (гибрид IMP-12). manifest.json не
// вшивается — им пинится sha256 поставки фронта на диске.
//
//go:embed index.html atlas.html history.html mcp.html settings.html wizard.html static
var FS embed.FS
