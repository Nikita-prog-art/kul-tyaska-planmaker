#!/usr/bin/env python3
"""Переносит новый каркас в уже сохранённые планы.

Магазин, шкалы, накопители, фолианты и условия берутся из каркаса,
а покупки, баланс, заметки и название плана остаются на месте.
Ключи позиций, которых нет в новом каркасе, отбрасываются; совпавшие
по названию — переносятся на новый ключ. Отметки «активирую» и «беру»,
а также выбранные значения шкал переносятся по ключу: миграция не должна
молча сбрасывать выбор игрока.

  sudo python3 apply-preset.py /var/lib/kultyaska/kultyaska.json \\
       internal/web/presets/09-kontinent-iskonnogo-balansa.json \\
       [--plan ID] [--only-skeleton "Жизнь 9"] [--dry-run] \\
       [--rename-old "Жизнь 6"] [--rename-new "Жизнь 8"]

Перед изменением делает резервную копию рядом с базой.
"""
import argparse
import copy
import datetime
import json
import os
import shutil
import time


def load(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def old_by_key(items):
    """Индекс старых позиций по ключу и по названию (для переноса отметок)."""
    by_key = {}
    for it in items or []:
        if it.get("key"):
            by_key[it["key"]] = it
        if it.get("name"):
            by_key.setdefault(it["name"], it)
    return by_key


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("db")
    ap.add_argument("preset")
    ap.add_argument("--plan", default="")
    ap.add_argument("--only-skeleton", default="")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--rename-old", default="")
    ap.add_argument("--rename-new", default="")
    args = ap.parse_args()

    db = load(args.db)
    sk = load(args.preset)
    shop = copy.deepcopy(sk.get("shop") or [])
    by_key = {it["key"]: it for it in shop}
    by_title = {(it.get("title") or "").lower(): it["key"] for it in shop}

    plans = db.get("plans") or {}
    targets = [args.plan] if args.plan else list(plans)
    changed = 0
    for pid in targets:
        p = plans.get(pid)
        if not p:
            continue
        if args.only_skeleton and args.only_skeleton.lower() not in (p.get("skeleton") or "").lower():
            continue

        old_shop = {it.get("key"): it for it in (p.get("shop") or [])}

        def remap_one(k):
            if not k:
                return None
            if k in by_key:
                return k
            # ключ мог измениться вместе с названием позиции
            old = old_shop.get(k)
            if old:
                return by_title.get((old.get("title") or "").lower())
            return None

        lost = []

        def remap(keys, report_lost):
            out = []
            for k in keys or []:
                nk = remap_one(k)
                if nk:
                    out.append(nk)
                elif report_lost:
                    old = old_shop.get(k)
                    lost.append((old or {}).get("title") or k)
            return out

        picks = []
        for pk in p.get("picks") or []:
            nk = remap_one(pk.get("key"))
            if not nk:
                old = old_shop.get(pk.get("key"))
                lost.append((old or {}).get("title") or pk.get("key"))
                continue
            qty = pk.get("qty") or 1
            limit = by_key[nk].get("repeat") or 0
            if limit and qty > limit:
                qty = limit
            picks.append({"key": nk, "qty": qty})
        p["picks"] = picks

        deferred = []
        for d in p.get("deferred") or []:
            nk = remap_one(d.get("key"))
            if nk:
                deferred.append({"key": nk, "slot": len(deferred) + 1})
            else:
                old = old_shop.get(d.get("key"))
                lost.append((old or {}).get("title") or d.get("key"))
        p["deferred"] = deferred
        p["not_taken"] = remap(p.get("not_taken") or [], False)

        # Отметки накопителей и фолиантов переносим по ключу (или названию):
        # перезагрузка каркаса не должна стирать их молча.
        old_accs = old_by_key(p.get("accumulators"))
        new_accs = copy.deepcopy(sk.get("accumulators") or [])
        for a in new_accs:
            old = old_accs.get(a.get("key")) or old_accs.get(a.get("name"))
            if old and old.get("activate"):
                a["activate"] = True
        old_folios = old_by_key(p.get("folios"))
        new_folios = copy.deepcopy(sk.get("folios") or [])
        for f in new_folios:
            old = old_folios.get(f.get("key")) or old_folios.get(f.get("name"))
            if old and old.get("take"):
                f["take"] = True

        # Значения шкал — выбор игрока. Границы и цены берём из каркаса,
        # а «уже есть / куплено до» сохраняем, если ключ тот же.
        old_ladders = {l.get("key"): l for l in (p.get("ladders") or [])}
        new_ladders = copy.deepcopy(sk.get("ladders") or [])
        for l in new_ladders:
            old = old_ladders.get(l.get("key"))
            value = old.get("value") if old else None
            if isinstance(value, (int, float)):
                lo, hi = l.get("from"), l.get("to")
                if isinstance(lo, (int, float)) and value < lo:
                    value = lo
                if isinstance(hi, (int, float)) and value > hi:
                    value = hi
                l["value"] = value

        p["shop"] = shop
        if "ladders" in sk:
            p["ladders"] = new_ladders
        if "accumulators" in sk:
            p["accumulators"] = new_accs
        if "folios" in sk:
            p["folios"] = new_folios
        if "requirements" in sk:
            p["requirements"] = copy.deepcopy(sk.get("requirements") or [])
        p["skeleton"] = sk.get("name") or p.get("skeleton")
        if "deferred_slots" in sk:
            p["deferred_slots"] = max(1, int(sk.get("deferred_slots") or 1))
        if sk.get("notes"):
            p["notes"] = sk["notes"]
        for field in ("earned", "subtotal", "multipliers"):
            if field in sk:
                p[field] = copy.deepcopy(sk.get(field))
        p["updated_at"] = datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z")

        if args.rename_old and args.rename_new:
            for field in ("title", "skeleton"):
                if args.rename_old in (p.get(field) or ""):
                    p[field] = (p[field] or "").replace(args.rename_old, args.rename_new)
        elif not p.get("title"):
            p["title"] = p["skeleton"]
        changed += 1
        note = "; потеряны покупки: " + ", ".join(lost) if lost else ""
        print("обновлён план: %s (%s)%s" % (p.get("title"), pid, note))

    if not changed:
        print("нечего обновлять")
        return 0
    if args.dry_run:
        print("dry-run: изменений %d, база не тронута" % changed)
        return 0

    st = os.stat(args.db)
    backup = "%s.bak-%s" % (args.db, time.strftime("%Y%m%d-%H%M%S"))
    shutil.copy2(args.db, backup)
    tmp = args.db + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(db, fh, ensure_ascii=False, indent=2)
    # Скрипт запускают через sudo: сохраняем прежние владельца и права,
    # иначе приложение не прочитает базу.
    os.chmod(tmp, st.st_mode & 0o777)
    os.chown(tmp, st.st_uid, st.st_gid)
    os.replace(tmp, args.db)
    os.chown(backup, st.st_uid, st.st_gid)
    os.chmod(backup, st.st_mode & 0o777)
    print("обновлено планов: %d; копия базы: %s" % (changed, backup))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
