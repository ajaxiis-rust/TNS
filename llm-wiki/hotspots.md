# Hotspots & Risk Symbols

> Symbols here carry the highest change risk.
> Prefer wrapping them in interfaces before modifying.

## Highest Fan-in (most-called)

| Symbol | Callers | File |
|--------|---------|------|
| `writeError` | 480 | engine/internal/webapi/routes.go:45 |
| `Scan` | 224 | engine/internal/cast/scan.go:77 |
| `writeJSON` | 126 | engine/internal/webapi/routes.go:38 |
| `nowUTC` | 52 | engine/internal/cast/store.go:177 |
| `nowUTC` | 52 | engine/internal/workshop/queue.go:324 |
| `nowUTC` | 52 | engine/internal/book/wake.go:372 |
| `readSettingsKeys` | 32 | engine/internal/providerroute/route.go:189 |
| `readSettingsKeys` | 32 | engine/internal/webapi/handlers_settings.go:2190 |
| `setModelJob` | 32 | engine/internal/webapi/handlers_settings.go:1359 |
| `OpenWriterAs` | 31 | engine/internal/db/open.go:55 |

## Highest Complexity

| Symbol | Score | Severity | File |
|--------|-------|----------|------|
| `TestScenes` | 103 | VERY HIGH | engine/internal/deepread/scenes_test.go:22 |
| `TestCharacterCentroids` | 90 | VERY HIGH | engine/internal/deepread/characters_test.go:22 |
| `TestWorldSchema` | 75 | VERY HIGH | engine/internal/db/world_test.go:13 |
| `TestLiterarySchema` | 74 | VERY HIGH | engine/internal/db/literary_test.go:11 |
| `TestDisputeBudget` | 71 | VERY HIGH | engine/internal/deepread/dispute_test.go:20 |
| `TestDeepreadKey` | 68 | VERY HIGH | engine/internal/deepread/key_test.go:24 |
| `TestLiteraryDeepreadSchema` | 61 | VERY HIGH | engine/internal/db/literary_deepread_test.go:12 |
| `TestStyle` | 61 | VERY HIGH | engine/internal/deepread/style_test.go:49 |
| `TestSalienceBands` | 60 | VERY HIGH | engine/internal/deepread/salience_test.go:24 |
| `TestModelInstallFileNaming` | 52 | VERY HIGH | engine/internal/webapi/model_files_test.go:48 |

## God Object Candidates

| Struct | Methods | Fields | Outgoing calls | Severity |
|--------|---------|--------|----------------|----------|
| `TurnState` | 0 | 36 | 0 | HIGH |
| `Config` | 5 | 17 | 3 | MEDIUM |
| `Ancketa` | 0 | 16 | 0 | MEDIUM |
| `settingsResponse` | 0 | 16 | 0 | MEDIUM |
| `compFixture` | 7 | 4 | 25 | LOW |
| `builder` | 0 | 14 | 0 | LOW |
| `DisputeQuestion` | 0 | 14 | 0 | LOW |
| `BookCharacter` | 1 | 13 | 0 | LOW |
| `Book` | 0 | 13 | 0 | LOW |
| `Scene` | 0 | 12 | 0 | LOW |

