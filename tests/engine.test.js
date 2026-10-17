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
  assert.ok(summary.deferredLimit >= 1);
  assert.ok(summary.notTaken.length > 0, 'не взятые уроки попадают в «Не взято»');
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
  assert.deepStrictEqual(numbered, [1, 2, 3, 4, 5, 6, 7, 8]);
});

test('накопитель активируется, только если заряжен или куплен', () => {
  const byName = (n) => plan.accumulators.filter((a) => a.name.indexOf(n) >= 0)[0];
  // «горожане»: 0,8 заряда + 20% за прожитую жизнь — полный.
  assert.strictEqual(KT.accumulatorState(plan, byName('горожане')).ready, true);
  const mu = KT.accumulatorState(plan, byName('окрестности'));
  assert.strictEqual(mu.ready, false);
  assert.ok(mu.reason.indexOf('0,04 из 1') >= 0, 'видно, сколько заряда не хватает: ' + mu.reason);
  // «хардкор» куплен в плане — можно; «равновесие» не куплено — нельзя.
  assert.strictEqual(KT.accumulatorState(plan, byName('хардкор')).ready, true);
  const ravno = KT.accumulatorState(plan, byName('равновесие'));
  assert.strictEqual(ravno.ready, false);
  assert.ok(ravno.reason.indexOf('сначала купи') >= 0, 'сказано, что сначала купить');
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
  assert.ok(KT.pruneAttachments(bare) > 0, 'сняты незаслуженные отметки');
  assert.strictEqual(bare.folios[0].take, false);
  const left = bare.accumulators.filter((a) => a.activate).map((a) => a.name).sort();
  assert.deepStrictEqual(left, ['горожане', 'хардкор'], 'остались только заряженный и купленный');
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
