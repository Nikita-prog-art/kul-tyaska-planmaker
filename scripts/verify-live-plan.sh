#!/usr/bin/env bash
# Проверка на живой базе: план, сохранённый старым кодом (со списками-null),
# должен отдаваться в конструктор массивами. Работает с копией базы и
# отдельным процессом на свободном порту, продакшен не трогает.
#
# Запускать на сервере:  bash verify-live-plan.sh
set -euo pipefail

DB="${1:-/var/lib/kultyaska/kultyaska.json}"
PORT="${2:-8097}"
BIN="${3:-/opt/kul-tyaska/kultyaska}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cp "$DB" "$TMP/db.json"
TOKEN="verify-$(date +%s)"

python3 - "$TMP/db.json" "$TOKEN" <<'PY'
import json, sys
path, token = sys.argv[1], sys.argv[2]
db = json.load(open(path, encoding='utf-8'))
users = db.get('users') or []
if not users:
    raise SystemExit('в базе нет пользователей')
uid = users[0]['id']
db.setdefault('sessions', {})[token] = {'token': token, 'user_id': uid, 'expires': '2099-01-01T00:00:00Z'}
with open(path, 'w', encoding='utf-8') as fh:
    json.dump(db, fh, ensure_ascii=False)
plans = db.get('plans') or {}
print('пользователь: %s, планов: %d' % (users[0]['nick'], len(plans)))
if not plans:
    raise SystemExit('в базе нет планов — проверять нечего')
print('первый план: %s' % list(plans.values())[0].get('title'))
PY

KULTYASKA_DB="$TMP/db.json" KULTYASKA_ADDR="127.0.0.1:$PORT" "$BIN" >"$TMP/app.log" 2>&1 &
APP=$!
sleep 2

PLAN_ID="$(python3 -c "import json;print(list((json.load(open('$TMP/db.json',encoding='utf-8')).get('plans') or {}))[0])")"
PAGE="$(curl -sS --max-time 20 -b "kt_session=$TOKEN" "http://127.0.0.1:$PORT/plans/$PLAN_ID")"
kill "$APP" 2>/dev/null || true
wait "$APP" 2>/dev/null || true

echo "--- что в данных плана ---"
printf '%s' "$PAGE" | grep -oE '"(picks|deferred|not_taken|shop|ladders|accumulators|folios)":(null|\[\])' | sort -u

if printf '%s' "$PAGE" | grep -q '"picks":\[\]'; then
    echo "ОК: конструктор получает пустые массивы"
else
    echo "ПЛОХО: в данных плана всё ещё null" >&2
    exit 1
fi
if printf '%s' "$PAGE" | grep -qE '"(picks|deferred|shop|ladders)":null'; then
    echo "ПЛОХО: остались null-списки" >&2
    exit 1
fi
