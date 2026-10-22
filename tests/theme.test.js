#!/usr/bin/env node
/* Переключатель темы: тёмная ↔ светлая, выбор запоминается в localStorage. */
'use strict';

const fs = require('fs');
const path = require('path');
const assert = require('assert');

const root = path.join(__dirname, '..');
const jsdomPath = path.join(root, '.cache/jsdtest/node_modules/jsdom');
if (!fs.existsSync(jsdomPath)) {
  console.error('jsdom не установлен — пропускаю тест темы.');
  console.error('  npm install --prefix .cache/jsdtest --cache .cache/npm-cache jsdom');
  process.exit(0);
}

const { JSDOM } = require(jsdomPath);
const theme = fs.readFileSync(path.join(root, 'internal/web/static/theme.js'), 'utf8');

let failures = 0;
function test(name, fn) {
  try {
    fn();
    console.log('  ✓ ' + name);
  } catch (e) {
    failures++;
    console.error('  ✗ ' + name);
    console.error('    ' + e.message);
  }
}

function boot(attr) {
  const dom = new JSDOM(
    `<!doctype html><html${attr ? ' data-theme="' + attr + '"' : ''}><body>` +
      '<button id="kt-theme" class="theme-toggle" type="button"></button></body></html>',
    { runScripts: 'outside-only', url: 'https://kultyaska.test/plans/x' });
  dom.window.eval(theme);
  return dom.window;
}

test('на тёмной теме кнопка предлагает светлую', () => {
  const w = boot('dark');
  const btn = w.document.getElementById('kt-theme');
  assert.strictEqual(btn.textContent, '☀️');
  assert.strictEqual(btn.title, 'Светлая тема');
  assert.strictEqual(w.document.documentElement.getAttribute('data-theme'), 'dark');
});

test('клик переключает тему и запоминает выбор', () => {
  const w = boot('dark');
  w.document.getElementById('kt-theme').click();
  assert.strictEqual(w.document.documentElement.getAttribute('data-theme'), 'light');
  assert.strictEqual(w.localStorage.getItem('kt-theme'), 'light');
  assert.strictEqual(w.document.getElementById('kt-theme').textContent, '🌙');

  w.document.getElementById('kt-theme').click();
  assert.strictEqual(w.document.documentElement.getAttribute('data-theme'), 'dark');
  assert.strictEqual(w.localStorage.getItem('kt-theme'), 'dark');
  assert.strictEqual(w.document.getElementById('kt-theme').title, 'Светлая тема');
});

test('без сохранённого выбора тема считается тёмной', () => {
  const w = boot('');
  assert.strictEqual(w.document.getElementById('kt-theme').textContent, '☀️');
  w.document.getElementById('kt-theme').click();
  assert.strictEqual(w.localStorage.getItem('kt-theme'), 'light');
});

test('кнопки может не быть — скрипт не падает', () => {
  const dom = new JSDOM('<!doctype html><html data-theme="light"><body></body></html>',
    { runScripts: 'outside-only', url: 'https://kultyaska.test/' });
  dom.window.eval(theme);
  assert.strictEqual(dom.window.document.documentElement.getAttribute('data-theme'), 'light');
});

if (failures) {
  console.error('\nПровалено проверок: ' + failures);
  process.exit(1);
}
console.log('\nВсе проверки темы пройдены.');
