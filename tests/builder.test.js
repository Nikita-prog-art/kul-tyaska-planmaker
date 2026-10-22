#!/usr/bin/env node
/* DOM-тест конструктора: гоняем builder.js в jsdom и щёлкаем по кнопкам,
   как это делал бы человек. Проверяем нумерацию уроков, пересчёт сумм,
   шкалы, «не беру», экспорт и сохранение состояния.
   jsdom лежит в .cache (см. scripts/goenv.sh), ставится командой:
     npm install --prefix .cache/jsdtest --cache .cache/npm-cache jsdom */
'use strict';

const fs = require('fs');
const path = require('path');
const assert = require('assert');

const root = path.join(__dirname, '..');
const jsdomPath = path.join(root, '.cache/jsdtest/node_modules/jsdom');
if (!fs.existsSync(jsdomPath)) {
  console.error('jsdom не установлен — пропускаю DOM-тест.');
  console.error('  npm install --prefix .cache/jsdtest --cache .cache/npm-cache jsdom');
  process.exit(0);
}

const { JSDOM } = require(jsdomPath);
const KT = require(path.join(root, 'internal/web/static/engine.js'));
const plan = JSON.parse(fs.readFileSync(path.join(root, 'internal/plan/testdata/plan.json'), 'utf8'));
const sections = JSON.parse(fs.readFileSync(path.join(root, 'internal/plan/testdata/sections.json'), 'utf8'));
const presets = [{ name: 'Жизнь 8 · Заскорузлые пирамиды', file: '08-zaskoruzlye-piramidy', balance: 1605.24 }];

// pageHtml собирает такую же страницу, какую отдаёт шаблон plan.jinja.
function pageHtml(p) {
  return `<!doctype html><html lang="ru"><body>
  <div class="builder" id="builder"></div>
  <form id="kt-save" method="post" action="/plans/${p.id}"><input type="hidden" name="state" id="kt-state"></form>
  <script type="application/json" id="kt-plan">${JSON.stringify(p)}</script>
  <script type="application/json" id="kt-sections">${JSON.stringify(sections)}</script>
  <script type="application/json" id="kt-presets">${JSON.stringify(presets)}</script>
</body></html>`;
}

// bootPage поднимает страницу в jsdom и выполняет оба скрипта конструктора.
function bootPage(p) {
  const page = new JSDOM(pageHtml(p), { runScripts: 'outside-only', url: 'https://kultyaska.test/plans/' + p.id });
  const w = page.window;
  w.confirm = () => true;
  w.alert = (msg) => { throw new Error('alert: ' + msg); };
  w.HTMLFormElement.prototype.submit = function () {
    w.__submitted = w.document.getElementById('kt-state').value;
  };
  w.eval(fs.readFileSync(path.join(root, 'internal/web/static/engine.js'), 'utf8'));
  w.eval(fs.readFileSync(path.join(root, 'internal/web/static/builder.js'), 'utf8'));
  return w;
}

const html = pageHtml(plan);

const dom = new JSDOM(html, { runScripts: 'outside-only', url: 'https://kultyaska.test/plans/' + plan.id });
const { window } = dom;
const { document } = window;
window.confirm = () => true;
window.alert = (msg) => { throw new Error('alert: ' + msg); };

let submitted = null;
window.HTMLFormElement.prototype.submit = function () { submitted = document.getElementById('kt-state').value; };

window.eval(fs.readFileSync(path.join(root, 'internal/web/static/engine.js'), 'utf8'));
window.eval(fs.readFileSync(path.join(root, 'internal/web/static/builder.js'), 'utf8'));

// --- помощники ---------------------------------------------------------

function rowByTitle(title) {
  return rowByTitleIn(document, title);
}

// rowByTitleIn ищет строку магазина в конкретном документе: страниц в тесте много.
function rowByTitleIn(doc, title) {
  const rows = doc.querySelectorAll('.row.item');
  for (const row of rows) {
    const t = row.querySelector('.item-title');
    if (t && t.textContent.indexOf(title) >= 0) return row;
  }
  throw new Error('не нашёл строку «' + title + '»');
}

// attachByRe ищет строку панели «Накопители и фолианты».
function attachByRe(doc, re) {
  const items = doc.querySelectorAll('#kt-attach .attach');
  for (const item of items) {
    if (re.test(item.textContent)) return item;
  }
  throw new Error('не нашёл отметку ' + re);
}

// pickIn нажимает «беру» у строки магазина на отдельной странице.
function pickIn(doc, title) {
  const row = rowByTitleIn(doc, title);
  const btn = row.querySelector('[data-act="pick"]');
  if (!btn) throw new Error('у строки «' + title + '» нет кнопки «беру»');
  btn.click();
}

function click(sel, where) {
  const scope = where || document;
  const el = scope.querySelector(sel);
  if (!el) throw new Error('не нашёл элемент ' + sel);
  el.click();
  return el;
}

function text(sel) {
  const el = document.querySelector(sel);
  return el ? el.textContent : null;
}

const state = () => JSON.parse(document.getElementById('kt-state').value || 'null');
const exportText = () => document.getElementById('kt-export-text').value;

let failures = 0;
function test(name, fn) {
  try {
    fn();
    console.log('  ✓ ' + name);
  } catch (e) {
    failures++;
    console.error('  ✗ ' + name + '\n    ' + (e.message || e).toString().split('\n').join('\n    '));
  }
}

console.log('Конструктор в jsdom:');

test('секции и строки магазина отрисованы', () => {
  assert.ok(document.querySelectorAll('.sec').length >= 5, 'секции есть');
  assert.ok(rowByTitle('пинг систем'), 'урок из каркаса виден');
  const talent = document.querySelector('.row.ladder[data-ladder="talent.die"]');
  assert.ok(talent && talent.textContent.indexOf('кость таланта') >= 0, 'шкала таланта видна');
  // Скрытые шкалы (например, «один на миллион» из прошлых жизней) не рисуются.
  const visibleLadders = plan.ladders.filter((l) => !l.hidden).length;
  assert.strictEqual(document.querySelectorAll('.ladder').length, visibleLadders, 'все видимые шкалы на месте');
});

test('стартовые суммы совпадают с расчётом движка', () => {
  const expected = KT.computeSummary(plan, sections);
  assert.strictEqual(text('#sum-spent'), KT.fmt(expected.total, true));
  assert.strictEqual(text('#sum-left'), KT.fmt(expected.remainder, true));
});

test('уроки пронумерованы, фиксированная цена номер не занимает', () => {
  const badges = Array.prototype.map.call(document.querySelectorAll('.row.item.picked .badge'), (b) => b.textContent)
    .filter((v) => v !== '');
  assert.ok(badges.indexOf('1') >= 0, 'есть первый урок');
  const fixed = rowByTitle('профессиональная Дева-в-Беде');
  assert.strictEqual(fixed.querySelector('.badge').textContent, '', 'фиксированный урок без номера');
});

test('уже взятая позиция не снимается, а нумерация идёт со второго урока', () => {
  const row = rowByTitle('поцелуй Мелисандры');
  assert.strictEqual(row.querySelector('.badge').textContent, '1', 'первый номер — у уже купленного урока');
  ['pick', 'defer', 'skip'].forEach((act) => {
    const btn = row.querySelector('.state.' + act);
    if (btn) assert.strictEqual(btn.disabled, true, 'кнопка «' + act + '» заблокирована');
  });
  row.querySelector('.state.pick').click(); // по disabled-кнопке клик не проходит
  const st = stateText();
  assert.ok(st.picks.some((p) => /poceluy/.test(p.key)), 'позиция осталась в плане');

  const summary = KT.computeSummary(st, sections);
  const lessons = summary.sections.filter((s) => s.key === 'lessons')[0];
  const first = lessons.lines.filter((l) => l.number === 1)[0];
  assert.strictEqual(first.cost, 5, 'опыт за уже купленный урок всё равно списывается');
  assert.strictEqual(first.note, 'уже куплено');
  assert.strictEqual(lessons.lines.filter((l) => l.number === 2)[0].cost, 10, 'следующий урок стоит 10 XP');
});

test('уже взятая заплатка в плане и помечена «уже куплено»', () => {
  const row = rowByTitle('отключение тренировок');
  assert.ok(row, 'заплатка видна');
  assert.strictEqual(row.querySelector('.state.skip').disabled, true, 'снять её нельзя');
  row.querySelector('.state.skip').click();
  const st = state();
  assert.ok(st.picks.some((p) => /otkluchenie_trenirovok/.test(p.key)), 'заплатка осталась в плане');

  const summary = KT.computeSummary(st, sections);
  const patches = summary.sections.filter((s) => s.key === 'patches')[0];
  const line = patches.lines.filter((l) => /otkluchenie/.test(l.key))[0];
  assert.ok(line, 'заплатка попала в расчёт');
  assert.strictEqual(line.note, 'уже куплено');
  assert.ok(exportText().indexOf('отключение тренировок') >= 0, 'заплатка есть в выгрузке');
});

test('обязательная позиция каркаса всегда в плане', () => {
  const row = rowByTitle('пинг систем');
  assert.strictEqual(row.querySelector('.state.skip').disabled, true, '«не беру» для обязательной позиции закрыто');
  assert.ok(row.textContent.indexOf('обязательно') >= 0, 'у позиции есть пометка');
});

test('условие каркаса про Джона Сноу видно и проверяется', () => {
  const before = document.getElementById('kt-req');
  assert.ok(before && before.textContent.indexOf('Джона Сноу') >= 0, 'условие показано');
  assert.ok(before.querySelector('.unmet'), 'сначала условие не выполнено');

  rowByTitle('контроль пола').querySelector('.state.pick').click();
  const after = document.getElementById('kt-req');
  assert.ok(after.querySelector('.met'), 'после «контроля пола» условие выполнено');
  assert.strictEqual(after.querySelectorAll('.unmet').length, 0, 'невыполненных условий не осталось');
});

test('«беру» добавляет урок, цена следующего растёт на 5', () => {
  const before = KT.computeSummary(plan, sections);
  const row = rowByTitle('сама себе безоар');
  assert.strictEqual(row.querySelector('.state.pick').className.indexOf('on'), -1, 'урок ещё не выбран');
  row.querySelector('.state.pick').click();
  const st = stateText();
  const after = KT.computeSummary(st, sections);
  assert.ok(after.total > before.total, 'сумма выросла');
  assert.ok(st.picks.some((p) => /sama_sebe/.test(p.key)), 'урок попал в picks');
  assert.strictEqual(after.paidLessons, before.paidLessons + 1, 'урок встал в очередь');
});

test('«невыученный» освобождает урок от оплаты', () => {
  const row = rowByTitle('да сдохни же ты наконец!');
  row.querySelector('.state.defer').click();
  const st = stateText();
  assert.ok(st.deferred.some((d) => /da_sdohni/.test(d.key)), 'урок ушёл в отложенные');
  assert.ok(!st.picks.some((p) => /da_sdohni/.test(p.key)), 'и убрался из покупок');
  assert.ok(exportText().indexOf('невыученный, в следующую жизнь') >= 0, 'в экспорте помечен как невыученный');
});

test('«не беру» попадает в раздел «Не взято»', () => {
  const row = rowByTitle('дилетантка широкого профиля');
  assert.strictEqual(row.className.indexOf('skipped'), -1, 'урок пока не отмечен как не взятый');
  row.querySelector('.state.skip').click();
  const st = stateText();
  assert.ok(st.not_taken.some((k) => /diletantka/.test(k)), 'урок помечен «не беру»');
  assert.ok(exportText().indexOf('Не взято:') >= 0, 'раздел появился');
  assert.ok(exportText().indexOf('дилетантка широкого профиля') >= 0, 'урок перечислен');
});

test('шкала таланта считается по шагам', () => {
  const row = document.querySelector('.row.ladder[data-ladder="talent.die"]');
  const before = Number(row.querySelector('.ladder-value').value);
  const costBefore = KT.ladderCost({ from: 1, step: 1, value: before, cost: { type: 'to_value', rate: 1 } });
  row.querySelectorAll('.ladder-ctl .btn')[1].click(); // кнопка «+»
  const row2 = document.querySelector('.row.ladder[data-ladder="talent.die"]');
  const after = Number(row2.querySelector('.ladder-value').value);
  assert.strictEqual(after, before + 1, 'значение выросло на шаг');
  const costAfter = KT.ladderCost({ from: 1, step: 1, value: after, cost: { type: 'to_value', rate: 1 } });
  assert.strictEqual(costAfter - costBefore, after, 'шаг стоит столько, сколько граней у кости');
  assert.ok(row2.querySelector('[data-cost]').textContent.length > 0, 'цена пересчитана');
});

test('поиск фильтрует строки', () => {
  const box = document.getElementById('kt-search');
  box.value = 'метод';
  box.dispatchEvent(new window.Event('input', { bubbles: true }));
  const visible = Array.prototype.filter.call(document.querySelectorAll('.row.item'), (r) => !r.hidden);
  assert.ok(visible.length > 0, 'что-то осталось');
  assert.ok(visible.every((r) => r.querySelector('.item-title').textContent.toLowerCase().indexOf('метод') >= 0),
    'остались только совпадения');
  box.value = '';
  box.dispatchEvent(new window.Event('input', { bubbles: true }));
});

test('«Сохранить» кладёт состояние в форму', () => {
  document.querySelector('.toolbar .btn:not(.ghost)').click();
  const st = state();
  assert.ok(st, 'состояние непустое');
  assert.strictEqual(st.title, plan.title);
  assert.ok(Array.isArray(st.shop) && st.shop.length === plan.shop.length, 'магазин уехал целиком');
  assert.ok(Array.isArray(st.picks) && st.picks.length > 0, 'покупки на месте');
  assert.ok(Array.isArray(st.ladders) && st.ladders.length > 0, 'шкалы на месте');
  assert.ok(typeof st.balance === 'number', 'баланс числом');
});

test('каркас скачивается и загружается обратно', () => {
  click('.toolbar-actions .btn.ghost'); // «Каркас» — открыть панель
  const area = document.getElementById('kt-skeleton-text');
  assert.ok(area.value.indexOf('"shop"') >= 0, 'в панели JSON каркаса');
  const before = stateText();
  const parsed = JSON.parse(area.value);
  parsed.balance = 5000;
  area.value = JSON.stringify(parsed);
  // «Загрузить в план» — вторая кнопка в панели
  const buttons = document.querySelectorAll('#kt-import .row.gap.wrap .btn');
  buttons[0].click();
  assert.strictEqual(document.getElementById('kt-balance').value.trim(), '5000', 'баланс подхватился');
  const after = stateText();
  assert.strictEqual(after.picks.length, before.picks.length, 'покупки сохранились при загрузке каркаса');
  assert.deepStrictEqual(after.not_taken, before.not_taken, '«не беру» переживает загрузку каркаса');
});

let currentSummary = null;
function stateText() {
  // Пересчитываем из текущего DOM-состояния: клики меняют план внутри builder.js,
  // поэтому берём его через сохранение в форму (без отправки).
  submitted = null;
  document.querySelector('.toolbar .btn:not(.ghost)').click();
  return state();
}

// Регрессия: только что созданный план уезжает с сервера со списками-null
// (так сериализуется nil-срез в Go), и кнопки должны всё равно работать.
test('план с null вместо пустых списков не ломает конструктор', () => {
  const fresh = JSON.parse(JSON.stringify(plan));
  fresh.picks = null;
  fresh.deferred = null;
  fresh.not_taken = null;
  fresh.accumulators = null;
  fresh.folios = null;
  fresh.multipliers = null;
  fresh.ladders = null;

  const w = bootPage(fresh);
  const d = w.document;
  assert.ok(d.querySelectorAll('.row.item').length > 0, 'строки магазина отрисовались');

  const row = Array.prototype.find.call(d.querySelectorAll('.row.item'), (r) =>
    r.querySelector('.item-title').textContent.indexOf('ученье — свет!') >= 0);
  assert.ok(row, 'нашёл урок «ученье — свет!»');

  row.querySelector('.state.pick').click(); // до починки здесь падало на plan.picks.some
  d.querySelector('.toolbar .btn:not(.ghost)').click(); // «Сохранить» — состояние уходит в форму
  const st = JSON.parse(d.getElementById('kt-state').value || 'null');
  assert.ok(st && Array.isArray(st.picks), 'состояние сохраняется массивом');
  assert.ok(st.picks.some((p) => /uchene_svet/.test(p.key)), 'выбранный урок записался');
  assert.ok(st.picks.some((p) => /poceluy/.test(p.key)), 'обязательная позиция каркаса тоже на месте');
  assert.ok(d.getElementById('sum-spent').textContent.length > 0, 'суммы пересчитались');
});

// --- награды за порядок, «уже есть» у шкал и предупреждение при экспорте ---

test('награда за порядок: видна потеря и чинится кнопкой', () => {
  const p = JSON.parse(JSON.stringify(plan));
  p.picks = [
    { key: 'lesson:poceluy_melisandry' },
    { key: 'lesson:posledniy_boy' },
    { key: 'lesson:ping_sistem' }
  ];
  p.deferred = [];
  p.not_taken = [];
  const d = bootPage(p).document;

  const row = rowByTitleIn(d, 'последний бой');
  assert.ok(row, 'строка «последний бой» есть');
  assert.ok(row.querySelector('.reward-hint.lost'), 'видно, что награда теряется');
  assert.ok(row.textContent.indexOf('квестовая награда') >= 0, 'награда названа');
  assert.ok(row.textContent.indexOf('+4%') >= 0, 'описано, что даёт награда');

  const fix = Array.prototype.find.call(row.querySelectorAll('button'), (b) => /поставить после/.test(b.textContent));
  assert.ok(fix, 'есть кнопка «поставить после»');
  fix.click();

  const after = rowByTitleIn(d, 'последний бой');
  assert.ok(after.querySelector('.reward-hint.got'), 'после перестановки награда получена');
  const rewards = d.getElementById('kt-rewards');
  assert.ok(rewards && !rewards.hidden, 'награда попала в список наград');
  assert.ok(d.getElementById('kt-export-text').value.indexOf('квестовая награда') >= 0, 'награда есть в выгрузке');
});

test('обязательный урок двигается вручную, уже взятый — нет', () => {
  const p = JSON.parse(JSON.stringify(plan));
  p.picks = [
    { key: 'lesson:poceluy_melisandry' },
    { key: 'lesson:posledniy_boy' },
    { key: 'lesson:ping_sistem' }
  ];
  p.deferred = [];
  p.not_taken = [];
  const d = bootPage(p).document;

  assert.strictEqual(rowByTitleIn(d, 'поцелуй Мелисандры').querySelector('.item-order'), null,
    'уже взятый ею урок не двигается: его место задано текстом');
  const ping = rowByTitleIn(d, 'пинг систем');
  assert.ok(ping.querySelector('.item-order'), 'обязательный урок можно двигать вручную');
  const before = ping.querySelector('.badge').textContent;
  ping.querySelectorAll('.item-order .btn')[0].click(); // «выше»
  const after = rowByTitleIn(d, 'пинг систем').querySelector('.badge').textContent;
  assert.notStrictEqual(after, before, 'номер урока изменился после перестановки');
});

test('шкала показывает, что уже есть, и считает только прирост', () => {
  const d = bootPage(JSON.parse(JSON.stringify(plan))).document;
  const row = d.querySelector('.row.ladder[data-ladder="talent.die"]');
  const owned = row.querySelector('.ladder-owned');
  assert.ok(owned, 'есть поле «уже есть»');
  assert.strictEqual(Number(owned.value), 22, 'по каркасу кость уже d22');
  assert.ok(row.textContent.indexOf('уже есть') >= 0, 'подпись на месте');

  // В цену входит только прирост от того, что уже есть: d23 + d24 = 47 XP,
  // а не все ступени с d1 (это было бы 324 XP).
  const value = Number(row.querySelector('.ladder-value').value);
  const cost = Number(row.querySelector('[data-cost="talent.die"]').textContent);
  assert.strictEqual(cost, KT.ladderCost({ from: 22, step: 1, value: value, cost: { type: 'to_value', rate: 1 } }),
    'цена — это прирост от «уже есть»');
  assert.ok(cost < 100, 'ступени с d1 в цену не попали');

  row.querySelectorAll('.ladder-ctl .btn')[1].click(); // «+» — покупаем следующую ступень
  const after = d.querySelector('.row.ladder[data-ladder="talent.die"]');
  const nextValue = Number(after.querySelector('.ladder-value').value);
  assert.strictEqual(nextValue, value + 1, 'значение выросло на шаг');
  assert.strictEqual(Number(after.querySelector('[data-cost="talent.die"]').textContent),
    KT.ladderCost({ from: 22, step: 1, value: nextValue, cost: { type: 'to_value', rate: 1 } }),
    'шаг стоит свой номер граней');
});

test('проваленные проверки предупреждают прямо в панели экспорта', () => {
  const warn = document.getElementById('kt-export-warn');
  assert.ok(warn && !warn.hidden, 'предупреждение показано');
  assert.ok(/Проверки не выполняются/.test(warn.textContent), 'сказано, что проверки не выполняются');
  assert.ok(exportText().indexOf('⚠ Проверки не выполняются') > 0, 'и в тексте плана тоже');
  assert.ok(document.getElementById('kt-warnings').textContent.indexOf('Джона Сноу') === -1,
    'условие на будущее не попадает в проверки');
  assert.ok(document.getElementById('kt-req').textContent.indexOf('На будущие жизни') >= 0,
    'условие показано отдельной памяткой');
});

function rowByTitleIn(doc, title) {
  for (const row of doc.querySelectorAll('.row.item')) {
    const t = row.querySelector('.item-title');
    if (t && t.textContent.indexOf(title) >= 0) return row;
  }
  return null;
}

// --- порядок уроков, количество покупок, отметки накопителей -------------

/* lessonTitles — названия уроков в том порядке, в каком они стоят в DOM. */
function lessonTitles(doc) {
  return Array.prototype.filter.call(doc.querySelectorAll('#sec-lessons .row.item'), (r) => {
    const b = r.querySelector('.badge');
    return b && b.textContent !== '';
  }).map((r) => r.querySelector('.item-title').textContent.replace(/\s+/g, ' ').trim());
}

/* titleIndex — где в списке стоит строка, содержащая подстроку. */
function titleIndex(titles, needle) {
  for (let i = 0; i < titles.length; i++) if (titles[i].indexOf(needle) >= 0) return i;
  return -1;
}

test('порядок строк в уроках повторяет очередь покупок', () => {
  const d = bootPage(plan).document;
  const titles = lessonTitles(d);
  assert.ok(titles.length >= 5, 'уроки отрисованы');
  assert.ok(titles[0].indexOf('поцелуй') === 0, 'первым стоит уже купленный ею урок');

  const badges = Array.prototype.map.call(
    d.querySelectorAll('#sec-lessons .row.item .badge'),
    (b) => b.textContent
  ).filter((v) => v !== '').map(Number);
  assert.deepStrictEqual(badges, badges.slice().sort((a, b) => a - b), 'номера идут сверху вниз');

  // «выше» у подвижного урока переставляет саму строку, а не только число.
  const before = titleIndex(lessonTitles(d), 'пинг систем');
  assert.ok(before > 1, 'пинг систем стоит не первым');
  rowByTitleIn(d, 'пинг систем').querySelectorAll('.item-order .btn')[0].click();
  const after = titleIndex(lessonTitles(d), 'пинг систем');
  assert.strictEqual(after, before - 1, 'строка переехала на место выше');

  // Уже взятый ею урок с места не двигается: он остаётся первым.
  assert.ok(lessonTitles(d)[0].indexOf('поцелуй') === 0, 'поцелуй Мелисандры остался первым');
});

test('повторяемую покупку можно взять дважды', () => {
  const w = bootPage(JSON.parse(JSON.stringify(plan)));
  const d = w.document;
  const row = rowByTitleIn(d, 'лестница в небо');
  assert.strictEqual(row.querySelector('.qty-value').textContent, '×1');
  assert.strictEqual(d.querySelectorAll('[data-qty="item:lestnica_v_nebo"]').length, 1, 'счётчик количества на месте');

  const spent = Number(d.getElementById('sum-spent').textContent.replace(',', '.'));
  row.querySelectorAll('.item-qty .btn')[1].click(); // «+»
  const after = rowByTitleIn(d, 'лестница в небо');
  assert.strictEqual(after.querySelector('.qty-value').textContent, '×2', 'вторая покупка отмечена');
  const spent2 = Number(d.getElementById('sum-spent').textContent.replace(',', '.'));
  assert.strictEqual(spent2 - spent, 5, 'вторая лестница стоит ещё 5 XP');

  Array.prototype.find.call(d.querySelectorAll('.btn'), (b) => b.textContent === 'Сохранить').click();
  const saved = JSON.parse(w.__submitted);
  const pick = saved.picks.filter((p) => p.key === 'item:lestnica_v_nebo')[0];
  assert.strictEqual(pick.qty, 2, 'количество уезжает в сохранение');

  // Третьей покупки не бывает: у позиции предел 2.
  rowByTitleIn(d, 'лестница в небо').querySelectorAll('.item-qty .btn')[1].click();
  assert.strictEqual(rowByTitleIn(d, 'лестница в небо').querySelector('.qty-value').textContent, '×2');
});

test('накопители и фолианты отмечаются отдельно от магазина', () => {
  const d = bootPage(JSON.parse(JSON.stringify(plan))).document;
  const card = d.querySelector('#kt-attach');
  assert.ok(card, 'панель отметок отрисована');

  const find = (re) => Array.prototype.find.call(card.querySelectorAll('.attach'), (l) => re.test(l.textContent));
  assert.strictEqual(find(/хардкор/).querySelector('input').checked, true, 'купленный накопитель уже отмечен к активации');
  assert.strictEqual(find(/равновесие/).querySelector('input').checked, false, 'некупленный — не отмечен');

  // Фолиант, который берут с собой, отмечается вручную и виден в выгрузке.
  const tuman = find(/Непроглядного Тумана/);
  assert.strictEqual(tuman.querySelector('input').checked, true, 'фолиант отмечен к переносу');
  tuman.querySelector('input').click();
  const txt = d.getElementById('kt-export-text').value;
  assert.ok(!/беру с собой:[^\n]*Непроглядного/.test(txt), 'ушёл из «беру с собой»');
  assert.ok(/оставляю в библиотеке:[^\n]*Непроглядного/.test(txt), 'попал в «оставляю в библиотеке»');
});

test('«минимум n-м уроком» зеленеет, когда условие выполнено', () => {
  // «дилетантка широкого профиля» просит минимум седьмой урок.
  const base = JSON.parse(JSON.stringify(plan));
  base.picks = [
    { key: 'lesson:poceluy_melisandry' },
    { key: 'lesson:diletantka_shirokogo_profilya' }, // второй — рано
    { key: 'lesson:uchene_svet' },
    { key: 'lesson:prirozhdennyy_alhimik' },
    { key: 'lesson:da_sdohni_zhe' },
    { key: 'lesson:posledniy_boy' },
    { key: 'lesson:ping_sistem' }
  ];
  base.deferred = [];
  base.not_taken = [];

  const early = rowByTitleIn(bootPage(base).document, 'дилетантка широкого профиля')
    .querySelector('[data-min-tag]');
  assert.ok(early, 'тег «минимум n-ным» на месте');
  assert.ok(early.classList.contains('warn') && !early.classList.contains('ok'), 'нарушенное требование красное');

  // Тот же урок седьмым: требование выполнено — тег зеленеет.
  const late = JSON.parse(JSON.stringify(base));
  late.picks.splice(1, 1);
  late.picks.push({ key: 'lesson:diletantka_shirokogo_profilya' });
  const good = rowByTitleIn(bootPage(late).document, 'дилетантка широкого профиля')
    .querySelector('[data-min-tag]');
  assert.ok(good.classList.contains('ok') && !good.classList.contains('warn'), 'выполненное требование зелёное');
  assert.ok(/условие выполнено/.test(good.title), 'в подсказке видно, что условие выполнено');

  const bad = rowByTitleIn(bootPage(base).document, 'пинг систем').querySelector('[data-min-tag]');
  assert.ok(bad.classList.contains('warn') && !bad.classList.contains('ok'), 'нарушенное требование красное');
});

test('отметить можно только заряженный и купленный накопитель', () => {
  const d = bootPage(JSON.parse(JSON.stringify(plan))).document;
  // «хардкор» куплен в плане — покупка его и заряжает.
  const hardkor = attachByRe(d, /хардкор/);
  assert.strictEqual(hardkor.querySelector('input').disabled, false, 'заряженный накопитель доступен');
  assert.ok(/\(1\/1\)/.test(hardkor.textContent), 'видны заряды: ' + hardkor.textContent);
  // «великий успех» набрал половину заряда, а поднять его в этом посмертии
  // нечем: уроки на заряд остались в прошлых посмертиях.
  const uspeh = attachByRe(d, /великий успех/);
  assert.strictEqual(uspeh.querySelector('input').disabled, true, 'незаряженный накопитель не отметить');
  assert.ok(/нет целого заряда: 0,5/.test(uspeh.textContent), 'сказано, сколько заряда не хватает');
  // «горожане»: 0,8 заряда, зато уроки подняли максимум и скорость зарядки.
  const gorozhane = attachByRe(d, /горожане/);
  assert.strictEqual(gorozhane.querySelector('input').disabled, true, 'незаряженный накопитель не отметить');
  assert.ok(/\(0,8\/2, \+25% за жизнь\)/.test(gorozhane.textContent), 'видны заряды и скорость: ' + gorozhane.textContent);
  // «равновесие зла и добра» в план не куплено — нельзя.
  const ravno = attachByRe(d, /равновесие/);
  assert.strictEqual(ravno.querySelector('input').disabled, true, 'некупленный накопитель не отметить');
  assert.ok(/сначала купи/.test(ravno.textContent), 'сказано, что сначала купить');
});

test('взять фолиант можно после урока и выкупа, и это бесплатно', () => {
  const p = JSON.parse(JSON.stringify(plan));
  // Снимаем урок метода и выкуп: брать фолиант нечем.
  p.picks = p.picks.filter((x) => x.key !== 'folio:vykupit_tuman' && x.key !== 'lesson:metod_neproglyadnogo_tumana');
  p.folios[0].take = true; // заодно проверяем, что незаслуженная отметка снимается
  const d = bootPage(p).document;

  const tuman = () => attachByRe(d, /Непроглядного Тумана/);
  assert.strictEqual(tuman().querySelector('input').disabled, true, 'без урока и выкупа фолиант не взять');
  assert.strictEqual(tuman().querySelector('input').checked, false, 'незаслуженная отметка снята');
  assert.ok(/сначала урок/.test(tuman().textContent) && /нужен выкуп/.test(tuman().textContent),
    'сказано и про урок, и про выкуп');

  // Покупаем урок метода и выкуп — галочка открывается.
  pickIn(d, 'метод Непроглядного Тумана');
  pickIn(d, 'выкупить метод Непроглядного Тумана');
  assert.strictEqual(tuman().querySelector('input').disabled, false, 'после урока и выкупа фолиант доступен');

  const spent = d.getElementById('sum-spent').textContent;
  tuman().querySelector('input').click();
  assert.strictEqual(d.getElementById('sum-spent').textContent, spent, 'сам выбор фолианта бесплатный');
  assert.ok(/беру с собой:[^\n]*Непроглядного/.test(d.getElementById('kt-export-text').value),
    'фолиант попал в «беру с собой»');
});

test('перезагрузка каркаса не стирает отметки накопителей и фолиантов', () => {
  const p = JSON.parse(JSON.stringify(plan));
  const w = bootPage(p);
  const d = w.document;
  assert.strictEqual(attachByRe(d, /хардкор/).querySelector('input').checked, true, 'отметка до загрузки');
  assert.strictEqual(attachByRe(d, /Непроглядного Тумана/).querySelector('input').checked, true, 'фолиант до загрузки');

  // Открываем панель каркаса и жмём «Загрузить в план».
  d.querySelector('.toolbar-actions .btn.ghost').click();
  const buttons = d.querySelectorAll('#kt-import .row.gap.wrap .btn');
  buttons[0].click();

  assert.strictEqual(attachByRe(d, /хардкор/).querySelector('input').checked, true, 'отметка накопителя пережила загрузку');
  assert.strictEqual(attachByRe(d, /Непроглядного Тумана/).querySelector('input').checked, true, 'отметка фолианта пережила загрузку');
});

test('в панели каркаса видно, что каркас в плане можно обновить', () => {
  const p = JSON.parse(JSON.stringify(plan));
  p.skeleton = 'Жизнь 8 · Заскорузлые пирамиды'; // так называется готовый каркас в списке
  const w = bootPage(p);
  const d = w.document;
  d.querySelector('.toolbar-actions .btn.ghost').click();
  const panel = d.getElementById('kt-import');
  assert.ok(/Каркас в плане/.test(panel.textContent), 'сказано, какой каркас внутри плана');
  const btn = Array.prototype.find.call(panel.querySelectorAll('button'), (b) => /Обновить из готового/.test(b.textContent));
  assert.ok(btn, 'есть кнопка обновления каркаса');
  // План с другим каркасом: подсказки нет, обновлять нечего.
  const other = JSON.parse(JSON.stringify(plan));
  other.skeleton = 'Жизнь 9 · что-то другое';
  const d2 = bootPage(other).document;
  d2.querySelector('.toolbar-actions .btn.ghost').click();
  assert.ok(!/Каркас в плане/.test(d2.getElementById('kt-import').textContent), 'чужой каркас не предлагаем');
});

test('обновление каркаса говорит, если покупка пропала из посмертия', () => {
  const p = JSON.parse(JSON.stringify(plan));
  const w = bootPage(p);
  const d = w.document;
  const seen = [];
  w.alert = (msg) => { seen.push(msg); };

  d.querySelector('.toolbar-actions .btn.ghost').click(); // «Каркас»
  const area = d.getElementById('kt-skeleton-text');
  const sk = JSON.parse(area.value);
  assert.ok(sk.shop.some((it) => it.key === 'lesson:uchene_svet'), 'урок «ученье — свет!» в каркасе');
  // Так выглядит каркас после правки: урока в этом посмертии больше нет.
  sk.shop = sk.shop.filter((it) => it.key !== 'lesson:uchene_svet');
  area.value = JSON.stringify(sk);
  d.querySelectorAll('#kt-import .row.gap.wrap .btn')[0].click(); // «Загрузить в план»

  assert.ok(/нет покупок: ученье — свет!/.test(seen.join('\n')), 'про пропажу сказано: ' + seen.join(' / '));
  d.querySelector('.toolbar .btn:not(.ghost)').click(); // «Сохранить»
  const st = JSON.parse(d.getElementById('kt-state').value);
  assert.ok(!st.picks.some((x) => x.key === 'lesson:uchene_svet'), 'исчезнувшая покупка убрана из плана');
  assert.ok(st.picks.some((x) => x.key === 'lesson:ping_sistem'), 'остальные покупки на месте');
});

if (failures) {
  console.error('\nПровалено проверок: ' + failures);
  process.exit(1);
}
console.log('\nВсе проверки конструктора пройдены.');

test('«Разложить уроки по требованиям» раскладывает уроки и просит сохранить', () => {
  // Уроки стоят как попало: «пинг систем» (минимум 10-м) раньше «ученья — свет!»
  // (минимум 4-м), «прирождённого алхимика» (6-м) и «дилетантки» (7-м).
  const p = JSON.parse(JSON.stringify(plan));
  p.picks = [
    { key: 'lesson:poceluy_melisandry' },
    { key: 'lesson:ping_sistem' },
    { key: 'lesson:uchene_svet' },
    { key: 'lesson:prirozhdennyy_alhimik' },
    { key: 'lesson:da_sdohni_zhe' },
    { key: 'lesson:metod_neproglyadnogo_tumana' },
    { key: 'lesson:posledniy_boy' },
    { key: 'lesson:sama_sebe_bezoar' },
    { key: 'lesson:diletantka_shirokogo_profilya' },
    { key: 'lesson:maksimum_zaradov_gorozhan' },
    { key: 'lesson:skorost_zaradki_gorozhan' }
  ];
  p.deferred = [];
  p.not_taken = p.not_taken.filter((k) => k !== 'lesson:diletantka_shirokogo_profilya');
  // Отметки накопителей и фолиантов снимаем: в этом плане их нечем заслужить,
  // а снятая отметка сама по себе пометила бы план несохранённым.
  p.accumulators.forEach((a) => { a.activate = false; });
  p.folios.forEach((f) => { f.take = false; });
  const w = bootPage(p);
  const d = w.document;
  assert.strictEqual(d.getElementById('kt-dirty').hidden, true, 'до сортировки сохранять нечего');

  const btn = Array.prototype.find.call(d.querySelectorAll('#sec-lessons .sec-tools button'),
    (b) => /Разложить/.test(b.textContent));
  assert.ok(btn, 'кнопка «Разложить уроки по требованиям» на месте');
  btn.click();

  assert.strictEqual(d.getElementById('kt-dirty').hidden, false, 'план помечен несохранённым — иначе порядок потеряется');

  w.HTMLFormElement.prototype.submit = function () { w.__state = d.getElementById('kt-state').value; };
  d.querySelector('.toolbar .btn:not(.ghost)').click(); // «Сохранить»
  const st = JSON.parse(w.__state);
  assert.deepStrictEqual(KT.computeSummary(st, sections).warnings.filter((x) => /минимум/.test(x)), [],
    'после раскладки нарушений «минимум n-ным» нет');
  const at = (key) => st.picks.findIndex((x) => x.key === key);
  assert.ok(at('lesson:uchene_svet') < at('lesson:diletantka_shirokogo_profilya'), '«ученье» (4-м) раньше «дилетантки» (7-м)');
  assert.ok(at('lesson:diletantka_shirokogo_profilya') < at('lesson:ping_sistem'), '«дилетантка» (7-м) раньше «пинг систем» (10-м)');
});

test('слотов невыученных в каркасе два', () => {
  const d = bootPage(JSON.parse(JSON.stringify(plan))).document;
  const slots = d.getElementById('sum-slots');
  assert.ok(slots, 'счётчик слотов на месте');
  assert.strictEqual(slots.textContent.trim(), 'слоты невыученных: 1 / 2',
    'в каркасе открыто два слота, один занят');
  assert.strictEqual(JSON.parse(d.getElementById('kt-plan').textContent).deferred_slots, 2,
    'план помнит, сколько слотов открыто');
});

test('«Разложить уроки» не спотыкается о фиксированные уроки и отложенные', () => {
  // Так выглядит живой план: между нумеруемыми уроками стоят фиксированные
  // («фиксированная цена», «трофей», «освоено»), а два урока отложены
  // невыученными. Раскладывать надо только нумеруемые и не трогать остальные.
  const p = JSON.parse(JSON.stringify(plan));
  p.picks = [
    { key: 'lesson:poceluy_melisandry' },
    { key: 'patch:otkluchenie_trenirovok' },
    { key: 'lesson:da_sdohni_zhe' },
    { key: 'lesson:prirozhdennyy_alhimik' },
    { key: 'lesson:diletantka_shirokogo_profilya' },
    { key: 'lesson:ping_sistem' },
    { key: 'lesson:professionalnaya_deva' },
    { key: 'lesson:nenavizhu_yady' },
    { key: 'lesson:prodlenie_bitvy' },
    { key: 'lesson:posledniy_boy' },
    { key: 'lesson:povyshenie_chistoty_korney' },
    { key: 'lesson:metod_ognennogo_vodopada' },
    { key: 'lesson:metod_neproglyadnogo_tumana' },
    { key: 'lesson:metod_tonkogo_pogloscheniya' },
    { key: 'lesson:maksimum_zaradov_gorozhan' },
    { key: 'lesson:skorost_zaradki_gorozhan' }
  ];
  p.deferred = [{ key: 'lesson:sama_sebe_bezoar', slot: 1 }, { key: 'lesson:uchene_svet', slot: 2 }];
  p.not_taken = [];
  p.accumulators.forEach((a) => { a.activate = false; });
  p.folios.forEach((f) => { f.take = false; });
  const w = bootPage(p);
  const d = w.document;

  const before = KT.computeSummary(JSON.parse(JSON.stringify(p)), sections)
    .warnings.filter((x) => /минимум/.test(x));
  assert.strictEqual(before.length, 3, 'до раскладки три нарушения: ' + before.join(' / '));

  const btn = Array.prototype.find.call(d.querySelectorAll('#sec-lessons .sec-tools button'), (b) => /Разложить/.test(b.textContent));
  btn.click();

  const hint = d.querySelector('#sec-lessons .sec-tools .sort-hint');
  assert.ok(/порядок разложен/.test(hint.textContent), 'кнопка говорит, что порядок разложен: ' + hint.textContent);

  w.HTMLFormElement.prototype.submit = function () { w.__state = d.getElementById('kt-state').value; };
  d.querySelector('.toolbar .btn:not(.ghost)').click(); // «Сохранить»
  const st = JSON.parse(w.__state);
  const sum = KT.computeSummary(st, sections);
  assert.deepStrictEqual(sum.warnings.filter((x) => /минимум/.test(x)), [],
    'после раскладки нарушений «минимум n-ным» нет');
  assert.deepStrictEqual(sum.warnings.filter((x) => /не получена/.test(x)), [],
    'награда «последний бой» не потерялась');

  const at = (key) => st.picks.findIndex((x) => x.key === key);
  assert.ok(at('lesson:prirozhdennyy_alhimik') < at('lesson:diletantka_shirokogo_profilya'),
    '«прирождённый алхимик» (6-м) раньше «дилетантки» (7-м)');
  assert.ok(at('lesson:diletantka_shirokogo_profilya') < at('lesson:ping_sistem'),
    '«дилетантка» (7-м) раньше «пинг систем» (10-м)');
  assert.ok(at('lesson:ping_sistem') < at('lesson:posledniy_boy'),
    '«последний бой» идёт после «пинг систем» — награда за порядок');
  // Обязательная заплатка осталась на своём месте, а урок с фиксированной
  // ценой так и не получил номера — его не раскладывали.
  assert.strictEqual(at('patch:otkluchenie_trenirovok'), 1, 'обязательная заплатка не сдвинулась');
  const deva = sum.sections.filter((s) => s.key === 'lessons')[0].lines
    .filter((l) => l.key === 'lesson:professionalnaya_deva')[0];
  assert.ok(deva && deva.fixed && !deva.number,
    'урок с фиксированной ценой номера не получил: ' + JSON.stringify(deva));

  // Повторный клик: менять нечего, и это видно.
  btn.click();
  const again = d.querySelector('#sec-lessons .sec-tools .sort-hint');
  assert.ok(/менять нечего/.test(again.textContent), 'повторная раскладка говорит, что менять нечего: ' + again.textContent);
});
