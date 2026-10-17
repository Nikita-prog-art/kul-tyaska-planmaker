/* Движок плана: стоимость, суммы по секциям, текст экспорта.
   Чистые функции без DOM — их же гоняют тесты в node (см. tests/engine.test.js).
   Логика намеренно повторяет серверную (internal/plan), чтобы предпросмотр
   в браузере и страница экспорта совпадали до символа. */
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.KT = api;
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var LESSON_BASE = 5;
  var LESSON_STEP = 5;

  function lessonCost(n) {
    if (!n || n <= 0) return 0;
    return LESSON_BASE + LESSON_STEP * (n - 1);
  }

  function stepCost(cost, level) {
    if (!cost) return 0;
    var v = 0;
    switch (cost.type) {
      case 'to_value': v = level * (cost.rate || 1); break;
      case 'linear': v = cost.rate || 0; break;
      case 'per_level': v = level * (cost.per_level || 0); break;
      case 'table': v = (cost.table && cost.table[Math.round(level)]) || 0; break;
      default: v = 0;
    }
    if (cost.ceil && v) v = Math.ceil(v);
    return v;
  }

  function ladderCost(ladder) {
    var total = 0;
    var step = ladder.step > 0 ? ladder.step : 1;
    var value = Number(ladder.value) || 0;
    var from = Number(ladder.from) || 0;
    for (var v = from + step; v <= value + 1e-9; v += step) total += stepCost(ladder.cost, v);
    return total;
  }

  function itemMap(plan) {
    var m = {};
    (plan.shop || []).forEach(function (it) { m[it.key] = it; });
    return m;
  }

  function hasTag(item, tag) {
    return !!(item && item.tags && item.tags.indexOf(tag) >= 0);
  }

  function deferredLimit(plan, items) {
    var limit = 1;
    (plan.picks || []).forEach(function (pk) {
      if (hasTag(items[pk.key], 'defer_slot')) limit++;
    });
    return limit;
  }

  /* computeSummary(plan, sections) → {sections, total, remainder, balance, warnings, ...} */
  function computeSummary(plan, sections) {
    var items = itemMap(plan);
    var bySection = {};
    var warnings = [];
    var rewards = [];
    var future = [];
    var total = 0;
    var deferredKeys = {};
    (plan.deferred || []).forEach(function (d) { deferredKeys[d.key] = true; });

    // Уроки: порядок покупки задаёт нумерацию и стоимость.
    var paid = 0;
    (plan.picks || []).forEach(function (pk) {
      var it = items[pk.key];
      if (!it || it.kind !== 'lesson' || deferredKeys[pk.key]) return;
      var line = { key: it.key, title: it.title, fixed: !!it.fixed, cost: 0, number: 0 };
      if (it.fixed) {
        line.cost = Number(it.cost) || 0;
      } else {
        paid++;
        line.number = paid;
        line.cost = lessonCost(paid);
        if (it.already_paid) {
          // Урок взят ею самой: номер занят, но опыт за него платим мы.
          line.note = 'уже куплено';
        }
        if ((it.min_lesson || 1) > paid) {
          warnings.push('«' + it.title + '» требует минимум ' + it.min_lesson + '-й урок, а стоит ' + paid + '-м');
        }
      }
      total += line.cost;
      (bySection.lessons = bySection.lessons || []).push(line);
    });

    // Позиции, которые план обязан содержать, и условия каркаса.
    (plan.shop || []).forEach(function (it) {
      if (!it.locked) return;
      var picked = (plan.picks || []).some(function (p) { return p.key === it.key; });
      if (!picked) {
        warnings.push('обязательная позиция «' + it.title + '» не взята' + (it.lock_note ? ': ' + it.lock_note : ''));
      }
    });
    // Покупка с условием: «выкупить метод» без самого урока метода — деньги
    // на ветер, о таком лучше сказать сразу (как и Go-движок).
    (plan.shop || []).forEach(function (it) {
      if (!picked(plan, it.key)) return;
      missingPrereqs(plan, it.requires).forEach(function (k) {
        warnings.push('«' + it.title + '» взят без «' + itemTitle(plan, k) + '»');
      });
    });
    (plan.requirements || []).forEach(function (req) {
      var missed = missingRequirementKeys(plan, req);
      if (req.soft) {
        // Условие «на будущие жизни»: не проверка, а памятка.
        future.push({ text: requirementText(plan, req, ''), met: missed.length === 0 });
        return;
      }
      missed.forEach(function (k) { warnings.push(requirementWarning(plan, req, k)); });
    });

    // Отложенные уроки: бесплатно, без номера.
    var limit = deferredLimit(plan, items);
    (plan.deferred || []).forEach(function (d) {
      var it = items[d.key];
      if (!it) return;
      if (it.no_defer) warnings.push('«' + it.title + '» нельзя отложить невыученными уроками');
      (bySection.lessons = bySection.lessons || []).push({
        key: it.key, title: it.title, deferred: true, cost: 0, number: 0,
        note: 'невыученный, в следующую жизнь'
      });
    });
    var used = (plan.deferred || []).filter(function (d) { return !!items[d.key]; }).length;
    if (used > limit) warnings.push('отложено уроков больше, чем слотов: ' + used + ' из ' + limit);

    // Прочие покупки.
    (plan.picks || []).forEach(function (pk) {
      var it = items[pk.key];
      if (!it || it.kind === 'lesson') return;
      var qty = pk.qty > 0 ? pk.qty : 1;
      var cost = (Number(it.cost) || 0) * qty;
      total += cost;
      var sec = it.section || 'other';
      (bySection[sec] = bySection[sec] || []).push({
        key: it.key, title: it.title, cost: cost, qty: qty,
        note: it.already_paid ? 'уже куплено' : (it.price || '')
      });
    });

    // Шкалы.
    (plan.ladders || []).forEach(function (l) {
      if (l.hidden || !(Number(l.value) > Number(l.from))) return;
      var cost = ladderCost(l);
      if (!cost) return;
      total += cost;
      var sec = l.section || 'other';
      (bySection[sec] = bySection[sec] || []).push({
        key: l.key, title: ladderTitle(l), cost: cost, ladder: true
      });
    });

    // Награды за порядок: «возьми эту позицию после той».
    (plan.shop || []).forEach(function (it) {
      var r = it.reward_after;
      if (!r || !r.key || !picked(plan, it.key) || deferredKeys[it.key]) return;
      var refIdx = pickIndex(plan, r.key);
      if (refIdx < 0) return; // опорной позиции в плане нет — сравнивать не с чем
      var title = r.title || 'квестовая награда';
      var text = title + (r.note ? ': ' + r.note : '');
      if (pickIndex(plan, it.key) > refIdx) {
        rewards.push('«' + it.title + '»: ' + text);
        noteLine(bySection, it.key, text);
        return;
      }
      var lost = r.lose || r.note || '';
      warnings.push('«' + it.title + '» стоит раньше «' + itemTitle(plan, r.key) + '» — ' + title +
        ' не получена' + (lost ? ' (' + lost + ')' : ''));
    });

    var order = (sections || []).slice().sort(function (a, b) { return (a.order || 99) - (b.order || 99); });
    var out = [];
    var run = Number(plan.balance) || 0;
    order.forEach(function (sec) {
      var lines = bySection[sec.key];
      if (!lines || !lines.length) return;
      var sum = lines.reduce(function (a, l) { return a + l.cost; }, 0);
      run -= sum;
      out.push({
        key: sec.key, title: sec.title, hint: sec.hint || '',
        order: sec.order || 99, lines: lines, sum: sum, remainder: run
      });
    });
    if (run < -1e-9) warnings.push('перерасход: ' + fmt(Math.abs(run), true) + ' XP больше, чем есть');

    var taken = {};
    (plan.picks || []).forEach(function (pk) { taken[pk.key] = true; });
    (plan.deferred || []).forEach(function (d) { taken[d.key] = true; });
    var notTaken = (plan.not_taken || []).map(function (k) {
      var it = items[k];
      return it ? { key: it.key, title: it.title } : null;
    }).filter(Boolean);

    return {
      sections: out, total: total, remainder: run, balance: Number(plan.balance) || 0,
      warnings: warnings, rewards: rewards, future: future,
      deferredLimit: limit, deferredUsed: used,
      notTaken: notTaken, paidLessons: paid
    };
  }

  function ladderTitle(l) {
    var val = number(l.value);
    if (l.unit === 'd') return l.title + ': d' + val;
    return l.title + ': ' + val + (l.unit || '');
  }

  function picked(plan, key) {
    return (plan.picks || []).some(function (p) { return p.key === key; });
  }

  /* pickIndex — место позиции в очереди покупок, -1 если её нет. */
  function pickIndex(plan, key) {
    var picks = plan.picks || [];
    for (var i = 0; i < picks.length; i++) {
      if (picks[i].key === key) return i;
    }
    return -1;
  }

  /* noteLine — дописывает пометку к строке плана (как в Go). */
  function noteLine(bySection, key, text) {
    Object.keys(bySection).forEach(function (sec) {
      bySection[sec].forEach(function (l) {
        if (l.key !== key) return;
        l.note = l.note ? l.note + ', ' + text : text;
      });
    });
  }

  /* missingRequirementKeys — чего не хватает для условия каркаса. */
  function missingRequirementKeys(plan, req) {
    var keys = req.keys || [];
    if (req.kind === 'all_of') {
      return keys.filter(function (k) { return !picked(plan, k); });
    }
    return keys.some(function (k) { return picked(plan, k); }) ? [] : [''];
  }

  /* requirementText — условие каркаса без оценки «выполнено/нет». */
  function requirementText(plan, req, key) {
    if (req.message) return req.message;
    var names = key ? [itemTitle(plan, key)] : (req.keys || []).map(function (k) { return itemTitle(plan, k); });
    return (req.kind === 'all_of' ? 'нужно взять: ' : 'нужно взять одно из: ') + names.join(', ');
  }

  function itemTitle(plan, key) {
    var it = itemMap(plan)[key];
    return it ? it.title : key;
  }

  function isDeferredKey(plan, key) {
    return (plan.deferred || []).some(function (d) { return d.key === key; });
  }

  /* missingPrereqs — чего из requires нет в плане. Отложенный урок считается
     взятым: он уезжает в следующую жизнь, а не пропадает. */
  function missingPrereqs(plan, keys) {
    var out = [];
    (keys || []).forEach(function (key) {
      if (!key) return;
      if (picked(plan, key) || isDeferredKey(plan, key)) return;
      out.push(key);
    });
    return out;
  }

  /* prereqText — «название (цена)» для подсказки о недостающей покупке.
     У выкупа фолианта убираем глагол: «нужен выкуп «метод Тумана»». */
  function prereqText(plan, key) {
    var it = itemMap(plan)[key];
    if (!it) return key;
    var title = String(it.title || key).replace(/^выкупить\s*/, '') || String(it.title || key);
    if (!/[«»]/.test(title)) title = '«' + title + '»';
    if (Number(it.cost) > 0) return title + ' (' + number(it.cost) + ' XP)';
    return title;
  }

  /* round6 убирает мусор от сложения дробей (0,8 + 20% = ровно 1). */
  function round6(v) { return Math.round(Number(v) * 1e6) / 1e6; }

  /* accumulatorValues — состояние накопителя на момент перерождения с учётом
     уже сделанных покупок: заряды, максимум и скорость зарядки. Прибавка за
     прожитую жизнь (rate) сюда не входит: эти проценты накопитель получит
     в следующей жизни, а на перерождении тратится то, что накоплено.
     Заряд считается целыми единицами: 0,8 — ещё не заряд, а 1 из 2 — уже заряд. */
  function accumulatorValues(plan, acc) {
    var charges = Number(acc.charges) || 0;
    var max = Number(acc.max) || 0;
    var rate = Number(acc.rate) || 0;
    (plan.picks || []).forEach(function (pk) {
      var it = itemMap(plan)[pk.key];
      if (!it) return;
      var times = pk.qty > 0 ? pk.qty : 1;
      (it.effects || []).forEach(function (e) {
        if (e.accumulator !== acc.key) return;
        charges += (Number(e.charges) || 0) * times;
        max += (Number(e.max) || 0) * times;
        // Скорость зарядки покупка задаёт, а не прибавляет: «до 25% за жизнь».
        if (Number(e.rate) > 0) rate = Number(e.rate);
      });
    });
    return { charges: round6(charges), max: round6(max), rate: round6(rate) };
  }

  /* accumulatorKnown — известно ли про накопитель что-то про заряды: либо
     каркас их указал, либо их меняют покупки. */
  function accumulatorKnown(plan, acc) {
    if (Number(acc.charges) !== 0 || Number(acc.rate) !== 0) return true;
    var touched = false;
    (plan.picks || []).forEach(function (pk) {
      var it = itemMap(plan)[pk.key];
      if (!it) return;
      (it.effects || []).forEach(function (e) {
        if (e.accumulator === acc.key) touched = true;
      });
    });
    return touched;
  }

  /* accumulatorState — можно ли активировать накопитель: он должен быть
     куплен (если каркас этого требует) и иметь хотя бы один целый заряд.
     Накопитель на 1 из 2 уже можно тратить — заряд тратится по одному. */
  function accumulatorState(plan, acc) {
    var missing = missingPrereqs(plan, acc.requires);
    if (missing.length) {
      return {
        ready: false,
        reason: 'сначала купи ' + missing.map(function (k) { return prereqText(plan, k); }).join(', ')
      };
    }
    var v = accumulatorValues(plan, acc);
    if (accumulatorKnown(plan, acc) && v.charges < 1 - 1e-9) {
      return { ready: false, reason: 'нет целого заряда: ' + number(v.charges) };
    }
    return { ready: true, reason: '' };
  }

  /* folioState — можно ли взять фолиант с собой: сначала урок метода, потом
     выкуп. Сам выбор фолианта бесплатный — он оплачен этими покупками. */
  function folioState(plan, folio) {
    var missing = missingPrereqs(plan, folio.requires);
    if (!missing.length) return { ready: true, reason: '' };
    var lessons = [], buys = [];
    missing.forEach(function (key) {
      var it = itemMap(plan)[key];
      if (it && it.kind === 'lesson') lessons.push('«' + (it.title || key) + '»');
      else buys.push(prereqText(plan, key));
    });
    var parts = [];
    if (lessons.length) parts.push('сначала урок ' + lessons.join(', '));
    if (buys.length) parts.push('нужен выкуп ' + buys.join(', '));
    return { ready: false, reason: parts.join(', ') };
  }

  /* pruneAttachments — снимает отметки, которые план позволить не может
     (незаряженный накопитель, невыкупленный фолиант). Возвращает число снятых. */
  function pruneAttachments(plan) {
    var changed = 0;
    (plan.accumulators || []).forEach(function (a) {
      if (a.activate && !accumulatorState(plan, a).ready) { a.activate = false; changed++; }
    });
    (plan.folios || []).forEach(function (f) {
      if (f.take && !folioState(plan, f).ready) { f.take = false; changed++; }
    });
    return changed;
  }

  /* requirementWarning — текст предупреждения по условию каркаса (как в Go). */
  function requirementWarning(plan, req, key) {
    return 'условие каркаса не выполнено — ' + requirementText(plan, req, key);
  }

  function number(v) {
    v = Number(v) || 0;
    if (v === Math.round(v)) return String(Math.round(v));
    return String(v).replace('.', ',');
  }

  /* fmt(v, decimals) — как в Go: 5 знаков, хвостовые нули убираются, запятая. */
  function fmt(v, decimals) {
    v = Number(v) || 0;
    if (!decimals) {
      var r = Math.round(Math.abs(v));
      return (v < 0 ? '-' : '') + r;
    }
    var s = Math.abs(v).toFixed(5);
    if (s.indexOf('.') >= 0) s = s.replace(/0+$/, '').replace(/\.$/, '');
    return (v < 0 ? '-' : '') + s.replace('.', ',');
  }

  /* noteSuffix печатает пометку к строке плана («уже куплено» и т. п.). */
  function noteSuffix(note) {
    return note ? ' (' + note + ')' : '';
  }

  /* renderExport(plan, summary, sections) — текст плана в принятом формате. */
  function renderExport(plan, summary, sections) {
    var dec = !!(plan.options && plan.options.decimals);
    var out = [];
    out.push('[*] ' + String(plan.title || '').trim() + (plan.emoji ? ' ' + String(plan.emoji).trim() : ''));
    out.push('');

    // Проваленные проверки видно сразу: план читает Ли Лун, а не только игрок.
    if ((summary.warnings || []).length) {
      out.push('⚠ Проверки не выполняются: ' + summary.warnings.length);
      out.push('');
    }

    (summary.sections || []).forEach(function (sec) {
      out.push(sec.title + ':');
      sec.lines.forEach(function (l) {
        var title = l.qty > 1 ? l.title + ' ×' + l.qty : l.title;
        if (l.deferred) {
          out.push('  - ' + title + ' (' + (l.note || 'невыученный, в следующую жизнь') + ')');
        } else if (l.number > 0) {
          out.push('  ' + l.number + '. ' + title + ': ' + fmt(l.cost, dec) + noteSuffix(l.note));
        } else {
          out.push('  - ' + title + ': ' + fmt(l.cost, dec) + noteSuffix(l.note));
        }
      });
      out.push('  Сумма: ' + fmt(sec.sum, dec) + ', ост. ' + fmt(sec.remainder, dec));
      out.push('');
    });

    var act = [], skip = [];
    (plan.accumulators || []).forEach(function (a) {
      // Если про заряды накопителя в тексте ничего нет, печатаем только имя.
      var line = a.name;
      if (accumulatorKnown(plan, a)) {
        var v = accumulatorValues(plan, a);
        line += ' (' + number(v.charges) + '/' + number(v.max);
        if (v.rate) line += ', +' + number(v.rate) + '% за жизнь';
        line += ')';
      }
      (a.activate ? act : skip).push(line);
    });
    if (act.length || skip.length) {
      out.push('Активация накопителей:');
      if (act.length) out.push('  - активирую: ' + act.join('; '));
      if (skip.length) out.push('  - не активирую: ' + skip.join('; '));
      out.push('');
    }

    var take = [], keep = [];
    (plan.folios || []).forEach(function (f) {
      var line = f.name + (f.note ? ' (' + f.note + ')' : '');
      (f.take ? take : keep).push(line);
    });
    if (take.length || keep.length) {
      out.push('Фолианты:');
      if (take.length) out.push('  - беру с собой: ' + take.join('; '));
      if (keep.length) out.push('  - оставляю в библиотеке: ' + keep.join('; '));
      out.push('');
    }

    var items = itemMap(plan);
    var deferredNames = (plan.deferred || []).map(function (d) {
      return items[d.key] ? items[d.key].title : null;
    }).filter(Boolean);
    out.push('Невыученные: ' + (deferredNames.length ? deferredNames.join(', ') : '—'));
    out.push('');

    if (summary.notTaken.length) {
      out.push('Не взято: ' + summary.notTaken.map(function (l) { return l.title; }).join(', '));
      out.push('');
    }

    if (summary.warnings.length) {
      out.push('Проверки:');
      summary.warnings.forEach(function (w) { out.push('  ! ' + w); });
      out.push('');
    }

    // Условия, которые касаются будущих жизней: памятка, а не проверка плана.
    if ((summary.future || []).length) {
      out.push('На будущие жизни:');
      summary.future.forEach(function (f) {
        out.push('  - ' + String(f.text).trim() + (f.met ? ' — выполнено' : ' — пока не выполнено'));
      });
      out.push('');
    }

    out.push('Итого потрачено: ' + fmt(summary.total, dec));
    out.push('Остаток: ' + fmt(summary.remainder, dec) + ' XP');
    if (plan.notes) { out.push(''); out.push(String(plan.notes).trim()); }
    return out.join('\n') + '\n';
  }

  /* availableLadders(plan) — шкалы, доступные в этом каркасе. */
  function visibleLadders(plan) {
    return (plan.ladders || []).filter(function (l) { return !l.hidden; });
  }

  /* nextStepCost(plan, ladderKey) — цена следующего шага (null, если дальше нельзя). */
  function nextLadderStep(ladder) {
    var step = ladder.step > 0 ? ladder.step : 1;
    var next = Number(ladder.value) + step;
    if (next > Number(ladder.to) + 1e-9) return null;
    return { value: next, cost: stepCost(ladder.cost, next) };
  }

  return {
    LESSON_BASE: LESSON_BASE,
    LESSON_STEP: LESSON_STEP,
    lessonCost: lessonCost,
    stepCost: stepCost,
    ladderCost: ladderCost,
    computeSummary: computeSummary,
    renderExport: renderExport,
    nextLadderStep: nextLadderStep,
    visibleLadders: visibleLadders,
    itemMap: itemMap,
    missingPrereqs: missingPrereqs,
    accumulatorValues: accumulatorValues,
    accumulatorKnown: accumulatorKnown,
    accumulatorState: accumulatorState,
    folioState: folioState,
    pruneAttachments: pruneAttachments,
    fmt: fmt,
    number: number
  };
});
