#!/usr/bin/env node
/* Кросс-тест: JS-движок конструктора должен считать ровно так же,
   как серверный Go-движок. Эталон делает Go (go test ./internal/plan -update),
   здесь мы прогоняем тот же план через static/engine.js и сверяем текст. */
'use strict';

const fs = require('fs');
const path = require('path');
const assert = require('assert');

const root = path.join(__dirname, '..');
const KT = require(path.join(root, 'internal/web/static/engine.js'));

const plan = JSON.parse(fs.readFileSync(path.join(root, 'internal/plan/testdata/plan.json'), 'utf8'));
const sections = JSON.parse(fs.readFileSync(path.join(root, 'internal/plan/testdata/sections.json'), 'utf8'));
const golden = fs.readFileSync(path.join(root, 'internal/plan/testdata/export.golden'), 'utf8');

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

console.log('JS-движок против эталона Go:');

test('стоимость урока растёт на 5 XP', () => {
  assert.strictEqual(KT.lessonCost(1), 5);
  assert.strictEqual(KT.lessonCost(4), 20);
  assert.strictEqual(KT.lessonCost(10), 50);
});

test('шкалы считаются по шагам', () => {
  assert.strictEqual(KT.stepCost({ type: 'to_value', rate: 1 }, 3), 3);
  assert.strictEqual(KT.stepCost({ type: 'linear', rate: 5 }, 10), 5);
  assert.strictEqual(KT.stepCost({ type: 'table', table: [0, 0, 10, 20] }, 3), 20);
  assert.strictEqual(KT.stepCost({ type: 'per_level', per_level: 50 }, 2), 100);
  assert.strictEqual(KT.stepCost({ type: 'to_value', rate: 2.5, ceil: true }, 5), 13);
});

test('текст плана совпадает с Go-эталоном', () => {
  const summary = KT.computeSummary(plan, sections);
  const got = KT.renderExport(plan, summary, sections);
  assert.strictEqual(got, golden);
});

test('суммы по секциям и остаток считаются последовательно', () => {
  const summary = KT.computeSummary(plan, sections);
  let run = plan.balance;
  let total = 0;
  summary.sections.forEach((s) => {
    run -= s.sum;
    total += s.sum;
    assert.ok(Math.abs(run - s.remainder) < 1e-9, 'остаток секции ' + s.key);
  });
  assert.ok(Math.abs(total - summary.total) < 1e-9, 'итог сходится с суммой секций');
});

test('предупреждения про «минимум n-ным уроком» на месте', () => {
  const summary = KT.computeSummary(plan, sections);
  const text = summary.warnings.join('\n');
  assert.ok(text.includes('пинг систем'), 'ждём предупреждение про «пинг систем»');
  assert.ok(summary.notTaken.length > 0, 'не взятые уроки попадают в «Не взято»');
});

test('слотов невыученных два: второй открыт в каркасе', () => {
  assert.strictEqual(KT.computeSummary(plan, sections).deferredLimit, 2,
    'урок «невыученные уроки всё ещё не выучены» она взяла раньше — слотов два');

  const p = JSON.parse(JSON.stringify(plan));
  p.deferred = [
    { key: 'lesson:diletantka_shirokogo_profilya', slot: 1 },
    { key: 'lesson:vodnoe_teloslozhenie', slot: 2 }
  ];
  let sum = KT.computeSummary(p, sections);
  assert.strictEqual(sum.deferredUsed, 2);
  assert.ok(!sum.warnings.some((w) => w.indexOf('слотов') >= 0),
    'два отложенных урока влезают: ' + sum.warnings.join('; '));

  p.deferred.push({ key: 'lesson:prodlenie_bitvy', slot: 3 });
  sum = KT.computeSummary(p, sections);
  assert.ok(sum.warnings.some((w) => w.indexOf('отложено уроков больше') >= 0),
    'третий уже не влезает: ' + sum.warnings.join('; '));
});

test('отложенный урок бесплатный и без номера', () => {
  const summary = KT.computeSummary(plan, sections);
  const lessons = summary.sections.filter((s) => s.key === 'lessons')[0];
  const deferred = lessons.lines.filter((l) => l.deferred);
  assert.strictEqual(deferred.length, 1);
  assert.strictEqual(deferred[0].cost, 0);
  assert.strictEqual(deferred[0].number, 0);
});

test('фиксированная цена не съедает номер урока', () => {
  const summary = KT.computeSummary(plan, sections);
  const lessons = summary.sections.filter((s) => s.key === 'lessons')[0];
  const numbered = lessons.lines.filter((l) => l.number > 0).map((l) => l.number);
  // Номера идут подряд: урок-трофей за 0 XP («метод Ядовитых Миазмов»)
  // и фиксированная «Дева-в-Беде» номер не занимают.
  assert.deepStrictEqual(numbered, [1, 2, 3, 4, 5, 6, 7, 8, 9]);
});

test('фиксированные уроки живут в своём разделе', () => {
  const summary = KT.computeSummary(plan, sections);
  const lessons = summary.sections.filter((s) => s.key === 'lessons')[0];
  const fixed = summary.sections.filter((s) => s.key === 'fixed')[0];
  assert.ok(fixed, 'раздел «Уроки с фиксированной ценой» есть');
  assert.ok(lessons.lines.every((l) => !l.fixed), 'в общем разделе фиксированных нет');
  assert.ok(fixed.lines.length > 0 && fixed.lines.every((l) => l.fixed && !l.number), 'у фиксированных нет номеров');
});

test('уроки печатаются одной строкой', () => {
  const lines = golden.split('\n').filter((l) => l.indexOf('Уроки: ') === 0);
  assert.strictEqual(lines.length, 1, 'строка уроков одна: ' + lines.length);
  assert.ok(/^Уроки: 1\. /.test(lines[0]), 'нумерация начинается с первого урока');
  assert.ok(golden.indexOf('Уроки с фиксированной ценой:') >= 0, 'фиксированные — отдельным разделом');
});

test('в плане только то, что берём', () => {
  assert.ok(golden.indexOf('не активирую') < 0, 'списка «не активирую» нет');
  assert.ok(golden.indexOf('оставляю в библиотеке') < 0, 'списка «оставляю в библиотеке» нет');
  assert.ok(/активирую: хардкор/.test(golden), 'активируемый накопитель назван');
  assert.ok(/беру с собой: метод Непроглядного Тумана/.test(golden), 'взятый фолиант назван');
});

test('накопитель активируется, только если заряжен или куплен', () => {
  const byName = (n) => plan.accumulators.filter((a) => a.name.indexOf(n) >= 0)[0];
  // «горожане»: 0,8 заряда — целого заряда ещё нет (прибавка за жизнь
  // достанется в следующей жизни), тратить нечего. Уроки этого посмертия
  // подняли максимум до 2 и скорость зарядки до 25%.
  const gorozhane = KT.accumulatorState(plan, byName('горожане'));
  assert.strictEqual(gorozhane.ready, false);
  assert.ok(gorozhane.reason.indexOf('нет целого заряда: 0,8') >= 0, gorozhane.reason);
  assert.deepStrictEqual(KT.accumulatorValues(plan, byName('горожане')), { charges: 0.8, max: 2, rate: 25 });
  // «окрестности Му» — вообще 0,03.
  assert.strictEqual(KT.accumulatorState(plan, byName('окрестности')).ready, false);
  // «хардкор» куплен в плане — покупка его и заряжает.
  assert.strictEqual(KT.accumulatorState(plan, byName('хардкор')).ready, true);
  // «великий успех» набрал половину заряда, а поднять его в этом посмертии
  // нечем: уроки «великий успех» и «заряд великого успеха» остались в прошлых.
  const uspeh = byName('великий успех');
  assert.strictEqual(KT.accumulatorState(plan, uspeh).ready, false);
  assert.strictEqual(KT.accumulatorValues(plan, uspeh).charges, 0.5);
  assert.ok(KT.accumulatorState(plan, uspeh).reason.indexOf('нет целого заряда: 0,5') >= 0);
  // «равновесие» не куплено — нельзя.
  const ravno = KT.accumulatorState(plan, byName('равновесие'));
  assert.strictEqual(ravno.ready, false);
  assert.ok(ravno.reason.indexOf('сначала купи') >= 0, 'сказано, что сначала купить');
});

test('заряд тратится по одному: 1 из 2 — уже можно', () => {
  const p = JSON.parse(JSON.stringify(plan));
  const gor = p.accumulators.filter((a) => a.name.indexOf('горожане') >= 0)[0];
  // В плане есть «максимум зарядов накопителя «горожане»»: максимум 2, заряд 0,8.
  assert.deepStrictEqual(KT.accumulatorValues(p, gor), { charges: 0.8, max: 2, rate: 25 });
  assert.strictEqual(KT.accumulatorState(p, gor).ready, false, '0,8 из 2 — не заряд');
  gor.charges = 1;
  assert.strictEqual(KT.accumulatorState(p, gor).ready, true, '1 из 2 — уже целый заряд');
});

test('фолиант берут после урока метода и выкупа, и это бесплатно', () => {
  const folio = plan.folios[0];
  assert.strictEqual(KT.folioState(plan, folio).ready, true, 'в эталонном плане фолиант выкуплен');

  // Убираем урок метода и выкуп — фолиант запирается.
  const bare = JSON.parse(JSON.stringify(plan));
  bare.picks = bare.picks.filter((p) => p.key !== 'lesson:metod_neproglyadnogo_tumana' && p.key !== 'folio:vykupit_tuman');
  const st = KT.folioState(bare, bare.folios[0]);
  assert.strictEqual(st.ready, false);
  assert.ok(st.reason.indexOf('сначала урок') >= 0 && st.reason.indexOf('нужен выкуп') >= 0, st.reason);

  // Незаслуженные отметки движок снимает, а план от выбора фолианта не дорожает.
  const before = KT.computeSummary(bare, sections).total;
  bare.folios[0].take = true;
  bare.accumulators.forEach((a) => { a.activate = true; });
  // «горожане» доводим до целого заряда: тогда отметка заслужена.
  bare.accumulators.filter((a) => a.name.indexOf('горожане') >= 0)[0].charges = 1;
  assert.ok(KT.pruneAttachments(bare) > 0, 'сняты незаслуженные отметки');
  assert.strictEqual(bare.folios[0].take, false);
  const left = bare.accumulators.filter((a) => a.activate).map((a) => a.name).sort();
  assert.deepStrictEqual(left, ['горожане', 'хардкор'],
    'остались только заряженный «горожане» и купленный «хардкор»; «великий успех» с 0,5 и «окрестности Му» с 0,03 сняты');
  assert.strictEqual(KT.computeSummary(bare, sections).total, before, 'отметки фолиантов денег не стоят');
});

test('выкуп без урока метода — это ошибка в плане', () => {
  const p = JSON.parse(JSON.stringify(plan));
  p.picks = p.picks.filter((x) => x.key !== 'lesson:metod_neproglyadnogo_tumana');
  const warnings = KT.computeSummary(p, sections).warnings.join('\n');
  assert.ok(warnings.indexOf('выкупить метод Непроглядного Тумана') >= 0, 'назван сам выкуп: ' + warnings);
  assert.ok(warnings.indexOf('взят без') >= 0, 'сказано, что он взят без урока: ' + warnings);
});

if (failures) {
  console.error('\nПровалено проверок: ' + failures);
  process.exit(1);
}
console.log('\nВсе проверки JS-движка пройдены.');
