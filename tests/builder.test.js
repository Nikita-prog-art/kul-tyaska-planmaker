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
  const rows = document.querySelectorAll('.row.item');
  for (const row of rows) {
    const t = row.querySelector('.item-title');
    if (t && t.textContent.indexOf(title) >= 0) return row;
  }
  throw new Error('не нашёл строку «' + title + '»');
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
  assert.ok(document.querySelectorAll('.ladder').length >= 8, 'все шкалы на месте');
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
  assert.strictEqual(first.cost, 0, 'уже купленный урок не списывает опыт');
  assert.strictEqual(first.note, 'уже куплено');
  assert.strictEqual(lessons.lines.filter((l) => l.number === 2)[0].cost, 10, 'следующий урок стоит 10 XP');
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
    r.querySelector('.item-title').textContent.indexOf('великий успех') >= 0);
  assert.ok(row, 'нашёл урок «великий успех»');

  row.querySelector('.state.pick').click(); // до починки здесь падало на plan.picks.some
  d.querySelector('.toolbar .btn:not(.ghost)').click(); // «Сохранить» — состояние уходит в форму
  const st = JSON.parse(d.getElementById('kt-state').value || 'null');
  assert.ok(st && Array.isArray(st.picks), 'состояние сохраняется массивом');
  assert.ok(st.picks.some((p) => /velikiy_uspeh/.test(p.key)), 'выбранный урок записался');
  assert.ok(st.picks.some((p) => /poceluy/.test(p.key)), 'обязательная позиция каркаса тоже на месте');
  assert.ok(d.getElementById('sum-spent').textContent.length > 0, 'суммы пересчитались');
});

if (failures) {
  console.error('\nПровалено проверок: ' + failures);
  process.exit(1);
}
console.log('\nВсе проверки конструктора пройдены.');
