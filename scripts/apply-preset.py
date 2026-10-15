#!/usr/bin/env python3
"""Переносит новый каркас в уже сохранённые планы.

Магазин, шкалы, накопители, фолианты и условия берутся из каркаса,
а покупки, баланс, заметки и название плана остаются на месте.
Ключи позиций, которых нет в новом каркасе, отбрасываются; совпавшие
по названию — переносятся на новый ключ.

  sudo python3 apply-preset.py /var/lib/kultyaska/kultyaska.json \\
       internal/web/presets/08-zaskoruzlye-piramidy.json \\
       [--plan ID] [--rename-old "Жизнь 6"] [--rename-new "Жизнь 8"]

Перед изменением делает резервную копию рядом с базой.
"""
import argparse
import json
import os
import shutil
import sys
import time


def load(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("db")
    ap.add_argument("preset")
    ap.add_argument("--plan", default="")
    ap.add_argument("--rename-old", default="")
    ap.add_argument("--rename-new", default="")
    args = ap.parse_args()

    db = load(args.db)
    sk = load(args.preset)
    shop = sk.get("shop") or []
    by_key = {it["key"]: it for it in shop}
    by_title = {(it.get("title") or "").lower(): it["key"] for it in shop}

    plans = db.get("plans") or {}
    targets = [args.plan] if args.plan else list(plans)
    changed = 0
    for pid in targets:
        p = plans.get(pid)
        if not p:
            continue

        def remap(keys):
            out = []
            for k in keys or []:
                if k in by_key:
                    out.append(k)
                    continue
                # ключ мог измениться вместе с названием позиции
                old = next((it for it in (p.get("shop") or []) if it.get("key") == k), None)
                if old:
                    nk = by_title.get((old.get("title") or "").lower())
                    if nk:
                        out.append(nk)
            return out

        p["picks"] = [{"key": k, "qty": 1} for k in remap([pk.get("key") for pk in (p.get("picks") or [])])]
        p["deferred"] = []
        p["not_taken"] = remap(p.get("not_taken") or [])
        p["shop"] = shop
        p["ladders"] = sk.get("ladders") or []
        p["accumulators"] = sk.get("accumulators") or []
        p["folios"] = sk.get("folios") or []
        p["requirements"] = sk.get("requirements") or []
        p["skeleton"] = sk.get("name") or p.get("skeleton")
        if args.rename_old and args.rename_new:
            for field in ("title", "skeleton"):
                if args.rename_old in (p.get(field) or ""):
                    p[field] = (p[field] or "").replace(args.rename_old, args.rename_new)
        elif not p.get("title"):
            p["title"] = p["skeleton"]
        changed += 1
        print("обновлён план: %s (%s)" % (p.get("title"), pid))

    if not changed:
        print("нечего обновлять")
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
