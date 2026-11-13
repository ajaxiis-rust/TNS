# Карта багов TNS65 — полный статический прогон 2026-11-13

Ветка `dry/full-fixes`. Модуль `engine/` (Go 1.27.3, golangci-lint v2.15.0, govulncheck v1.9.0).
Конфиг: `engine/.golangci.yml`. Сырые данные: `full-fixes/golangci-report.json`, `race-test.log`, `govulncheck.log`.

Это бейзлайн-карта для триажа, НЕ гейт. Воспроизведение команд — в конце файла.

## 0. Сводка

Всего находок линтеров: **1522**. Полный `go test -race -count=1 ./...` — **зелёный, 0 падений** (~170 c).
govulncheck — **1 уязвимость stdlib: GO-2026-6859** (race в net/http → краш HTTP/1-сервера; фикс — Go toolchain ≥ 1.27.4).

| Линтер | Находок | Категория | Приоритет триажа |
|---|---|---|---|
| `wrapcheck` | 737 | несостыковки/шум | P2 — политика |
| `goconst` | 323 | несостыковки/шум | P2 — политика |
| `noctx` | 209 | надёжность | P1 — проверить |
| `gosec` | 108 | безопасность | P1 — проверить |
| `contextcheck` | 32 | надёжность | P1 — проверить |
| `sqlclosecheck` | 26 | надёжность | P1 — проверить |
| `revive` | 25 | несостыковки/шум | P2 — политика |
| `unparam` | 15 | несостыковки/шум | P2 — политика |
| `rowserrcheck` | 7 | вероятный баг | P0 — чинить |
| `misspell` | 7 | несостыковки/шум | P2 — политика |
| `nilerr` | 5 | вероятный баг | P0 — чинить |
| `errcheck` | 5 | вероятный баг | P0 — чинить |
| `errorlint` | 5 | вероятный баг | P0 — чинить |
| `unused` | 5 | надёжность | P1 — проверить |
| `staticcheck` | 4 | вероятный баг | P0 — чинить |
| `exhaustive` | 4 | несостыковки/шум | P2 — политика |
| `bodyclose` | 2 | вероятный баг | P0 — чинить |
| `ineffassign` | 1 | вероятный баг | P0 — чинить |
| `govet` | 1 | вероятный баг | P0 — чинить |
| `gocritic` | 1 | вероятный баг | P0 — чинить |

## 1. P0 — вероятные баги (полный список)

### bodyclose — 2

- `internal/models/provider.go:235` — response body must be closed
- `internal/models/weaver.go:71` — response body must be closed

### nilerr — 5

- `internal/turn/composition.go:57` — error is not nil (line 55) but it returns nil
- `internal/webapi/handlers_llama_install.go:477` — error is not nil (line 472) but it returns nil
- `internal/webapi/tunnel.go:343` — error is not nil (line 342) but it returns nil
- `internal/workshop/generate.go:80` — error is not nil (line 78) but it returns nil
- `internal/workshop/generate.go:170` — error is not nil (line 164) but it returns nil

### errcheck — 5

- `cmd/tns65/main.go:132` — Error return value of `lock.Release` is not checked
- `cmd/tns65/main.go:504` — Error return value is not checked
- `internal/supervisor/child.go:135` — Error return value is not checked
- `internal/supervisor/lock_test.go:47` — Error return value of `first.Release` is not checked
- `internal/webapi/tunnel_test.go:460` — Error return value is not checked

### errorlint — 5

- `internal/models/provider.go:274` — non-wrapping format verb for fmt.Errorf. Use `%w` to format errors
- `internal/swarm/launch.go:61` — comparing with != will fail on wrapped errors. Use errors.Is to check for a specific error
- `internal/webapi/handlers_book.go:221` — switch on an error will fail on wrapped errors. Use errors.Is to check for specific errors
- `internal/webapi/handlers_llama_install.go:593` — non-wrapping format verb for fmt.Errorf. Use `%w` to format errors
- `internal/workshop/forge_input.go:193` — comparing with == will fail on wrapped errors. Use errors.Is to check for a specific error

### rowserrcheck — 7

- `internal/book/edits_test.go:107` — rows.Err must be checked
- `internal/cast/promote.go:65` — rows.Err must be checked
- `internal/webapi/handlers_chat.go:388` — rows.Err must be checked
- `internal/webapi/handlers_chat.go:467` — rows.Err must be checked
- `internal/webapi/handlers_library.go:141` — rows.Err must be checked
- `internal/webapi/handlers_library.go:168` — rows.Err must be checked
- `internal/workshop/editcensor.go:411` — rows.Err must be checked

### ineffassign — 1

- `internal/webapi/handlers_settings.go:253` — ineffectual assignment to models

### govet — 1

- `internal/birth/ancketa_test.go:227` — inline: Constant reflect.Ptr should be inlined

### staticcheck — 4

- `internal/journal/journal_test.go:301` — QF1012: Use fmt.Fprintf(...) instead of Write([]byte(fmt.Sprintf(...)))
- `internal/paganel/atlas_view.go:84` — S1016: should convert hero (type HeroPosition) to AtlasHero instead of using struct literal
- `internal/turn/composition.go:201` — QF1008: could remove embedded field "Composition" from selector
- `internal/webapi/tunnel_test.go:439` — QF1006: could lift into loop condition

### gocritic — 1

- `internal/journal/journal.go:79` — appendAssign: append result not assigned to the same slice

## 2. P1 — надёжность (сеть/контекст/БД/мёртвый код)

### sqlclosecheck — 26

- `internal/birth/birth_local_test.go:74` — Close should use defer
- `internal/birth/birth_test.go:63` — Close should use defer
- `internal/birth/seed_test.go:49` — Close should use defer
- `internal/book/chapter.go:216` — Close should use defer
- `internal/book/chapter.go:234` — Close should use defer
- `internal/book/edits_test.go:117` — Close should use defer
- `internal/book/window.go:178` — Close should use defer
- `internal/book/window.go:199` — Close should use defer
- `internal/db/base_test.go:55` — Close should use defer
- `internal/db/base_test.go:86` — Close should use defer
- `internal/db/base_test.go:151` — Close should use defer
- `internal/db/literary_test.go:54` — Close should use defer
- `internal/db/literary_test.go:96` — Close should use defer
- `internal/db/literary_test.go:124` — Close should use defer
- `internal/db/world_test.go:62` — Close should use defer
- `internal/db/world_test.go:97` — Close should use defer
- `internal/db/world_test.go:148` — Close should use defer
- `internal/paganel/forge_ring.go:312` — Close should use defer
- `internal/turn/pipeline_test.go:79` — Close should use defer
- `internal/webapi/handlers_library.go:157` — Close should use defer
- `internal/workshop/complement.go:99` — Close should use defer
- `internal/workshop/editcensor.go:421` — Close should use defer
- `internal/workshop/horoscope_match_test.go:158` — Close should use defer
- `internal/workshop/horoscope_match_test.go:177` — Close should use defer
- `internal/workshop/queue_test.go:45` — Close should use defer
- `internal/workshop/queue_test.go:78` — Close should use defer

### contextcheck — 32

- `cmd/tns65/main.go:195` — Function `startTelegramBot` should pass the context parameter
- `internal/book/claim.go:84` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/book/claim.go:155` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/book/digest.go:121` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/book/publish_gate.go:89` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/embedserver/embedserver.go:117` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/models/embed.go:66` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/models/provider.go:218` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/models/provider.go:347` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/models/weaver.go:60` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/turn/pipeline.go:65` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/turn/stages.go:672` — Function `queueSedimentCandidates->Enqueue->attempt` should pass the context parameter
- `internal/turn/timejump.go:250` — Function `orderRecompute->Enqueue->attempt` should pass the context parameter
- `internal/turn/weaver_path.go:42` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/webapi/handlers_llama_install.go:402` — Function `detectLlamaServer->detectLlamaServer$1->probeLlamaServer` should pass the context parameter
- `internal/webapi/handlers_llama_install.go:425` — Function `probeLlamaServer` should pass the context parameter
- `internal/webapi/handlers_llama_install.go:457` — Function `scanLlamaServer` should pass the context parameter
- `internal/webapi/handlers_llama_install.go:491` — Function `probeLlamaServer` should pass the context parameter
- `internal/webapi/handlers_llama_install.go:514` — Function `detectLlamaServer->detectLlamaServer$1->probeLlamaServer` should pass the context parameter
- `internal/webapi/handlers_llama_install.go:533` — Function `runLlamaInstallScript` should pass the context parameter
- `internal/webapi/handlers_llama_install.go:538` — Function `detectLlamaServer->detectLlamaServer$1->probeLlamaServer` should pass the context parameter
- `internal/webapi/handlers_settings.go:321` — Function `readEmbedderStatus` should pass the context parameter
- `internal/webapi/handlers_settings.go:682` — Function `restartTelegramChannel->restartTelegramChannel$1` should pass the context parameter
- `internal/webapi/handlers_settings.go:1365` — Function `startInstalledModelChild->detectLlamaServer->detectLlamaServer$1->probeLlamaServer` should pass the context parameter
- `internal/webapi/handlers_settings.go:1703` — Function `buildFirstRunResponse->probeFirstRunStation->llamaStationStatus->detectLlamaServer->detectLlamaServer$1->probeLlamaServer` should pass the context parameter
- `internal/webapi/handlers_settings.go:1738` — Function `buildFirstRunResponse->probeFirstRunStation->llamaStationStatus->detectLlamaServer->detectLlamaServer$1->probeLlamaServer` should pass the context parameter
- `internal/workshop/compose.go:86` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/workshop/editor.go:59` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/workshop/forge_book.go:52` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/workshop/generate.go:62` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/workshop/generate.go:157` — Non-inherited new context, use function like `context.WithXXX` instead
- `internal/workshop/style.go:258` — Non-inherited new context, use function like `context.WithXXX` instead

### noctx — 209

По пакетам (файл→пакет верхнего уровня): `internal`: 208, `cmd`: 1

Примеры (первые 10):
- `cmd/tns65/main.go:479` — os/exec.Command must not be called. use os/exec.CommandContext
- `internal/bake/bake.go:252` — (*net/http.Client).Post must not be called. use (*net/http.Client).Do(*http.Request)
- `internal/bake/bake_test.go:145` — os/exec.Command must not be called. use os/exec.CommandContext
- `internal/bake/bake_test.go:149` — os/exec.Command must not be called. use os/exec.CommandContext
- `internal/bake/bake_test.go:222` — os/exec.Command must not be called. use os/exec.CommandContext
- `internal/bake/bake_test.go:232` — os/exec.Command must not be called. use os/exec.CommandContext
- `internal/birth/anchor.go:105` — (*database/sql.DB).Query must not be called. use (*database/sql.DB).QueryContext
- `internal/birth/anchor_test.go:117` — (*database/sql.DB).Exec must not be called. use (*database/sql.DB).ExecContext
- `internal/birth/anchor_test.go:126` — (*database/sql.DB).Exec must not be called. use (*database/sql.DB).ExecContext
- `internal/birth/anchor_test.go:196` — (*database/sql.DB).Exec must not be called. use (*database/sql.DB).ExecContext

Полный список — в JSON.

### unused — 5

- `internal/turn/clock.go:18` — type realClock is unused
- `internal/turn/clock.go:20` — func realClock.Now is unused
- `internal/turn/clock.go:21` — func realClock.After is unused
- `internal/turn/stopcrane.go:37` — func commitTurn is unused
- `internal/turn/stopcrane.go:204` — func writeHeroState is unused

## 3. gosec — 108 (по правилам)

| Правило | Находок |
|---|---|
| G306 | 79 |
| G101 | 8 |
| G115 | 5 |
| G404 | 3 |
| G204 | 3 |
| G703 | 3 |
| G122 | 2 |
| G602 | 2 |
| G112 | 1 |
| G505 | 1 |
| G401 | 1 |

### gosec G306

- `internal/bake/bake.go:281` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/bake/bake_test.go:32` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/bake/bake_test.go:185` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/bake/bake_test.go:192` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/birth.go:518` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/checksum_test.go:25` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/checksum_test.go:48` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/checksum_test.go:112` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/langbase_test.go:64` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/langbase_test.go:67` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/seed_test.go:125` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/seed_test.go:152` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/seed_test.go:167` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/birth/seed_test.go:170` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/book/export.go:162` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/book/export.go:225` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/cast/scan_test.go:162` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/censor/censor_test.go:48` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/censor/censor_test.go:167` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/censor/censor_test.go:171` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/censor/censor_test.go:243` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/censor/refusal_test.go:16` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/langpack/build.go:51` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/langpack/build.go:73` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/langpack/langpack_test.go:119` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/langpack/langpack_test.go:133` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/langpack/langpack_test.go:179` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/langpack/langpack_test.go:244` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/langpack/langpack_test.go:262` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/langpack/versions.go:73` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/library/library.go:95` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/library/library.go:119` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/models/fetch_test.go:227` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/models/provider_test.go:48` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/models/provider_test.go:300` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/netgate/gate_test.go:37` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/netgate/gate_test.go:91` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/sediment/dedup_test.go:109` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/sediment/dedup_test.go:125` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/selector/saw_test.go:102` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/supervisor/supervisor_test.go:351` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/swarm/swarm_test.go:52` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/swarm/swarm_test.go:91` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/turn/confirm_test.go:26` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/turn/prepath_test.go:19` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/turn/prepath_test.go:23` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/turn/timejump_test.go:21` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/turn/timejump_test.go:159` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_atlas_test.go:90` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_coeffs_test.go:41` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_coeffs_test.go:201` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_coeffs_test.go:205` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_forge.go:49` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_llama_install_test.go:74` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_llama_install_test.go:78` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_llama_install_test.go:112` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_llama_install_test.go:332` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_mcp.go:139` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_monitor_test.go:22` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_providers_test.go:40` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_providers_test.go:59` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_settings_test.go:42` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_settings_test.go:624` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_settings_test.go:747` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_settings_test.go:757` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_settings_test.go:819` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_settings_test.go:851` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_settings_test.go:865` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/handlers_settings_test.go:869` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/model_files_test.go:31` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/model_files_test.go:189` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/model_files_test.go:198` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/tunnel_test.go:265` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/webapi/tunnel_test.go:268` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/workshop/forge_book_test.go:48` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/workshop/forge_book_test.go:161` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/workshop/forge_input_test.go:62` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/workshop/queue_test.go:176` — G306: Expect WriteFile permissions to be 0600 or less
- `internal/workshop/style_test.go:57` — G306: Expect WriteFile permissions to be 0600 or less

### gosec G101

- `internal/sediment/clean_test.go:77` — G101: Potential hardcoded credentials
- `internal/turn/prepath_test.go:95` — G101: Potential hardcoded credentials
- `internal/turn/slots.go:13` — G101: Potential hardcoded credentials
- `internal/turn/slots.go:14` — G101: Potential hardcoded credentials
- `internal/turn/slots.go:15` — G101: Potential hardcoded credentials
- `internal/turn/slots.go:16` — G101: Potential hardcoded credentials
- `internal/turn/slots.go:17` — G101: Potential hardcoded credentials
- `internal/turn/slots.go:18` — G101: Potential hardcoded credentials

### gosec G115

- `internal/birth/pack.go:144` — G115: integer overflow conversion uint64 -> int64
- `internal/turn/fate.go:123` — G115: integer overflow conversion int64 -> uint64
- `internal/turn/fate.go:124` — G115: integer overflow conversion uint64 -> int
- `internal/turn/fate.go:125` — G115: integer overflow conversion uint64 -> int
- `internal/workshop/luck.go:111` — G115: integer overflow conversion uint64 -> int64

### gosec G404

- `internal/birth/npc_layers.go:107` — G404: Use of weak random number generator (math/rand or math/rand/v2 instead of crypto/rand)
- `internal/birth/pack.go:112` — G404: Use of weak random number generator (math/rand or math/rand/v2 instead of crypto/rand)
- `internal/workshop/luck.go:44` — G404: Use of weak random number generator (math/rand or math/rand/v2 instead of crypto/rand)

### gosec G204

- `internal/lemmarpc/client.go:129` — G204: Subprocess launched with a potential tainted input or cmd arguments
- `internal/supervisor/child.go:312` — G204: Subprocess launched with a potential tainted input or cmd arguments
- `internal/swarm/launch.go:38` — G204: Subprocess launched with a potential tainted input or cmd arguments

### gosec G703

- `internal/webapi/handlers_atlas.go:351` — G703: Path traversal via taint analysis
- `internal/webapi/handlers_mcp.go:589` — G703: Path traversal via taint analysis
- `internal/webapi/model_files_test.go:88` — G703: Path traversal via taint analysis

### gosec G122

- `internal/langpack/langpack_test.go:209` — G122: Filesystem operation in filepath.Walk/WalkDir callback uses race-prone path; consider root-scoped APIs (e.g. os.Root) to prevent symlink TOCTOU traversal
- `internal/netgate/gate_test.go:183` — G122: Filesystem operation in filepath.Walk/WalkDir callback uses race-prone path; consider root-scoped APIs (e.g. os.Root) to prevent symlink TOCTOU traversal

### gosec G602

- `internal/langpack/versions.go:53` — G602: slice index out of range
- `internal/portgate/portgate_test.go:31` — G602: slice index out of range

### gosec G112

- `internal/webapi/server.go:79` — G112: Potential Slowloris Attack because ReadHeaderTimeout is not configured in the http.Server

### gosec G505

- `internal/webapi/ws.go:5` — G505: Blocklisted import crypto/sha1: weak cryptographic primitive

### gosec G401

- `internal/webapi/ws.go:164` — G401: Use of weak cryptographic primitive

## 4. P2 — несостыковки/шум (сводно, полный список в JSON)

### wrapcheck — 737

Топ-10 файлов: `internal/webapi/tunnel.go`: 32, `internal/book/claim.go`: 21, `internal/webapi/handlers_settings.go`: 20, `internal/birth/birth.go`: 17, `internal/book/deepread.go`: 17, `internal/book/wake.go`: 17, `internal/workshop/complement.go`: 17, `internal/book/chapter.go`: 15, `internal/turn/troupe.go`: 15, `cmd/tns65/main.go`: 14

### goconst — 323

Топ-10 файлов: `internal/webapi/handlers_settings_test.go`: 15, `internal/webapi/telegram_commands_test.go`: 11, `internal/webapi/handlers_settings.go`: 10, `internal/cast/ceiling_test.go`: 8, `internal/structural/score_test.go`: 8, `internal/webapi/tunnel_test.go`: 8, `internal/book/edits_test.go`: 7, `internal/cast/describe_test.go`: 7, `internal/cast/scan_test.go`: 7, `internal/db/literary_test.go`: 7

### revive — 25

Топ-10 файлов: `cmd/tns65/hooks_release.go`: 3, `internal/paganel/corridor.go`: 3, `internal/turn/pipeline.go`: 2, `internal/version/version.go`: 2, `internal/webapi/handlers_settings.go`: 2, `internal/webapi/tunnel_test.go`: 2, `internal/db/open.go`: 1, `internal/paganel/scout.go`: 1, `internal/paganel/settings.go`: 1, `internal/sediment/record.go`: 1

### unparam — 15

Топ-10 файлов: `internal/book/publish_gate_test.go`: 2, `internal/birth/ancketa_test.go`: 1, `internal/book/export_test.go`: 1, `internal/cast/store_test.go`: 1, `internal/models/fetch.go`: 1, `internal/models/fetch_test.go`: 1, `internal/paganel/calibrate_test.go`: 1, `internal/paganel/forge_ring_test.go`: 1, `internal/paganel/ring_journal_test.go`: 1, `internal/portgate/portgate_test.go`: 1

### misspell — 7

Топ-10 файлов: `internal/birth/npc_layers.go`: 4, `internal/birth/npc_layers_test.go`: 1, `internal/workshop/complement.go`: 1, `internal/workshop/queue.go`: 1

- `internal/birth/npc_layers.go:27` — `Humours` is a misspelling of `Humors`
- `internal/birth/npc_layers.go:28` — `Humours` is a misspelling of `Humors`
- `internal/birth/npc_layers.go:49` — `Humours` is a misspelling of `Humors`
- `internal/birth/npc_layers.go:122` — `Humours` is a misspelling of `Humors`
- `internal/birth/npc_layers_test.go:101` — `Humours` is a misspelling of `Humors`
- `internal/workshop/complement.go:84` — `cancelled` is a misspelling of `canceled`
- `internal/workshop/queue.go:47` — `cancelled` is a misspelling of `canceled`

### exhaustive — 4

Топ-10 файлов: `internal/selector/saw.go`: 1, `internal/selector/selector.go`: 1, `internal/workshop/forge_pipeline.go`: 1, `internal/workshop/queue.go`: 1

- `internal/selector/saw.go:176` — missing cases in expression switch: MoodNeutral
- `internal/selector/selector.go:113` — missing cases in expression switch: RecommendNone
- `internal/workshop/forge_pipeline.go:111` — missing cases in expression switch: sediment.TokenThing, sediment.TokenPrice
- `internal/workshop/queue.go:66` — missing cases in expression switch: KindAnalysis, KindForgePlan, KindPythia, KindEdit, KindEpoch

## 5. Рекомендации

1. **GO-2026-6859**: обновить Go toolchain до ≥ 1.27.4 (`go 1.27.3` стоит). Единственная найденная уязвимость, чинится апгрейдом тулчейна.
2. **P0 чинить сразу** — это короткий список (см. раздел 1), в основном реальные утечки/потерянные ошибки.
3. **wrapcheck (737)** — почти весь шум от архитектуры «верни ошибку наружу как есть»; рекомендуется настроить `wrapcheck.ignoreSigs` под свои внутренние пакеты или выключить, оставив `errorlint`.
4. **goconst (323)** — дубли строк; чинить пачками по пакетам, часть — кандидаты в константы/ключи i18n.
5. **noctx (209)** — в основном `http.NewRequest` без контекста (fetch моделей, Telegram, health); системно чинится передачей `ctx` сверху — связка с `contextcheck` (32).
6. **gosec G304/G204 и пр.** — проверить ручным триажем; в коде работы с файлами моделей это часто осознанный паттерн.
7. Race-прогон зелёный, но по памяти известны флаки censor/supervisor под параллельной нагрузкой — гоном `go test -race ./...` в CI держать как отдельный шаг.

## 6. Воспроизведение

```bash
cd engine
/home/ajaxiis/go/bin/golangci-lint run --output.json.path=../full-fixes/golangci-report.json --output.text.path=none ./...
go test -race -count=1 -timeout=25m ./... 2>&1 | tee ../full-fixes/race-test.log
/home/ajaxiis/go/bin/govulncheck ./... 2>&1 | tee ../full-fixes/govulncheck.log
```
