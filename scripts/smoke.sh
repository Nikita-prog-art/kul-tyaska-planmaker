#!/usr/bin/env bash
# Дымовая проверка развёрнутого приложения по публичному адресу.
#   ./scripts/smoke.sh https://example.trycloudflare.com
set -uo pipefail

BASE="${1:?укажи базовый URL, например https://xxx.trycloudflare.com}"
BASE="${BASE%/}"
JAR="$(mktemp)"
NICK="smoke$RANDOM"
FAILS=0

ok()   { echo "  ✓ $1"; }
bad()  { echo "  ✗ $1"; FAILS=$((FAILS + 1)); }
check() { if [ "$2" = "$3" ]; then ok "$1 ($2)"; else bad "$1: получил $2, ждал $3"; fi; }

echo "Дымовая проверка $BASE"

check "главная отдаётся" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "$BASE/")" "200"
if curl -sS --max-time 20 "$BASE/" | grep -q "Планировщик"; then ok "на главной есть заголовок"; else bad "на главной нет заголовка"; fi

for asset in app.css engine.js builder.js; do
  check "статика /static/$asset" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "$BASE/static/$asset")" "200"
done

check "регистрация" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 25 -c "$JAR" \
  -d "nick=$NICK&discord=@$NICK&password=verysecret1&password2=verysecret1" "$BASE/register")" "303"

PLANS="$(curl -sS --max-time 20 -b "$JAR" -c "$JAR" "$BASE/plans")"
if printf '%s' "$PLANS" | grep -q "Мои планы"; then ok "страница планов"; else bad "страница планов не отдалась"; fi
if printf '%s' "$PLANS" | grep -q "06-zaskoruzlye-piramidy"; then ok "каркас виден в списке"; else bad "каркаса нет в списке"; fi

LOC="$(curl -sS -o /dev/null -w '%{redirect_url}' --max-time 20 -b "$JAR" -c "$JAR" \
  -d "title=Смоук&emoji=🩸&preset=06-zaskoruzlye-piramidy" "$BASE/plans")"
# curl отдаёт Location уже склеенным с базой — приводим к полному URL.
case "$LOC" in
  /*) LOC="$BASE$LOC" ;;
esac
if printf '%s' "$LOC" | grep -q "/plans/"; then ok "план создан из каркаса ($LOC)"; else bad "план не создался (redirect: $LOC)"; fi

EDITOR="$(curl -sS --max-time 20 -b "$JAR" "$LOC")"
for needle in 'id="kt-plan"' 'id="kt-sections"' "пинг систем" "великий успех"; do
  if printf '%s' "$EDITOR" | grep -q "$needle"; then ok "в конструкторе есть $needle"; else bad "в конструкторе нет $needle"; fi
done

TXT="$(curl -sS --max-time 20 -b "$JAR" "$LOC/export?format=txt")"
# План только что создан и покупок в нём нет, поэтому проверяем каркас экспорта.
for needle in "Невыученные" "Остаток" "Итого потрачено" "[*]"; do
  if printf '%s' "$TXT" | grep -qF "$needle"; then ok "в экспорте есть «$needle»"; else bad "в экспорте нет «$needle»"; fi
done

check "страница экспорта" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 -b "$JAR" "$LOC/export")" "200"
check "справка по механике" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "$BASE/guide")" "200"
check "каркас отдаётся файлом" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "$BASE/presets/06-zaskoruzlye-piramidy")" "200"
check "профиль" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 -b "$JAR" "$BASE/profile")" "200"
check "смена пароля" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 25 -b "$JAR" -c "$JAR" \
  -d "current=verysecret1&password=verysecret2&password2=verysecret2" "$BASE/profile/password")" "303"
# Админка доступна только первому (или назначенному) пользователю — 403 это нормально.
ADMIN_CODE="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 -b "$JAR" "$BASE/admin")"
if [ "$ADMIN_CODE" = "200" ]; then ok "админка ($ADMIN_CODE, тестовый ник — админ)"
elif [ "$ADMIN_CODE" = "403" ]; then ok "админка закрыта для не-админа (403)"
else bad "админка: получил $ADMIN_CODE, ждал 200 или 403"; fi

echo "---"
if [ "$FAILS" -eq 0 ]; then
  echo "Все проверки пройдены (тестовый ник: $NICK)"
else
  echo "Провалено проверок: $FAILS (тестовый ник: $NICK)"
fi
rm -f "$JAR"
exit "$FAILS"
