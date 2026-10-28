# ROUTES — таблица маршрутов webapi

Единственный эталон маршрутов веба игрока (P7). Каждая задача P7, добавляющая
маршрут, правит этот файл и тест сверки `TestRouteTable`: список ниже обязан
совпадать с роутером дословно (метод и путь).

## Таблица маршрутов

| Метод | Путь | Тело запроса | Ответ | Ошибки |
| --- | --- | --- | --- | --- |
| GET | `/api/health` | — | `{"status":"ok"}` | `method_not_allowed` |
| GET | `/` | — | `index.html` из embed | `method_not_allowed` |
| GET | `/static/` | — | файл `static/*` из embed | `method_not_allowed`, `not_found` |
| GET | `/ws` | — | WebSocket 101 | `bad_request` |
| GET | `/api/world` | query `world=<slug>` | `{"world":"…","fields":{…}}` — замороженные поля анкеты из `world_meta` | `method_not_allowed`, `bad_request`, `internal` |
| PUT | `/api/world/tunables` | `{"world":"…","tunables":{…}}` | `{"status":"ok"}` | `method_not_allowed`, `bad_request`, `internal` |
| GET | `/atlas.html` | — | `atlas.html` из embed | `method_not_allowed`, `not_found` |
| GET | `/mcp.html` | — | `mcp.html` из embed | `method_not_allowed`, `not_found` |
| GET | `/api/atlas/worlds` | — | `{"active":"…","worlds":[{"slug","title","active","characters","places","paths","entities","sessions"}]}` | `method_not_allowed`, `internal` |
| POST | `/api/atlas/worlds/birth` | анкета рождения (14 ключей) | `{"status":"ok","world":{"slug","title"}}` | `method_not_allowed`, `bad_request`, `conflict`, `internal` |
| POST | `/api/atlas/worlds/active` | `{"world":"<slug>"}` | `{"status":"ok","active":"<slug>"}` | `method_not_allowed`, `bad_request` |
| POST | `/api/atlas/worlds/replay` | анкета переигровки (тот же `slug`) | `{"status":"ok","world":{"slug","title"}}` | `method_not_allowed`, `bad_request`, `not_found`, `conflict`, `internal` |
| DELETE | `/api/atlas/worlds/delete` | query `world=<slug>&confirm=<slug>` | `{"status":"ok"}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| GET | `/api/mcp/dash` | query `world=<slug>` | `{"world":"…","queue":N,"current":0..1,"stats":{…}}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| GET | `/api/mcp/sources` | query `world=<slug>` | `{"world":"…","sources":[{id,name,format,status,progress,chapters,templates}]}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| POST | `/api/mcp/import` | multipart `file` + `world` | `{"world":"…","sources":[…]}` | `method_not_allowed`, `bad_request`, `conflict`, `not_found`, `internal` |
| POST | `/api/mcp/forge` | `{"world":"<slug>"}` | `{"world":"…","sources":[…]}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| DELETE | `/api/mcp/source/{id}` | query `world=<slug>` | `{"world":"…","sources":[…]}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| GET | `/api/mcp/rings` | query `world=<slug>` | `{"world":"…","journal":{…}}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| GET | `/api/mcp/coeffs` | — | `{"confirm":{…},"sediment":{…},"saw":{…},"time":{…}}` | `method_not_allowed`, `internal` |
| PUT | `/api/mcp/coeffs` | `{"confirm":{…},"sediment":{…},"saw":{…},"time":{…}}` | `{"status":"ok","warning":"меняет пересчёт и логику ходов"}` | `method_not_allowed`, `bad_request`, `internal` |
| GET | `/api/mcp/lemmatizer` | — | `{"pack_id","name","lang","version","enabled","lemmatizer":{"kind","protocol","binary","dict"}}` | `method_not_allowed`, `not_found`, `internal` |
| PUT | `/api/mcp/lemmatizer` | `{"enabled":bool}` | `{"status":"ok"}` | `method_not_allowed`, `bad_request`, `internal` |
| GET | `/settings.html` | — | `settings.html` из embed | `method_not_allowed`, `not_found` |
| GET | `/api/settings` | query `world=<slug>` | `{"world":"…","sections":[…],"embedding_dimension":1024,"general":{…},"world":{…},"advanced":{…},"bindings":[…]}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| PUT | `/api/settings/bindings` | `{"world":"…","bindings":[{"role","model_ref","provider"}]}` | `{"status":"ok"}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| PUT | `/api/settings/config` | `{"host","port","paganel_enabled","scout_enabled","gen_addr","gen_model","gen_timeout","gen_max_tokens","gen_temperature","gen_retries","gen_concurrency","embed_addr","embed_model","embed_timeout","local","sampling"}` | `{"status":"ok"}` | `method_not_allowed`, `bad_request`, `internal` |
| PUT | `/api/settings/keys` | `{"keys":{…}}` | `{"status":"ok"}` | `method_not_allowed`, `bad_request`, `internal` |
| POST | `/api/settings/models/install` | `{"world":"…","role":"…","source":"https://huggingface.co/…","model_ref":"…"}` | `{"status":"ok","model_ref":"…"}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |
| POST | `/api/settings/models/rollback` | `{"world":"…","role":"…"}` | `{"status":"ok"}` | `method_not_allowed`, `bad_request`, `not_found`, `conflict`, `internal` |

Список для дословной сверки (`TestRouteTable`):

```routes
GET /api/health
GET /
GET /static/
GET /ws
GET /api/world
PUT /api/world/tunables
GET /atlas.html
GET /mcp.html
GET /api/atlas/worlds
POST /api/atlas/worlds/birth
POST /api/atlas/worlds/active
POST /api/atlas/worlds/replay
DELETE /api/atlas/worlds/delete
GET /api/mcp/dash
GET /api/mcp/sources
POST /api/mcp/import
POST /api/mcp/forge
DELETE /api/mcp/source/
GET /api/mcp/rings
GET /api/mcp/coeffs
PUT /api/mcp/coeffs
GET /api/mcp/lemmatizer
PUT /api/mcp/lemmatizer
GET /settings.html
GET /api/settings
PUT /api/settings/bindings
PUT /api/settings/config
PUT /api/settings/keys
POST /api/settings/models/install
POST /api/settings/models/rollback
```

## Контракт ошибок

HTTP-код плюс тело:

```json
{"error":{"code":"…","message":"…"}}
```

Коды ошибок — константы в `routes.go`: `not_found`, `method_not_allowed`,
`bad_request`, `conflict`, `internal`.

## Анкета рождения (Атлас)

POST `/api/atlas/worlds/birth` и POST `/api/atlas/worlds/replay` принимают
анкету рождения — ровно 14 ключей (слаг латиницей и 13 смысловых полей,
T-02-01):

```text
slug
title
description
synopsis
prologue
lang
travel_mode
genres
world_rules
social_order
economy_mods
magic
hero_birth
censorship
```

Мировые поля правятся только здесь и только до рождения (T-08-04, T-08-05).

## WebSocket

- Подключение: `GET /ws` с заголовками `Upgrade: websocket`,
  `Connection: Upgrade`, `Sec-WebSocket-Key`.
- Подписка клиента: текстовый фрейм
  `{"type":"subscribe","topics":["screen"]}`.
- Рассылка сервера: текстовый фрейм
  `{"topic":"…","event_type":"…","payload":{…}}`; сообщение получают клиенты,
  подписавшиеся на `topic`.
