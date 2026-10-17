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
if printf '%s' "$PLANS" | grep -q "08-zaskoruzlye-piramidy"; then ok "каркас виден в списке"; else bad "каркаса нет в списке"; fi

LOC="$(curl -sS -o /dev/null -w '%{redirect_url}' --max-time 20 -b "$JAR" -c "$JAR" \
  -d "title=Смоук&emoji=🩸&preset=08-zaskoruzlye-piramidy" "$BASE/plans")"
# curl отдаёт Location уже склеенным с базой — приводим к полному URL.
case "$LOC" in
  /*) LOC="$BASE$LOC" ;;
esac
if printf '%s' "$LOC" | grep -q "/plans/"; then ok "план создан из каркаса ($LOC)"; else bad "план не создался (redirect: $LOC)"; fi

EDITOR="$(curl -sS --max-time 20 -b "$JAR" "$LOC")"
for needle in 'id="kt-plan"' 'id="kt-sections"' "пинг систем" "великий успех"; do
  if printf '%s' "$EDITOR" | grep -q "$needle"; then ok "в конструкторе есть $needle"; else bad "в конструкторе нет $needle"; fi
done
# Регрессия: списки должны приезжать массивами, иначе кнопки конструктора падают.
# Пустым «picks» уже не бывает: каркас сам добавляет обязательные позиции.
if printf '%s' "$EDITOR" | grep -q '"picks":\['; then
  ok "покупки приезжают массивом"
else
  bad "в данных плана нет массива \"picks\" (кнопки могут не нажиматься)"
fi
if printf '%s' "$EDITOR" | grep -qE '"(picks|deferred|shop|ladders)":null'; then
  bad "в данных плана есть null вместо массива"
else
  ok "null вместо массивов нет"
fi
# Каркас жизни 8: обязательные позиции и условие встречи с Джоном Сноу.
if printf '%s' "$EDITOR" | grep -q '"already_paid":true'; then
  ok "уже купленный ею урок помечен already_paid"
else
  bad "в каркасе нет already_paid (первый урок уже куплен)"
fi
if printf '%s' "$EDITOR" | grep -q '"locked":true'; then
  ok "обязательные позиции помечены locked"
else
  bad "в каркасе нет locked"
fi
if printf '%s' "$EDITOR" | grep -q '"requirements"'; then
  ok "условия каркаса приехали"
else
  bad "в каркасе нет requirements"
fi
if printf '%s' "$EDITOR" | grep -q '"reward_after"'; then
  ok "награда за порядок приехала в каркасе"
else
  bad "в каркасе нет reward_after («последний бой» после «пинг систем»)"
fi
# Повторяемые покупки: «лестница в небо» дважды, «взять 1 фолиант» до трёх раз.
if printf '%s' "$EDITOR" | grep -q '"repeat":2' && printf '%s' "$EDITOR" | grep -q '"repeat":3'; then
  ok "повторяемые покупки помечены repeat"
else
  bad "в каркасе нет repeat (лестница в небо, взять фолиант)"
fi
# Отметки «активирую» и «беру» заслуживаются: у выкупа есть условие-урок,
# у накопителя — покупка, у фолианта — урок и выкуп.
if printf '%s' "$EDITOR" | grep -q '"requires":\["lesson:metod_neproglyadnogo_tumana"\]'; then
  ok "выкуп фолианта требует урок метода"
else
  bad "у выкупа фолианта нет requires с уроком метода"
fi
if printf '%s' "$EDITOR" | grep -q '"requires":\["item:hardkor"\]'; then
  ok "накопитель «хардкор» требует покупку"
else
  bad "у накопителя «хардкор» нет requires"
fi
if printf '%s' "$EDITOR" | grep -q '"requires":\["lesson:metod_neproglyadnogo_tumana","folio:vykupit_tuman"\]'; then
  ok "фолиант берут после урока метода и выкупа"
else
  bad "у фолианта нет requires (урок метода + выкуп)"
fi
# Цены сверены с планами игроков: оплата перерождения Джона Сноу — 200 XP.
if printf '%s' "$EDITOR" | grep -q 'item:pererozhdenie_dzhona'; then
  ok "оплата перерождения Джона Сноу есть в каркасе"
else
  bad "в каркасе нет оплаты перерождения Джона Сноу"
fi
TXT0="$(curl -sS --max-time 20 -b "$JAR" "$LOC/export?format=txt")"
if printf '%s' "$TXT0" | grep -q "уже куплено"; then
  ok "в экспорте есть пометка «уже куплено»"
else
  bad "в экспорте нет пометки «уже куплено»"
fi
# Опыт за уже взятые позиции списывается: первый урок стоит 5 XP, нумерация идёт со второго.
if printf '%s' "$TXT0" | grep -q "1. поцелуй Мелисандры: 5 (уже куплено)"; then
  ok "первый урок занимает номер и списывает 5 XP"
else
  bad "первый урок посчитан неверно: $(printf '%s' "$TXT0" | grep -m1 'поцелуй Мелисандры')"
fi
if printf '%s' "$TXT0" | grep -q "отключение тренировок: 0 (уже куплено)"; then
  ok "уже взятая заплатка в плане и помечена"
else
  bad "заплатки «отключение тренировок» нет в плане"
fi
# Проваленные проверки видны сразу: у свежего плана «пинг систем» стоит вторым,
# а по тексту он минимум десятый.
if printf '%s' "$TXT0" | grep -q "⚠ Проверки не выполняются"; then
  ok "экспорт предупреждает, что проверки не выполняются"
else
  bad "в экспорте нет строки «⚠ Проверки не выполняются»"
fi
# Условие про Джона Сноу — памятка на будущие жизни, а не проверка.
if printf '%s' "$TXT0" | grep -q "На будущие жизни:" && printf '%s' "$TXT0" | grep -q "Джона Сноу"; then
  ok "условие про Джона Сноу уехало в памятку «На будущие жизни»"
else
  bad "условие про Джона Сноу не попало в памятку"
fi
if printf '%s' "$TXT0" | grep -q "условие каркаса не выполнено — чтобы снова встретить"; then
  bad "условие на будущее всё ещё валит проверки"
else
  ok "условие на будущее проверки не валит"
fi

TXT="$(curl -sS --max-time 20 -b "$JAR" "$LOC/export?format=txt")"
# План только что создан и покупок в нём нет, поэтому проверяем каркас экспорта.
for needle in "Невыученные" "Остаток" "Итого потрачено" "[*]"; do
  if printf '%s' "$TXT" | grep -qF "$needle"; then ok "в экспорте есть «$needle»"; else bad "в экспорте нет «$needle»"; fi
done
# Отметки накопителей и фолиантов — отдельные блоки, как требует формат плана.
if printf '%s' "$TXT" | grep -q "Активация накопителей:"; then
  ok "в экспорте есть блок «Активация накопителей»"
else
  bad "в экспорте нет блока про активацию накопителей"
fi

check "страница экспорта" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 -b "$JAR" "$LOC/export")" "200"
PAGE="$(curl -sS --max-time 20 -b "$JAR" "$LOC/export")"
if printf '%s' "$PAGE" | grep -q 'export-warn'; then
  ok "на странице экспорта висит предупреждение о проверках"
else
  bad "на странице экспорта нет предупреждения о проверках"
fi
if printf '%s' "$PAGE" | grep -q "На будущие жизни"; then
  ok "на странице экспорта есть памятка «На будущие жизни»"
else
  bad "на странице экспорта нет памятки «На будущие жизни»"
fi
check "справка по механике" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "$BASE/guide")" "200"
check "каркас отдаётся файлом" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "$BASE/presets/08-zaskoruzlye-piramidy")" "200"
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
