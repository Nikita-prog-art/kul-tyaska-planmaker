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
  assert.deepStrictEqual(numbered, [1, 2, 3, 4, 5, 6, 7]);
});

if (failures) {
  console.error('\nПровалено проверок: ' + failures);
  process.exit(1);
}
console.log('\nВсе проверки JS-движка пройдены.');
