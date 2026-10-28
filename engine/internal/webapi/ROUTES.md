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

Список для дословной сверки (`TestRouteTable`):

```routes
GET /api/health
GET /
GET /static/
GET /ws
```

## Контракт ошибок

HTTP-код плюс тело:

```json
{"error":{"code":"…","message":"…"}}
```

Коды ошибок — константы в `routes.go`: `not_found`, `method_not_allowed`,
`bad_request`.

## WebSocket

- Подключение: `GET /ws` с заголовками `Upgrade: websocket`,
  `Connection: Upgrade`, `Sec-WebSocket-Key`.
- Подписка клиента: текстовый фрейм
  `{"type":"subscribe","topics":["screen"]}`.
- Рассылка сервера: текстовый фрейм
  `{"topic":"…","event_type":"…","payload":{…}}`; сообщение получают клиенты,
  подписавшиеся на `topic`.
