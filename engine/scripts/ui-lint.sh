#!/usr/bin/env bash
set -uo pipefail

# Статический линтер интерфейса (T-08-01). Работает без Node и без сборщика.
# Resolve the engine root (this script lives in engine/scripts).
cd "$(dirname "$0")/.."

fail=0

# 1. Нет внешних CDN: в статике нет http(s)-ссылок.
cdn_out="$(grep -rnE 'https?://' web/ 2>/dev/null || true)"
if [ -n "$cdn_out" ]; then
    echo "FAIL: внешние CDN найдены:"
    echo "$cdn_out"
    fail=1
else
    echo "OK: нет внешних CDN"
fi

# 2. Нет inline-скриптов вне разрешённого списка. Разрешённый список пуст:
#    все <script> обязаны иметь src=.
inline_out="$(grep -rn '<script' web/*.html 2>/dev/null | grep -vE 'src=' || true)"
if [ -n "$inline_out" ]; then
    echo "FAIL: inline-скрипты вне разрешённого списка:"
    echo "$inline_out"
    fail=1
else
    echo "OK: нет inline-скриптов вне разрешённого списка"
fi

# 3. Каждый id из разметки используется в JS.
ids="$(grep -hoE 'id="[^"]+"' web/*.html 2>/dev/null | sed -E 's/^id="//; s/"$//' | sort -u)"
missing_ids=()
while IFS= read -r id; do
    [ -z "$id" ] && continue
    if ! grep -rqF -- "$id" web/static/*.js 2>/dev/null; then
        missing_ids+=("$id")
    fi
done <<< "$ids"

if [ "${#missing_ids[@]}" -gt 0 ]; then
    echo "FAIL: id из разметки не используются в JS:"
    printf '  %s\n' "${missing_ids[@]}"
    fail=1
else
    echo "OK: каждый id из разметки используется в JS"
fi

# 4. Нет кириллических ключей i18n: ключи в i18n.js только ASCII-латиницей.
cyr_keys="$(grep -nE '^[[:space:]]*"[^"]*[А-Яа-яЁё][^"]*"[[:space:]]*:' web/static/i18n.js 2>/dev/null || true)"
if [ -n "$cyr_keys" ]; then
    echo "FAIL: кириллические ключи i18n:"
    echo "$cyr_keys"
    fail=1
else
    echo "OK: нет кириллических ключей i18n"
fi

# 5. Код-имена ролей на игровых экранах — ошибка (UI-10): чат/индекс, Атлас и
#    История показывают только функциональные названия; mcp.html и settings.html
#    допустимы и не проверяются.
code_names='Паганель|Пила|Ткач|Гефест|Цензор|Редактор|Пифия|Труппа|Челнок|Селектор'
game_screens=(web/index.html web/atlas.html web/history.html)
screen_names="$(grep -rnE "$code_names" "${game_screens[@]}" 2>/dev/null || true)"
if [ -n "$screen_names" ]; then
    echo "FAIL: код-имена ролей на игровых экранах:"
    echo "$screen_names"
    fail=1
else
    echo "OK: нет код-имён ролей на игровых экранах"
fi

# 6. Игровые ключи словаря i18n (chat.*, atlas.*, history.*) не содержат
#    код-имён ролей в значениях.
game_i18n="$(grep -nE '^[[:space:]]*"(chat|atlas|history)\.' web/static/i18n.js 2>/dev/null | grep -E "$code_names" || true)"
if [ -n "$game_i18n" ]; then
    echo "FAIL: код-имена ролей в игровых ключах i18n:"
    echo "$game_i18n"
    fail=1
else
    echo "OK: нет код-имён ролей в игровых ключах i18n"
fi

exit "$fail"
