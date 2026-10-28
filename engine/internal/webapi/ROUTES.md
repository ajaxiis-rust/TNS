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
| GET | `/api/atlas/worlds` | — | `{"active":"…","worlds":[{"slug","title","active","characters","places","paths","entities","sessions"}]}` | `method_not_allowed`, `internal` |
| POST | `/api/atlas/worlds/birth` | анкета рождения (14 ключей) | `{"status":"ok","world":{"slug","title"}}` | `method_not_allowed`, `bad_request`, `conflict`, `internal` |
| POST | `/api/atlas/worlds/active` | `{"world":"<slug>"}` | `{"status":"ok","active":"<slug>"}` | `method_not_allowed`, `bad_request` |
| POST | `/api/atlas/worlds/replay` | анкета переигровки (тот же `slug`) | `{"status":"ok","world":{"slug","title"}}` | `method_not_allowed`, `bad_request`, `not_found`, `conflict`, `internal` |
| DELETE | `/api/atlas/worlds/delete` | query `world=<slug>&confirm=<slug>` | `{"status":"ok"}` | `method_not_allowed`, `bad_request`, `not_found`, `internal` |

Список для дословной сверки (`TestRouteTable`):

```routes
GET /api/health
GET /
GET /static/
GET /ws
GET /api/world
PUT /api/world/tunables
GET /atlas.html
GET /api/atlas/worlds
POST /api/atlas/worlds/birth
POST /api/atlas/worlds/active
POST /api/atlas/worlds/replay
DELETE /api/atlas/worlds/delete
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
