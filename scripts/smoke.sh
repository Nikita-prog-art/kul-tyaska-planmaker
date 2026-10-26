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

# После выкладки туннель и приложение поднимаются не мгновенно: если начать
# сразу, весь прогон падает на 502 от края, хотя сервер в порядке. Ждём health.
READY=0
for _ in $(seq 1 20); do
  if [ "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$BASE/healthz" || true)" = "200" ]; then
    READY=1
    break
  fi
  sleep 2
done
if [ "$READY" = "1" ]; then ok "сайт отвечает"; else bad "сайт не поднялся за 40 секунд"; fi

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
# Библиотека: право взять фолиант и право взять его с доступом на Накоплении Ци —
# две разные покупки (5 и 10 XP), а не одна.
if printf '%s' "$EDITOR" | grep -q '"key":"folio:vzyat_1_foliant_nakoplenie"'; then
  ok "в каркасе есть отдельное право «1 фолиант на Накоплении Ци»"
else
  bad "в каркасе нет права «взять 1 фолиант на Накоплении Ци»"
fi
# Слотов «невыученных» два: второй она открыла раньше уроком.
if printf '%s' "$EDITOR" | grep -q '"deferred_slots":2'; then
  ok "в каркасе два слота невыученных"
else
  bad "в каркасе не два слота невыученных"
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
# Покупки меняют накопитель: заряд, максимум и скорость зарядки.
if printf '%s' "$EDITOR" | grep -q '"effects":\[{"accumulator":"acc:hardkor","charges":1}\]'; then
  ok "покупка «хардкор» заряжает накопитель"
else
  bad "у «хардкора» нет effects с зарядом"
fi
if printf '%s' "$EDITOR" | grep -q '"accumulator":"acc:gorozhane","rate":25'; then
  ok "скорость зарядки горожан задаётся покупкой"
else
  bad "у «скорости зарядки горожан» нет effects с rate"
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
if printf '%s' "$TXT0" | grep -q "1. Мелисандра: 5 (уже куплено)"; then
  ok "первый урок занимает номер и списывает 5 XP"
else
  bad "первый урок посчитан неверно: $(printf '%s' "$TXT0" | grep -m1 -E '^Уроки: ') "
fi
if printf '%s' "$TXT0" | grep -q "Заплатки: без тренировок: 0 (уже куплено)"; then
  ok "уже взятая заплатка в плане и помечена"
else
  bad "заплатки «без тренировок» нет в плане"
fi
# Названия в плане короткие: длинных формулировок из текста посмертия там нет.
if printf '%s' "$TXT0" | grep -q "поцелуй Мелисандры"; then
  bad "в экспорте длинное название урока вместо короткого"
else
  ok "позиции названы коротко"
fi
# Проваленные проверки видны сразу: у свежего плана «пинг систем» стоит вторым,
# а по тексту он минимум десятый.
if printf '%s' "$TXT0" | grep -q "⚠ Проверки не выполняются"; then
  ok "экспорт предупреждает, что проверки не выполняются"
else
  bad "в экспорте нет строки «⚠ Проверки не выполняются»"
fi
# Условие про Джона Сноу — памятка в конструкторе, а не в плане.
if printf '%s' "$TXT0" | grep -q "На будущие жизни"; then
  bad "в экспорте осталась памятка «На будущие жизни»"
else
  ok "памятки «На будущие жизни» в плане нет"
fi
if printf '%s' "$TXT0" | grep -q "Каркас по посмертию"; then
  bad "в экспорт попали заметки каркаса"
else
  ok "заметок каркаса в плане нет"
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
# Отметки накопителей и фолиантов печатаются только для того, что берём:
# в свежем плане активировать нечего, поэтому ни заголовка, ни списков нет.
if printf '%s' "$TXT" | grep -q "Активация накопителей:"; then
  bad "в экспорте пустой блок «Активация накопителей»"
else
  ok "блок активации не печатается, когда активировать нечего"
fi
if printf '%s' "$TXT" | grep -q "не активирую"; then
  bad "в экспорте остался список «не активирую»"
else
  ok "списка «не активирую» в плане нет"
fi
if printf '%s' "$TXT" | grep -q "оставляю в библиотеке"; then
  bad "в экспорте остался список «оставляю в библиотеке»"
else
  ok "списка «оставляю в библиотеке» в плане нет"
fi
# Уроки идут одной строкой, как требует шаблон плана.
if printf '%s' "$TXT" | grep -q "^Уроки: 1\. "; then
  ok "уроки напечатаны одной нумерованной строкой"
else
  bad "уроки не уложены в одну строку"
fi

check "страница экспорта" "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 -b "$JAR" "$LOC/export")" "200"
PAGE="$(curl -sS --max-time 20 -b "$JAR" "$LOC/export")"
if printf '%s' "$PAGE" | grep -q 'export-warn'; then
  ok "на странице экспорта висит предупреждение о проверках"
else
  bad "на странице экспорта нет предупреждения о проверках"
fi
if printf '%s' "$PAGE" | grep -q "На будущие жизни"; then
  bad "на странице экспорта осталась памятка «На будущие жизни»"
else
  ok "на странице экспорта нет лишней памятки"
fi
# Переносы строк делает показ, а не текст: у поля выгрузки мягкий перенос.
if printf '%s' "$PAGE" | grep -q 'id="export-text"'; then
  ok "поле выгрузки на странице есть"
else
  bad "на странице экспорта нет поля выгрузки"
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
