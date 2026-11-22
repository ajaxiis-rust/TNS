# Codebase Overview

**Module:** ``

| Packages | Files | Symbols | Import edges | Routes | SQL queries | Env reads |
|----------|-------|---------|--------------|--------|-------------|----------|
| 37 | 433 | 4906 | 2942 | 0 | 546 | 2 |

## Top Hotspots (fan-in)

| Symbol | Callers | File |
|--------|---------|------|
| `writeError` | 480 | engine/internal/webapi/routes.go |
| `Scan` | 212 | engine/internal/cast/scan.go |
| `writeJSON` | 126 | engine/internal/webapi/routes.go |
| `nowUTC` | 50 | engine/internal/workshop/queue.go |
| `nowUTC` | 50 | engine/internal/book/wake.go |

## Package Instability (highest first)

| Package | Fan-in | Fan-out | Instability |
|---------|--------|---------|-------------|
| `golang.org/x/text/encoding/unicode` | 0 | 0 | -1.00 |
| `golang.org/x/text/transform` | 0 | 0 | -1.00 |
| `github.com/AlexMaxy/gomorphy` | 0 | 0 | -1.00 |
| `modernc.org/sqlite` | 0 | 0 | -1.00 |
| `golang.org/x/text/encoding` | 0 | 0 | -1.00 |

## Quick Reference

| Topic | Detail page |
|-------|-------------|
| Architecture & layers | `architecture.md` |
| Exported API surface | `api-surface.md` |
| High-risk symbols | `hotspots.md` |
| Env vars (2) | `env.md` |
| SQL queries (546) | `errors.md` |

