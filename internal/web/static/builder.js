/* Конструктор плана: всё считается и рисуется в браузере.
   На сервер уходит только явное «Сохранить» — одним POST-ом. */
(function () {
  'use strict';

  var KT = window.KT;
  var builder = document.getElementById('builder');
  if (!builder || !KT) return;

  var plan = normalizePlan(readJSON('kt-plan'));
  var sections = readJSON('kt-sections') || [];
  var presets = readJSON('kt-presets') || [];
  var dirty = false;
  var filter = '';

  /* Сервер может отдать план со списками-null (так уезжает nil-срез Go),
     поэтому приводим план к предсказуемому виду до первой отрисовки. */
  function normalizePlan(raw) {
    var p = raw && typeof raw === 'object' ? raw : {};
    ['shop', 'picks', 'deferred', 'not_taken', 'ladders', 'accumulators', 'folios', 'multipliers', 'requirements']
      .forEach(function (key) {
        if (!Array.isArray(p[key])) p[key] = [];
      });
    if (!p.options || typeof p.options !== 'object') p.options = { decimals: true, numbered: true };
    p.balance = Number(p.balance) || 0;
    p.earned = Number(p.earned) || 0;
    p.deferred_slots = Math.max(1, Number(p.deferred_slots) || 1);
    if (!p.title) p.title = 'Новый план';
    p.shop.forEach(function (it, i) {
      if (!it.key) it.key = (it.kind || 'item') + ':item' + i;
      if (!it.kind) it.kind = 'item';
      if (!it.section) it.section = 'other';
      it.min_lesson = Number(it.min_lesson) || 1;
      it.cost = Number(it.cost) || 0;
      if (it.already_paid) it.locked = true;
    });
    enforceLocked(p);
    return p;
  }

  /* enforceLocked доставляет в план позиции, которые он обязан содержать
     (Ли Лун уже взяла их сама или не станет смотреть план без них).
     Уже оплаченные встают в начало очереди — в порядке магазина, — чтобы
     нумерация уроков совпадала с текстом; остальные обязательные позиции
     добавляются в конец: их место игрок выбирает сам. */
  function enforceLocked(p) {
    var have = {};
    (p.picks || []).forEach(function (pk) { have[pk.key] = true; });
    var front = [], back = [];
    (p.shop || []).forEach(function (it) {
      if (!it.locked || have[it.key]) return;
      (it.already_paid ? front : back).push({ key: it.key, qty: 1 });
      have[it.key] = true;
    });
    if (front.length || back.length) {
      p.picks = front.concat(p.picks || [], back);
    }
    p.deferred = (p.deferred || []).filter(function (d) { return !isLocked(p, d.key); });
    p.not_taken = (p.not_taken || []).filter(function (k) { return !isLocked(p, k); });
  }

  function isLocked(p, key) {
    var it = (p.shop || []).filter(function (x) { return x.key === key; })[0];
    return !!(it && it.locked);
  }

  function readJSON(id) {
    var el = document.getElementById(id);
    if (!el) return null;
    try { return JSON.parse(el.textContent); } catch (e) { console.error(e); return null; }
  }

  function h(tag, attrs, children) {
    var el = document.createElement(tag);
    if (attrs) {
      Object.keys(attrs).forEach(function (k) {
        var v = attrs[k];
        if (v === null || v === undefined || v === false) return;
        if (k === 'class') el.className = v;
        else if (k === 'text') el.textContent = v;
        else if (k === 'html') el.innerHTML = v;
        else if (k.slice(0, 2) === 'on') el.addEventListener(k.slice(2), v);
        else if (k === 'dataset') Object.keys(v).forEach(function (d) { el.dataset[d] = v[d]; });
        else if (v === true) el.setAttribute(k, '');
        else el.setAttribute(k, v);
      });
    }
    (children || []).forEach(function (c) {
      if (c === null || c === undefined || c === false) return;
      el.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return el;
  }

  function fmt(v) { return KT.fmt(v, plan.options && plan.options.decimals); }

  function sectionMeta(key) {
    for (var i = 0; i < sections.length; i++) if (sections[i].key === key) return sections[i];
    return { key: key, title: key, order: 99 };
  }

  function item(key) {
    for (var i = 0; i < plan.shop.length; i++) if (plan.shop[i].key === key) return plan.shop[i];
    return null;
  }

  function markDirty() {
    dirty = true;
    var badge = document.getElementById('kt-dirty');
    if (badge) badge.hidden = false;
  }

  // --- состояние выбора -------------------------------------------------

  function isPicked(key) {
    return plan.picks.some(function (p) { return p.key === key; });
  }
  function isDeferred(key) {
    return plan.deferred.some(function (d) { return d.key === key; });
  }
  function isNotTaken(key) {
    return plan.not_taken.indexOf(key) >= 0;
  }
  function pickIndex(key) {
    for (var i = 0; i < plan.picks.length; i++) if (plan.picks[i].key === key) return i;
    return -1;
  }

  function setPick(key, on) {
    var idx = pickIndex(key);
    if (on && idx < 0) {
      plan.picks.push({ key: key, qty: 1 });
      plan.not_taken = plan.not_taken.filter(function (k) { return k !== key; });
    } else if (!on && idx >= 0) {
      plan.picks.splice(idx, 1);
    }
    syncLegacySections(key);
  }

  function setDeferred(key, on) {
    var idx = -1;
    plan.deferred.forEach(function (d, i) { if (d.key === key) idx = i; });
    if (on && idx < 0) {
      plan.deferred.push({ key: key, slot: plan.deferred.length + 1 });
      setPick(key, false);
      plan.not_taken = plan.not_taken.filter(function (k) { return k !== key; });
    } else if (!on && idx >= 0) {
      plan.deferred.splice(idx, 1);
      renumberSlots();
    }
  }

  function setNotTaken(key, on) {
    var has = isNotTaken(key);
    if (on && !has) {
      plan.not_taken.push(key);
      setPick(key, false);
      setDeferred(key, false);
    } else if (!on && has) {
      plan.not_taken = plan.not_taken.filter(function (k) { return k !== key; });
    }
  }

  function renumberSlots() {
    plan.deferred.forEach(function (d, i) { d.slot = i + 1; });
  }

  /* carryMarks переносит отметку («активирую» / «беру») из старого списка
     накопителей или фолиантов в новый: совпадение по ключу или названию. */
  function carryMarks(fresh, old, field) {
    return (fresh || []).map(function (item) {
      var prev = (old || []).filter(function (o) {
        return (item.key && o.key === item.key) || (item.name && o.name === item.name);
      })[0];
      if (prev) item[field] = !!prev[field];
      return item;
    });
  }

  /* syncLegacySections — держим разделы плана в согласии с магазином. */
  function syncLegacySections(key) {
    var it = item(key);
    if (!it) return;
    // Накопитель «хардкор» и «равновесие» покупаются в магазине, а отмечаются
    // к активации в списке накопителей — здесь следим, чтобы список знал про
    // купленный накопитель и не потерял его при сохранении.
    if (it.section !== 'accumulators' || !isPicked(key)) return;
    var name = accumulatorName(it.title);
    var found = null;
    plan.accumulators.forEach(function (a) {
      if (a.key === key || a.name === name) found = a;
    });
    if (found) { found.activate = true; return; }
    plan.accumulators.push({ key: key, name: name, activate: true, charges: 0, max: 0, rate: 0 });
  }

  /* accumulatorName убирает из названия покупки слово «накопитель». */
  function accumulatorName(title) {
    return String(title || '').replace(/^накопитель\s+/i, '').replace(/[«»]/g, '');
  }

  // --- каркас -----------------------------------------------------------

  function applySkeletonJson(text, keepPicks) {
    var sk;
    try { sk = JSON.parse(text); }
    catch (e) { alert('JSON не разобрался: ' + e.message); return false; }
    if (!sk || typeof sk !== 'object') { alert('Каркас должен быть JSON-объектом.'); return false; }
    if (!sk.shop || !sk.shop.length) { alert('В каркасе нет ни одной позиции shop.'); return false; }
    var oldShop = plan.shop;
    var oldPicks = plan.picks;
    var oldDeferred = plan.deferred;
    var oldNotTaken = plan.not_taken;

    plan.shop = sk.shop.map(function (it, i) {
      it.key = it.key || (it.kind || 'item') + ':' + slug(it.title || it.full || ('item' + i));
      it.kind = it.kind || 'item';
      it.section = it.section || 'other';
      it.min_lesson = it.min_lesson || 1;
      it.cost = Number(it.cost) || 0;
      if (it.fixed === undefined) it.fixed = it.kind !== 'lesson';
      return it;
    });
    plan.ladders = sk.ladders && sk.ladders.length ? sk.ladders : plan.ladders;
    // Отметки накопителей и фолиантов переносим по ключу (или названию):
    // перезагрузка каркаса не должна стирать их молча. Незаслуженные снимет render.
    plan.accumulators = carryMarks(sk.accumulators, plan.accumulators, 'activate');
    plan.folios = carryMarks(sk.folios, plan.folios, 'take');
    plan.balance = Number(sk.balance) || 0;
    plan.skeleton = sk.name || '';
    plan.earned = Number(sk.earned) || 0;
    plan.subtotal = Number(sk.subtotal) || 0;
    plan.multipliers = sk.multipliers || [];
    plan.requirements = sk.requirements || [];
    plan.deferred_slots = Math.max(1, Number(sk.deferred_slots) || 1);
    if (sk.source) plan.source = sk.source;
    if (sk.notes) plan.notes = sk.notes;

    if (keepPicks === false) {
      plan.picks = [];
      plan.deferred = [];
      plan.not_taken = [];
      enforceLocked(plan);
      return true;
    }
    // Старые ключи, если совпадают по названию, переносим.
    var byTitle = {};
    plan.shop.forEach(function (it) { byTitle[(it.title || '').toLowerCase()] = it.key; });
    var lost = [];
    function remap(list, keyName, reportLost) {
      var out = [];
      (list || []).forEach(function (entry) {
        // «не беру» хранится строками, покупки и отложенные — объектами {key}.
        var k = typeof entry === 'string' ? entry : entry[keyName];
        if (!k) return;
        if (item(k)) { out.push(entry); return; }
        var old = oldShop.filter(function (o) { return o.key === k; })[0];
        if (old && byTitle[(old.title || '').toLowerCase()]) {
          if (keyName === 'key') out.push({ key: byTitle[old.title.toLowerCase()], qty: entry.qty || 1 });
          else out.push({ key: byTitle[old.title.toLowerCase()], slot: entry.slot || 0 });
          return;
        }
        // Покупки, которых в новом каркасе больше нет, из плана уходят:
        // в этом посмертии их не предлагают. О таком надо сказать вслух.
        if (reportLost) lost.push((old && old.title) || k);
      });
      return out;
    }
    plan.picks = remap(oldPicks, 'key', true);
    plan.deferred = remap(oldDeferred, 'key', true);
    plan.not_taken = remap(oldNotTaken, 'key', false).map(function (e) { return typeof e === 'string' ? e : e.key; });
    enforceLocked(plan);
    renumberSlots();
    if (lost.length) {
      alert('В новом каркасе нет покупок: ' + lost.join(', ') +
        '. Они убраны из плана — в этом посмертии их не предлагают, очки освободились.');
    }
    return true;
  }

  function skeletonJson() {
    return JSON.stringify({
      name: plan.skeleton || plan.title,
      balance: plan.balance,
      earned: plan.earned || 0,
      subtotal: plan.subtotal || 0,
      multipliers: plan.multipliers || [],
      source: plan.source || '',
      notes: plan.notes || '',
      shop: plan.shop,
      ladders: plan.ladders,
      accumulators: plan.accumulators,
      folios: plan.folios,
      requirements: plan.requirements || [],
      deferred_slots: plan.deferred_slots || 1
    }, null, 2);
  }

  function slug(s) {
    var map = { 'а': 'a', 'б': 'b', 'в': 'v', 'г': 'g', 'д': 'd', 'е': 'e', 'ё': 'e', 'ж': 'zh', 'з': 'z', 'и': 'i', 'й': 'y', 'к': 'k', 'л': 'l', 'м': 'm', 'н': 'n', 'о': 'o', 'п': 'p', 'р': 'r', 'с': 's', 'т': 't', 'у': 'u', 'ф': 'f', 'х': 'h', 'ц': 'c', 'ч': 'ch', 'ш': 'sh', 'щ': 'sch', 'ъ': '', 'ы': 'y', 'ь': '', 'э': 'e', 'ю': 'yu', 'я': 'ya' };
    var out = String(s).toLowerCase().split('').map(function (ch) { return map[ch] !== undefined ? map[ch] : ch; }).join('');
    out = out.replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 48);
    return out || 'item';
  }

  function download(name, text, type) {
    var blob = new Blob([text], { type: type || 'text/plain;charset=utf-8' });
    var url = URL.createObjectURL(blob);
    var a = h('a', { href: url, download: name });
    document.body.appendChild(a);
    a.click();
    setTimeout(function () { URL.revokeObjectURL(url); a.remove(); }, 500);
  }

  // --- отрисовка --------------------------------------------------------

  function render() {
    // Отметки «активирую» и «беру» план обязан заслужить: если покупку сняли,
    // незаряженный накопитель или невыкупленный фолиант перестают быть отмеченными.
    if (KT.pruneAttachments(plan) > 0) markDirty();
    builder.textContent = '';
    builder.appendChild(renderToolbar());
    var grid = h('div', { class: 'builder-grid' }, [
      h('div', { class: 'builder-main', id: 'kt-sections' }, renderSections()),
      renderSidebar()
    ]);
    builder.appendChild(grid);
    builder.appendChild(renderImportPanel());
    builder.appendChild(renderExportModal());
    recompute();
    if (filter) applyFilter(filter);
  }

  function renderToolbar() {
    return h('div', { class: 'toolbar' }, [
      h('label', { class: 'field grow' }, [
        h('span', { text: 'Название плана' }),
        h('input', {
          id: 'kt-title', value: plan.title || '',
          oninput: function (e) { plan.title = e.target.value; markDirty(); }
        })
      ]),
      h('label', { class: 'field tiny' }, [
        h('span', { text: 'Смайлик' }),
        h('input', {
          id: 'kt-emoji', value: plan.emoji || '', maxlength: 8,
          oninput: function (e) { plan.emoji = e.target.value; markDirty(); }
        })
      ]),
      h('label', { class: 'field' }, [
        h('span', { text: 'Баланс XP' }),
        h('input', {
          id: 'kt-balance', class: 'num', value: plan.balance || 0, inputmode: 'decimal',
          oninput: function (e) {
            plan.balance = parseNum(e.target.value);
            markDirty();
            recompute();
          }
        })
      ]),
      h('label', { class: 'field' }, [
        h('span', { text: 'Поиск по магазину' }),
        h('input', {
          id: 'kt-search', placeholder: 'например, талант', value: filter,
          oninput: function (e) { applyFilter(e.target.value); }
        })
      ]),
      h('div', { class: 'toolbar-actions' }, [
        h('button', { class: 'btn ghost', type: 'button', onclick: openImport, text: 'Каркас' }),
        h('button', { class: 'btn ghost', type: 'button', onclick: openExport, text: 'Экспорт' }),
        h('button', { class: 'btn', type: 'button', onclick: save, text: 'Сохранить' }),
        h('span', { class: 'dirty', id: 'kt-dirty', hidden: !dirty, text: 'есть несохранённое' })
      ])
    ]);
  }

  function renderSidebar() {
    return h('aside', { class: 'sidebar' }, [
      h('div', { class: 'card summary' }, [
        h('div', { class: 'sum-row' }, [
          h('span', { class: 'muted small', text: 'Баланс' }),
          h('b', { id: 'sum-balance', text: fmt(plan.balance) })
        ]),
        h('div', { class: 'sum-row' }, [
          h('span', { class: 'muted small', text: 'Потрачено' }),
          h('b', { id: 'sum-spent', text: '0' })
        ]),
        h('div', { class: 'sum-row big' }, [
          h('span', { class: 'muted small', text: 'Остаток' }),
          h('b', { id: 'sum-left', text: fmt(plan.balance) })
        ]),
        h('div', { class: 'bar' }, [h('i', { id: 'sum-bar' })]),
        h('div', { class: 'muted small', id: 'sum-slots', text: '' })
      ]),
      h('div', { class: 'card', id: 'kt-rewards', hidden: true }),
      h('div', { class: 'card', id: 'kt-req' }),
      renderAttachments(),
      h('div', { class: 'card', id: 'kt-warnings' }),
      h('nav', { class: 'card sec-nav', id: 'kt-nav' })
    ]);
  }

  /* renderAttachments — какие накопители активируем и какие фолианты берём:
     в плане это отдельные строки, которых в магазине нет. Отметить можно только
     то, что план действительно может: накопитель — заряженный или купленный,
     фолиант — с уроком метода и выкупом. Сам выбор фолианта бесплатный. */
  function renderAttachments() {
    var accs = plan.accumulators || [];
    var folios = plan.folios || [];
    if (!accs.length && !folios.length) return null;
    var body = [
      h('h3', { text: 'Накопители и фолианты' }),
      h('div', { class: 'muted small', text: 'Отметь, что активируешь и какие фолианты берёшь с собой. Брать фолиант бесплатно: цена — это покупки «взять фолиант» и «выкупить метод» в магазине.' })
    ];
    if (accs.length) {
      body.push(h('div', { class: 'attach-list' }, accs.map(function (a) {
        var st = KT.accumulatorState(plan, a);
        // Заряды печатаем, только если они вообще известны: с учётом покупок
        // («заряд великого успеха», «максимум зарядов», «скорость зарядки»).
        var label = a.name;
        if (KT.accumulatorKnown(plan, a)) {
          var v = KT.accumulatorValues(plan, a);
          label += ' (' + KT.number(v.charges) + '/' + KT.number(v.max)
            + (v.rate ? ', +' + KT.number(v.rate) + '% за жизнь' : '') + ')';
        }
        if (!st.ready) label += ' — ' + st.reason;
        return h('label', {
          class: 'attach' + (st.ready ? '' : ' off'),
          title: st.ready ? 'можно активировать' : st.reason
        }, [
          h('input', {
            type: 'checkbox', checked: !!a.activate, disabled: !st.ready,
            onchange: function (e) { a.activate = e.target.checked; markDirty(); recompute(); }
          }),
          h('span', { text: label })
        ]);
      })));
    }
    if (folios.length) {
      body.push(h('div', { class: 'attach-list' }, folios.map(function (f) {
        var st = KT.folioState(plan, f);
        // Когда каркас не знает условий, показываем как раньше цену выкупа.
        var label = f.name;
        if ((f.requires || []).length) {
          label += st.ready ? ' — выкуп взят, беру' : ' — ' + st.reason;
        } else if (f.cost) {
          label += ' — ' + f.cost;
        }
        return h('label', {
          class: 'attach' + (st.ready ? '' : ' off'),
          title: st.ready ? 'можно взять с собой' : st.reason
        }, [
          h('input', {
            type: 'checkbox', checked: !!f.take, disabled: !st.ready,
            onchange: function (e) { f.take = e.target.checked; markDirty(); recompute(); }
          }),
          h('span', { text: label })
        ]);
      })));
    }
    return h('div', { class: 'card', id: 'kt-attach' }, body);
  }

  /* renderRewards показывает награды за порядок, которые план уже получает. */
  function renderRewards(summary) {
    var box = document.getElementById('kt-rewards');
    if (!box) return;
    box.textContent = '';
    var list = summary.rewards || [];
    if (!list.length) {
      box.hidden = true;
      return;
    }
    box.hidden = false;
    box.appendChild(h('h3', { text: 'Награды за порядок' }));
    box.appendChild(h('ul', { class: 'reward-list' }, list.map(function (r) {
      return h('li', { text: r });
    })));
  }

  /* renderRequirements показывает условия каркаса и их выполнение.
     Условия «на будущие жизни» — памятка: они не попадают в проверки. */
  function renderRequirements() {
    var box = document.getElementById('kt-req');
    if (!box) return;
    box.textContent = '';
    var reqs = plan.requirements || [];
    if (!reqs.length) {
      box.hidden = true;
      return;
    }
    box.hidden = false;
    var groups = [
      { title: 'Условия каркаса', soft: false },
      { title: 'На будущие жизни (не проверка)', soft: true }
    ];
    groups.forEach(function (group) {
      var list = reqs.filter(function (req) { return !!req.soft === group.soft; });
      if (!list.length) return;
      box.appendChild(h('h3', { class: group.soft ? 'muted' : '', text: group.title }));
      list.forEach(function (req) {
        var keys = req.keys || [];
        var met = req.kind === 'all_of'
          ? keys.every(function (k) { return isPicked(k); })
          : keys.some(function (k) { return isPicked(k); });
        var names = keys.map(function (k) {
          var it = item(k);
          return it ? it.title : k;
        });
        var mark = met ? '✓' : (group.soft ? '·' : '✗');
        box.appendChild(h('div', { class: 'req ' + (group.soft ? 'soft ' : '') + (met ? 'met' : 'unmet') }, [
          h('span', { class: 'req-mark', text: mark }),
          h('span', { class: 'req-text' }, [
            h('span', { text: (req.message || '') + ' ' }),
            h('span', { class: 'muted small', text: '(' + (req.kind === 'all_of' ? 'нужны: ' : 'нужно одно из: ') + names.join(', ') + ')' })
          ])
        ]));
      });
    });
  }

  function renderSections() {
    var order = sections.slice().sort(function (a, b) { return (a.order || 99) - (b.order || 99); });
    var groups = {};
    plan.shop.forEach(function (it) {
      var sec = it.section || 'other';
      (groups[sec] = groups[sec] || []).push(it);
    });
    var ladders = {};
    (plan.ladders || []).forEach(function (l) {
      if (l.hidden) return;
      var sec = l.section || 'other';
      (ladders[sec] = ladders[sec] || []).push(l);
    });

    var out = [];
    order.forEach(function (sec) {
      var items = orderedItems(sec.key, groups[sec.key] || []);
      var lads = ladders[sec.key] || [];
      if (!items.length && !lads.length && sec.key !== 'lessons') return;
      out.push(renderSection(sec, items, lads));
    });
    // Позиции из неизвестных секций — в «прочее».
    return out;
  }

  /* orderedItems расставляет строки секции уроков в порядке покупки: сверху то,
     что берётся раньше. Так перестановка уроков видна глазами, а не только в
     номерах. Отложенные (невыученные) идут следом, остальное — как в магазине. */
  function orderedItems(secKey, items) {
    if (secKey !== 'lessons') return items;
    var rank = {}, drank = {};
    plan.picks.forEach(function (pk, i) {
      if (rank[pk.key] === undefined) rank[pk.key] = i;
    });
    (plan.deferred || []).forEach(function (d, i) {
      if (drank[d.key] === undefined) drank[d.key] = i;
    });
    var picked = [], deferred = [], rest = [];
    items.forEach(function (it) {
      if (rank[it.key] !== undefined) picked.push(it);
      else if (drank[it.key] !== undefined) deferred.push(it);
      else rest.push(it);
    });
    picked.sort(function (a, b) { return rank[a.key] - rank[b.key]; });
    deferred.sort(function (a, b) { return drank[a.key] - drank[b.key]; });
    return picked.concat(deferred, rest);
  }

  function renderSection(sec, items, ladders) {
    var body = [];
    if (ladders.length) {
      body.push(h('div', { class: 'ladders' }, ladders.map(renderLadder)));
    }
    if (items.length) {
      body.push(h('div', { class: 'items' }, items.map(function (it) { return renderItem(it, sec.key); })));
    }
    if (!items.length && sec.key === 'lessons') {
      body.push(h('p', { class: 'muted small', text: 'Магазин пуст: загрузи каркас посмертия кнопкой «Каркас».' }));
    }
    return h('section', { class: 'sec', dataset: { section: sec.key }, id: 'sec-' + sec.key }, [
      h('header', { class: 'sec-head' }, [
        h('h2', { text: sec.title }),
        sec.hint ? h('span', { class: 'muted small', text: sec.hint }) : null,
        h('span', { class: 'sec-sum', dataset: { sum: sec.key } }, [
          h('span', { class: 'muted small', text: 'сумма ' }),
          h('b', { text: '0' }),
          h('span', { class: 'muted small', text: ' · ост. ' }),
          h('b', { text: fmt(plan.balance) })
        ])
      ]),
      h('div', { class: 'sec-body' }, body),
      sec.key === 'lessons' ? h('div', { class: 'sec-tools' }, [
        h('button', {
          class: 'btn ghost small', type: 'button', onclick: function () {
            sortPicks();
            markDirty(); // порядок изменился — план надо сохранить
            render();
          }, text: 'Разложить уроки по требованиям'
        }),
        h('span', {
          class: 'muted small' + (hasMinViolation() ? ' warn-live' : ''),
          text: hasMinViolation()
            ? 'есть уроки раньше своего «минимум n-ным» — нажми, чтобы разложить'
            : 'сортирует выбранные уроки так, чтобы «минимум n-ным» выполнялось'
        })
      ]) : null
    ]);
  }

  function renderLadder(l) {
    var next = KT.nextLadderStep(l);
    var owned = h('input', {
      class: 'num ladder-owned', type: 'number', value: KT.number(l.from),
      min: 0, max: l.to, step: l.step,
      title: 'сколько уже есть у Ли Лун — за это опыт не списывается',
      onchange: function (e) {
        var v = parseNum(e.target.value);
        v = Math.max(0, Math.min(l.to, Math.round(v / l.step) * l.step));
        l.from = v;
        if (Number(l.value) < v) l.value = v;
        markDirty();
        render();
      }
    });
    var input = h('input', {
      class: 'num ladder-value', type: 'number', value: l.value,
      min: l.from, max: l.to, step: l.step,
      onchange: function (e) {
        var v = parseNum(e.target.value);
        v = Math.max(l.from, Math.min(l.to, Math.round(v / l.step) * l.step));
        l.value = v;
        e.target.value = v;
        markDirty();
        recompute();
      }
    });
    return h('div', { class: 'row ladder', dataset: { ladder: l.key } }, [
      h('div', { class: 'ladder-main' }, [
        h('div', { class: 'item-title', text: l.title }),
        h('div', { class: 'muted small ladder-hint' }, [
          document.createTextNode('уже есть: '),
          owned,
          document.createTextNode(fmtLadderValue(l, Number(l.from)) + ' · покупаю до ' +
            fmtLadderValue(l, Number(l.to)) + ', шаг ' + KT.number(l.step) +
            (l.note ? ' · ' + l.note : ''))
        ])
      ]),
      h('div', { class: 'ladder-ctl' }, [
        h('button', {
          class: 'btn tiny', type: 'button', text: '−',
          onclick: function () { stepLadder(l, -1); }
        }),
        input,
        h('button', {
          class: 'btn tiny', type: 'button', disabled: !next,
          title: next ? 'следующий шаг: ' + fmt(next.cost) + ' XP' : 'дальше нельзя',
          onclick: function () { stepLadder(l, 1); }
        }, [document.createTextNode(next ? '+ (' + fmt(next.cost) + ')' : '+')])
      ]),
      h('div', {
        class: 'item-cost', dataset: { cost: l.key },
        title: 'опыт за прирост от ' + fmtLadderValue(l, Number(l.from)) + ' до ' + fmtLadderValue(l, Number(l.value)),
        text: fmt(KT.ladderCost(l))
      })
    ]);
  }

  /* fmtLadderValue печатает значение шкалы так же, как её название. */
  function fmtLadderValue(l, v) {
    if (l.unit === 'd') return 'd' + KT.number(v);
    return KT.number(v) + (l.unit || '');
  }

  function stepLadder(l, dir) {
    var step = l.step > 0 ? l.step : 1;
    var v = Number(l.value) + dir * step;
    v = Math.max(l.from, Math.min(l.to, v));
    if (v === Number(l.value)) return;
    l.value = v;
    markDirty();
    render();
  }

  function renderItem(it, sectionKey) {
    var tags = [];
    if (it.locked) {
      tags.push(h('span', {
        class: 'tag lock',
        title: it.lock_note || 'позиция обязательна по тексту',
        text: it.already_paid ? '🔒 уже взято' : '🔒 обязательно'
      }));
    }
    var reward = rewardHint(it);
    if (reward) tags.push(reward.tag);
    if (it.min_lesson > 1) {
      tags.push(h('span', {
        class: 'tag warn', dataset: { minTag: it.min_lesson },
        title: 'можно взять как минимум ' + it.min_lesson + '-м уроком',
        text: 'минимум ' + it.min_lesson + '-м уроком'
      }));
    }
    if ((it.tags || []).indexOf('one_chance') >= 0) tags.push(h('span', { class: 'tag warn', text: 'только один раз в жизни' }));
    if ((it.tags || []).indexOf('trophy') >= 0) tags.push(h('span', { class: 'tag', text: 'трофей' }));
    if ((it.tags || []).indexOf('osvoeno') >= 0) tags.push(h('span', { class: 'tag', text: 'освоено' }));
    if ((it.tags || []).indexOf('defer_slot') >= 0) tags.push(h('span', { class: 'tag ok', text: '+1 слот невыученных' }));
    if (it.fixed && it.kind === 'lesson') tags.push(h('span', { class: 'tag', text: 'фиксированная цена' }));
    if (it.requires && it.requires.length) tags.push(h('span', { class: 'tag', text: 'нужен урок' }));
    if (it.excludes && it.excludes.length) tags.push(h('span', { class: 'tag warn', text: 'несовместимо' }));

    var lockedHint = it.locked
      ? h('div', { class: 'muted small' }, [document.createTextNode(it.lock_note || 'позиция обязательна по тексту')])
      : null;

    var rewardLine = reward ? reward.line : null;

    var row = h('div', {
      class: 'row item' + (it.locked ? ' locked-item' : ''),
      dataset: { key: it.key, kind: it.kind, section: sectionKey },
      title: it.desc || ''
    }, [
      h('span', { class: 'badge', dataset: { badge: it.key }, text: '' }),
      h('div', { class: 'item-main' }, [
        h('div', { class: 'item-title' }, [it.title].concat(tags)),
        lockedHint,
        rewardLine,
        it.desc ? h('details', { class: 'item-desc' }, [
          h('summary', { text: 'описание' }),
          h('p', { text: it.desc })
        ]) : null
      ]),
      h('div', { class: 'item-cost', dataset: { cost: it.key }, text: fmt(it.cost) }),
      h('div', { class: 'item-state' }, [
        h('button', {
          class: 'state pick', type: 'button', dataset: { act: 'pick', key: it.key }, text: 'беру',
          disabled: it.locked,
          title: it.locked ? 'снять нельзя: ' + (it.lock_note || 'позиция обязательна') : '',
          onclick: function () { setState(it.key, 'pick'); }
        }),
        it.kind === 'lesson' ? h('button', {
          class: 'state defer', type: 'button', dataset: { act: 'defer', key: it.key }, text: 'невыученный',
          disabled: it.locked,
          title: it.locked ? 'обязательную позицию нельзя отложить' : '',
          onclick: function () { setState(it.key, 'defer'); }
        }) : null,
        h('button', {
          class: 'state skip', type: 'button', dataset: { act: 'skip', key: it.key }, text: 'не беру',
          disabled: it.locked,
          title: it.locked ? 'отказаться нельзя: ' + (it.lock_note || 'позиция обязательна') : '',
          onclick: function () { setState(it.key, 'skip'); }
        })
      ]),
      it.kind === 'lesson' && !it.already_paid ? h('div', { class: 'item-order' }, [
        h('button', { class: 'btn tiny', type: 'button', text: '↑', title: 'выше в очереди уроков', onclick: function () { movePick(it.key, -1); } }),
        h('button', { class: 'btn tiny', type: 'button', text: '↓', title: 'ниже в очереди уроков', onclick: function () { movePick(it.key, 1); } })
      ]) : null,
      // Позиции, которые можно взять несколько раз (фолианты, лестница в небо):
      // количество задаётся здесь, цену считает движок.
      it.repeat > 1 ? h('div', { class: 'item-qty' }, [
        h('button', {
          class: 'btn tiny', type: 'button', text: '−', title: 'взять на одну меньше',
          onclick: function () { setQty(it.key, -1); }
        }),
        h('span', {
          class: 'qty-value', dataset: { qty: it.key },
          text: '×' + (pickQty(it.key) || 0)
        }),
        h('button', {
          class: 'btn tiny', type: 'button', text: '+', title: 'взять ещё одну (всего до ' + it.repeat + ')',
          onclick: function () { setQty(it.key, 1); }
        })
      ]) : null
    ]);
    return row;
  }

  /* pickQty — сколько раз позиция взята (0, если её нет в плане). */
  function pickQty(key) {
    for (var i = 0; i < plan.picks.length; i++) {
      if (plan.picks[i].key === key) return plan.picks[i].qty > 0 ? plan.picks[i].qty : 1;
    }
    return 0;
  }

  /* setQty меняет количество покупок повторяемой позиции. */
  function setQty(key, dir) {
    var it = item(key);
    if (!it || !(it.repeat > 1)) return;
    var idx = pickIndex(key);
    if (idx < 0) { // ещё не взято: «+» сразу берёт первую штуку
      if (dir < 0) return;
      setPick(key, true);
      markDirty();
      render();
      return;
    }
    var cur = plan.picks[idx].qty > 0 ? plan.picks[idx].qty : 1;
    var next = cur + dir;
    if (next < 1) { setPick(key, false); markDirty(); render(); return; }
    if (next > it.repeat) return;
    plan.picks[idx].qty = next;
    markDirty();
    render();
  }

  function setState(key, state) {
    if (isLocked(plan, key)) return; // обязательные позиции не снимаются
    var picked = isPicked(key), deferred = isDeferred(key), skipped = isNotTaken(key);
    if (state === 'pick') {
      if (picked) setPick(key, false);
      else { setPick(key, true); setDeferred(key, false); }
      if (state === 'pick' && !picked) setNotTaken(key, false);
    } else if (state === 'defer') {
      if (deferred) setDeferred(key, false);
      else { setDeferred(key, true); setNotTaken(key, false); }
    } else {
      if (skipped) setNotTaken(key, false);
      else setNotTaken(key, true);
    }
    markDirty();
    render();
  }

  function movePick(key, dir) {
    // Уже взятые ею уроки стоят первыми: их место задано текстом.
    if (isPinned(key)) return;
    var idx = pickIndex(key);
    if (idx < 0) return;
    var to = idx + dir;
    if (to < 0 || to >= plan.picks.length) return;
    if (isPinned(plan.picks[to].key)) return; // не проталкиваемся сквозь уже взятые
    var tmp = plan.picks[idx];
    plan.picks[idx] = plan.picks[to];
    plan.picks[to] = tmp;
    markDirty();
    render();
  }

  /* isPinned — позиция, место которой задано текстом (уже взята ею самой). */
  function isPinned(key) {
    var it = item(key);
    return !!(it && it.already_paid);
  }

  /* pickIndex — место позиции в очереди покупок, -1 если её нет. */
  function pickIndex(key) {
    for (var i = 0; i < plan.picks.length; i++) {
      if (plan.picks[i].key === key) return i;
    }
    return -1;
  }

  /* moveAfter ставит позицию сразу после указанной. */
  function moveAfter(key, refKey) {
    if (!placeAfter(key, refKey)) return;
    markDirty();
    render();
  }

  /* placeAfter переставляет позицию в очереди, ничего не перерисовывая. */
  function placeAfter(key, refKey) {
    var i = pickIndex(key), ref = pickIndex(refKey);
    if (i < 0 || ref < 0 || i === ref + 1) return false;
    var pk = plan.picks.splice(i, 1)[0];
    plan.picks.splice(pickIndex(refKey) + 1, 0, pk);
    return true;
  }

  /* rewardHint — пометка о награде за порядок: тег и строка под названием. */
  function rewardHint(it) {
    var r = it.reward_after;
    if (!r || !r.key) return null;
    var ref = item(r.key);
    var refTitle = ref ? ref.title : r.key;
    var title = r.title || 'квестовая награда';
    var st = rewardState(it);
    var tag = h('span', {
      class: 'tag ' + (st === 'lost' ? 'warn' : (st === 'got' ? 'ok' : '')),
      title: 'награда за порядок: возьми после «' + refTitle + '»',
      text: st === 'got' ? '✓ ' + title : (st === 'lost' ? '✗ ' + title : 'после «' + refTitle + '»')
    });
    var text;
    if (st === 'got') {
      text = title + ': ' + (r.note || '') + ' — урок стоит после «' + refTitle + '»';
    } else if (st === 'lost') {
      text = 'Взято раньше «' + refTitle + '» — ' + title.toLowerCase() + ' не достанется' +
        (r.lose ? ': ' + r.lose : '');
    } else {
      text = 'Даст ' + title.toLowerCase() + ', если взять после «' + refTitle + '»' + (r.note ? ': ' + r.note : '');
    }
    var line = h('div', { class: 'small reward-hint ' + st }, [document.createTextNode(text)]);
    if (st === 'lost') {
      line.appendChild(h('button', {
        class: 'btn tiny', type: 'button', text: 'поставить после «' + refTitle + '»',
        onclick: function () { moveAfter(it.key, r.key); }
      }));
    }
    return { tag: tag, line: line };
  }

  /* rewardState — получена ли награда за порядок: got | lost | idle. */
  function rewardState(it) {
    var r = it.reward_after;
    if (!r || !r.key || !isPicked(it.key)) return 'idle';
    if ((plan.deferred || []).some(function (d) { return d.key === it.key; })) return 'idle';
    var ref = pickIndex(r.key);
    if (ref < 0) return 'idle';
    return pickIndex(it.key) > ref ? 'got' : 'lost';
  }

  /* sortPicks — раскладка уроков: сначала уже оплаченные (их место задано
     текстом), дальше по требованию «минимум n-ным», затем по порядку магазина. */
  function sortPicks() {
    var meta = {};
    plan.shop.forEach(function (it) { meta[it.key] = it; });
    var order = {};
    plan.shop.forEach(function (it, i) { order[it.key] = i; });
    plan.picks.sort(function (a, b) {
      var A = meta[a.key] || {}, B = meta[b.key] || {};
      if (!!A.already_paid !== !!B.already_paid) return A.already_paid ? -1 : 1;
      if (A.kind === 'lesson' && B.kind === 'lesson' && !A.fixed && !B.fixed) {
        var ma = A.min_lesson || 1, mb = B.min_lesson || 1;
        if (ma !== mb) return ma - mb;
        return (order[a.key] || 0) - (order[b.key] || 0);
      }
      return 0;
    });
    // Награда за порядок важнее «минимума»: если урок должен идти после другого,
    // ставим его сразу за ним, иначе награда потеряется.
    plan.shop.forEach(function (it) {
      if (!it.reward_after || !it.reward_after.key) return;
      if (pickIndex(it.key) < 0 || pickIndex(it.reward_after.key) < 0) return;
      if (pickIndex(it.key) < pickIndex(it.reward_after.key)) placeAfter(it.key, it.reward_after.key);
    });
  }

  // --- пересчёт ---------------------------------------------------------

  function recompute() {
    var summary = KT.computeSummary(plan, sections);
    var lineByKey = {};
    summary.sections.forEach(function (sec) {
      sec.lines.forEach(function (l) { if (!lineByKey[l.key]) lineByKey[l.key] = l; });
    });

    // числа в строках магазина
    var paidCount = summary.paidLessons;
    Array.prototype.forEach.call(document.querySelectorAll('.item'), function (row) {
      var key = row.dataset.key;
      var it = item(key);
      if (!it) return;
      var line = lineByKey[key];
      var costEl = row.querySelector('[data-cost]');
      var badge = row.querySelector('[data-badge]');
      if (it.kind === 'lesson' && !it.fixed && !line) {
        // ещё не выбран: показываем цену, которая будет при следующем клике
        var previewIdx = paidCount + 1;
        if (costEl) costEl.textContent = fmt(KT.lessonCost(previewIdx));
      } else if (costEl && line) {
        costEl.textContent = fmt(line.cost);
      } else if (costEl) {
        costEl.textContent = fmt(it.cost);
      }
      if (badge) badge.textContent = line && line.number ? String(line.number) : '';
      var qtyEl = row.querySelector('[data-qty]');
      if (qtyEl) qtyEl.textContent = '×' + pickQty(key);
      row.classList.toggle('picked', isPicked(key));
      row.classList.toggle('deferred', isDeferred(key));
      row.classList.toggle('skipped', isNotTaken(key));
      row.querySelectorAll('.state').forEach(function (btn) {
        var act = btn.dataset.act;
        var on = (act === 'pick' && isPicked(key)) || (act === 'defer' && isDeferred(key)) || (act === 'skip' && isNotTaken(key));
        btn.classList.toggle('on', on);
        btn.setAttribute('aria-pressed', on ? 'true' : 'false');
      });
      // подсветка требования «минимум n-ным»
      var num = line && line.number ? line.number : (isPicked(key) ? paidCount : paidCount + 1);
      var minTag = row.querySelector('[data-min-tag]');
      if (minTag) {
        var min = Number(minTag.dataset.minTag) || 1;
        // Отложенный урок номер не занимает — требование к нему не относится.
        var met = isDeferred(key) || min <= num;
        minTag.classList.toggle('ok', met);
        minTag.classList.toggle('warn', !met);
        minTag.title = isDeferred(key)
          ? 'отложен невыученным: номер урока не занимает'
          : 'можно взять как минимум ' + min + '-м уроком' + (met
            ? ' — условие выполнено: эта позиция ' + num + '-я'
            : ' — сейчас эта позиция ' + num + '-я');
      }
      if (it.kind === 'lesson' && !it.fixed && (it.min_lesson || 1) > num && !isDeferred(key)) {
        row.classList.add('locked');
      } else {
        row.classList.remove('locked');
      }
    });

    // шкалы
    Array.prototype.forEach.call(document.querySelectorAll('[data-ladder]'), function (row) {
      var key = row.dataset.ladder;
      var l = (plan.ladders || []).filter(function (x) { return x.key === key; })[0];
      if (!l) return;
      var el = row.querySelector('[data-cost]');
      var line = lineByKey[key];
      if (el) el.textContent = fmt(line ? line.cost : KT.ladderCost(l));
      var input = row.querySelector('.ladder-value');
      if (input && document.activeElement !== input) input.value = l.value;
    });

    // суммы секций
    var byKey = {};
    summary.sections.forEach(function (s) { byKey[s.key] = s; });
    Array.prototype.forEach.call(document.querySelectorAll('[data-section]'), function (secEl) {
      var key = secEl.dataset.section;
      var holder = secEl.querySelector('[data-sum]');
      if (!holder) return;
      var s = byKey[key];
      var sumB = holder.querySelectorAll('b')[0];
      var remB = holder.querySelectorAll('b')[1];
      if (sumB) sumB.textContent = fmt(s ? s.sum : 0);
      if (remB) remB.textContent = fmt(s ? s.remainder : plan.balance);
    });

    // сайдбар
    setText('sum-balance', fmt(plan.balance));
    setText('sum-spent', fmt(summary.total));
    setText('sum-left', fmt(summary.remainder));
    setText('sum-slots', 'слоты невыученных: ' + summary.deferredUsed + ' / ' + summary.deferredLimit);
    var bar = document.getElementById('sum-bar');
    if (bar) {
      var pct = plan.balance > 0 ? Math.min(100, (summary.total / plan.balance) * 100) : 0;
      bar.style.width = pct.toFixed(1) + '%';
      bar.classList.toggle('over', summary.remainder < -1e-9);
    }

    var warnBox = document.getElementById('kt-warnings');
    if (warnBox) {
      warnBox.textContent = '';
      if (!summary.warnings.length) {
        warnBox.appendChild(h('p', { class: 'muted small', text: 'Проверок нет: план сходится.' }));
      } else {
        warnBox.appendChild(h('h3', { text: 'Проверки' }));
        warnBox.appendChild(h('ul', { class: 'warn-list' }, summary.warnings.map(function (w) {
          return h('li', { text: w });
        })));
      }
    }

    renderRewards(summary);
    renderRequirements();

    // В панели экспорта сразу видно, что план не пройдёт проверки.
    var exportWarn = document.getElementById('kt-export-warn');
    if (exportWarn) {
      exportWarn.textContent = '';
      if (summary.warnings.length) {
        exportWarn.hidden = false;
        exportWarn.appendChild(h('b', { text: '⚠ Проверки не выполняются: ' + summary.warnings.length }));
        exportWarn.appendChild(h('ul', { class: 'warn-list' }, summary.warnings.map(function (w) {
          return h('li', { text: w });
        })));
        exportWarn.appendChild(h('p', { class: 'muted small', text: 'Этот же список уедет в выгрузку: Ли Лун увидит его вместе с планом.' }));
      } else {
        exportWarn.hidden = true;
      }
    }

    var nav = document.getElementById('kt-nav');
    if (nav) {
      nav.textContent = '';
      nav.appendChild(h('h3', { text: 'Секции' }));
      summary.sections.forEach(function (s) {
        nav.appendChild(h('a', {
          href: '#sec-' + s.key, class: 'nav-link',
          onclick: function (e) {
            e.preventDefault();
            var el = document.getElementById('sec-' + s.key);
            if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
          }
        }, [
          h('span', { text: s.title }),
          h('b', { text: fmt(s.sum) }),
          h('i', { class: 'muted small', text: 'ост. ' + fmt(s.remainder) })
        ]));
      });
      if (!summary.sections.length) nav.appendChild(h('p', { class: 'muted small', text: 'Пока ничего не выбрано.' }));
    }

    var area = document.getElementById('kt-export-text');
    if (area) area.value = KT.renderExport(plan, summary, sections);
    var skArea = document.getElementById('kt-skeleton-text');
    if (skArea && !skArea.dataset.touched) skArea.value = skeletonJson();
    builder.dataset.summary = JSON.stringify({ spent: summary.total, left: summary.remainder });
  }

  function setText(id, text) {
    var el = document.getElementById(id);
    if (el) el.textContent = text;
  }

  // --- панели -----------------------------------------------------------

  /* loadPreset тянет готовый каркас и накладывает его на план. */
  function loadPreset(file) {
    if (!file) return;
    fetch('/presets/' + encodeURIComponent(file))
      .then(function (r) { return r.json(); })
      .then(function (sk) {
        if (applySkeletonJson(JSON.stringify(sk), true)) {
          markDirty();
          closePanel('kt-import');
          render();
        }
      })
      .catch(function (err) { alert('Не удалось загрузить каркас: ' + err.message); });
  }

  function renderImportPanel() {
    var select = h('select', { onchange: function (e) { loadPreset(e.target.value); } },
      [h('option', { value: '', text: '— выбрать готовый —' })].concat(presets.map(function (p) {
        return h('option', { value: p.file, text: p.name + ' · ' + p.balance + ' XP' });
      })));

    // Каркас хранится внутри плана: если готовый каркас с тем же названием
    // обновляли, цены и условия в плане отстали — подсказываем и даём кнопку.
    var own = presets.filter(function (p) { return p.name === plan.skeleton; })[0];
    var ownRow = own ? h('div', { class: 'muted small' }, [
      'Каркас в плане: «' + plan.skeleton + '». ',
      h('button', { class: 'btn ghost small', type: 'button', text: 'Обновить из готового', onclick: function () { loadPreset(own.file); } }),
      ' — покупки и отметки сохранятся, а цены и условия станут текущими.'
    ]) : null;

    var area = h('textarea', {
      id: 'kt-skeleton-text', class: 'mono', rows: 16, spellcheck: 'false',
      oninput: function (e) { e.target.dataset.touched = '1'; }
    });
    area.value = skeletonJson();

    return h('div', { class: 'panel-modal', id: 'kt-import', hidden: true }, [
      h('div', { class: 'panel-inner' }, [
        h('header', { class: 'panel-head' }, [
          h('h2', { text: 'Каркас посмертия' }),
          h('button', { class: 'btn ghost small', type: 'button', text: 'Закрыть', onclick: function () { closePanel('kt-import'); } })
        ]),
        h('p', { class: 'muted small', text: 'Вставь JSON каркаса, загрузи готовый или скачай текущий. Покупки по совпадающим названиям сохранятся.' }),
        ownRow,
        h('div', { class: 'row gap wrap' }, [
          select,
          h('button', {
            class: 'btn', type: 'button', text: 'Загрузить в план', onclick: function () {
              if (applySkeletonJson(area.value, true)) {
                markDirty();
                closePanel('kt-import');
                render();
              }
            }
          }),
          h('button', {
            class: 'btn ghost', type: 'button', text: 'Загрузить и очистить покупки', onclick: function () {
              if (confirm('Загрузить каркас и сбросить выбранные покупки?') && applySkeletonJson(area.value, false)) {
                markDirty();
                closePanel('kt-import');
                render();
              }
            }
          }),
          h('button', {
            class: 'btn ghost', type: 'button', text: 'Скачать JSON', onclick: function () {
              download('skeleton-' + slug(plan.skeleton || plan.title) + '.json', skeletonJson(), 'application/json');
            }
          }),
          h('button', {
            class: 'btn ghost', type: 'button', text: 'Шаблон пустого каркаса', onclick: function () {
              area.value = skeletonJson() || '';
              area.dataset.touched = '1';
            }
          })
        ]),
        area,
        h('details', { class: 'muted small' }, [
          h('summary', { text: 'Формат каркаса' }),
          h('pre', {
            class: 'mono small', text: JSON.stringify({
              name: 'Жизнь 7: континент Заскорузлых пирамид',
              balance: 1605.24,
              earned: 1596.67,
              multipliers: ['Женский пол: ×1,5'],
              shop: [
                { key: 'lesson:ping_sistem', kind: 'lesson', section: 'lessons', title: 'пинг систем', min_lesson: 10, cost: 0, fixed: true, tags: ['one_chance'], desc: '…' },
                { key: 'lesson:uchene_svet', kind: 'lesson', section: 'lessons', title: 'ученье — свет!', min_lesson: 4, desc: '…' }
              ],
              ladders: [{ key: 'talent.die', section: 'talent', title: 'кость таланта', unit: 'd', from: 1, to: 22, step: 1, value: 1, cost: { type: 'to_value', rate: 1 } }],
              accumulators: [{ key: 'acc:gorozhane', name: 'горожане', charges: 0.8, max: 1, rate: 20 }],
              folios: [{ key: 'folio:ognennyy_dozhd', name: 'метод Огненного Дождя', take: false, cost: '20 XP' }]
            }, null, 2)
          })
        ])
      ])
    ]);
  }

  function renderExportModal() {
    var area = h('textarea', { id: 'kt-export-text', class: 'mono export-text', rows: 24, readonly: true });
    var warn = h('div', { class: 'export-warn', id: 'kt-export-warn', hidden: true });
    return h('div', { class: 'panel-modal', id: 'kt-export', hidden: true }, [
      h('div', { class: 'panel-inner' }, [
        h('header', { class: 'panel-head' }, [
          h('h2', { text: 'Экспорт плана' }),
          h('button', { class: 'btn ghost small', type: 'button', text: 'Закрыть', onclick: function () { closePanel('kt-export'); } })
        ]),
        warn,
        h('div', { class: 'row gap wrap' }, [
          h('button', {
            class: 'btn', type: 'button', text: 'Скопировать', onclick: function (e) {
              area.select();
              if (navigator.clipboard) navigator.clipboard.writeText(area.value);
              else document.execCommand('copy');
              e.target.textContent = 'Скопировано';
              setTimeout(function () { e.target.textContent = 'Скопировать'; }, 1500);
            }
          }),
          h('button', {
            class: 'btn ghost', type: 'button', text: 'Скачать .txt', onclick: function () {
              download('plan-' + slug(plan.title) + '.txt', area.value);
            }
          }),
          h('a', { class: 'btn ghost', href: '/plans/' + plan.id + '/export', text: 'Страница экспорта' })
        ]),
        area
      ])
    ]);
  }

  function openImport() {
    var panel = document.getElementById('kt-import');
    var area = document.getElementById('kt-skeleton-text');
    if (area) { area.value = skeletonJson(); area.dataset.touched = ''; }
    if (panel) panel.hidden = false;
  }

  function openExport() {
    recompute();
    var panel = document.getElementById('kt-export');
    if (panel) panel.hidden = false;
  }

  function closePanel(id) {
    var el = document.getElementById(id);
    if (el) el.hidden = true;
  }

  function save() {
    // Порядок уроков не трогаем: как игрок расставил, так и сохраняем.
    var state = {
      title: plan.title, emoji: plan.emoji, balance: plan.balance, notes: plan.notes || '',
      decimals: !!(plan.options && plan.options.decimals), skeleton: plan.skeleton || '',
      earned: plan.earned || 0, multipliers: plan.multipliers || [], source: plan.source || '',
      shop: plan.shop, picks: plan.picks, deferred: plan.deferred, not_taken: plan.not_taken,
      ladders: plan.ladders, accumulators: plan.accumulators, folios: plan.folios,
      requirements: plan.requirements || [], deferred_slots: plan.deferred_slots || 1
    };
    document.getElementById('kt-state').value = JSON.stringify(state);
    dirty = false;
    document.getElementById('kt-save').submit();
  }

  function parseNum(v) {
    var n = parseFloat(String(v).replace(',', '.'));
    return isNaN(n) ? 0 : n;
  }

  /* hasMinViolation — есть ли урок, стоящий раньше своего «минимум n-ным». */
  function hasMinViolation() {
    var n = 0;
    for (var i = 0; i < plan.picks.length; i++) {
      var it = item(plan.picks[i].key);
      if (!it || it.kind !== 'lesson' || it.fixed || isDeferred(it.key)) continue;
      n++;
      if ((it.min_lesson || 1) > n) return true;
    }
    return false;
  }

  // Поиск по магазину.
  function applyFilter(value) {
    filter = String(value || '').toLowerCase();
    Array.prototype.forEach.call(document.querySelectorAll('.item'), function (row) {
      var title = row.querySelector('.item-title').textContent.toLowerCase();
      row.hidden = filter.length > 0 && title.indexOf(filter) < 0;
    });
  }

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') { closePanel('kt-export'); closePanel('kt-import'); }
    if ((e.ctrlKey || e.metaKey) && e.key === 's') { e.preventDefault(); save(); }
  });

  window.addEventListener('beforeunload', function (e) {
    if (dirty) { e.preventDefault(); e.returnValue = ''; }
  });

  var search = document.getElementById('kt-search');
  if (search) search.addEventListener('input', function (e) { applyFilter(e.target.value); });

  // Порядок уроков builder больше не меняет сам: он двигает его только
  // по кнопке «Разложить уроки по требованиям» или «поставить после …».
  render();
})();
