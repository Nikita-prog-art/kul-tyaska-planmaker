/* Переключатель темы: тёмная «ночная тушь» ↔ светлая «пергамент».
   Выбор живёт в localStorage; без сохранённого выбора тему задаёт система
   (ранний скрипт в base.jinja). */
(function () {
  'use strict';

  var root = document.documentElement;
  var btn = document.getElementById('kt-theme');

  function current() {
    return root.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
  }

  function label(theme) {
    if (!btn) return;
    var next = theme === 'light' ? 'Тёмная тема' : 'Светлая тема';
    btn.textContent = theme === 'light' ? '🌙' : '☀️';
    btn.title = next;
    btn.setAttribute('aria-label', next);
  }

  function apply(theme) {
    root.setAttribute('data-theme', theme);
    try { localStorage.setItem('kt-theme', theme); } catch (e) { /* приватный режим */ }
    label(theme);
  }

  label(current());

  if (btn) {
    btn.addEventListener('click', function () {
      apply(current() === 'light' ? 'dark' : 'light');
    });
  }
})();
