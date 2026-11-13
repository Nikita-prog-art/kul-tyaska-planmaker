#!/usr/bin/env python3
"""Удаляет из базы тестовые аккаунты, созданные дымовой проверкой (nick начинается с smoke).

  sudo python3 prune-smoke.py /var/lib/kultyaska/kultyaska.json [--dry-run]

Перед изменением делает резервную копию рядом с базой. Печатает, какие аккаунты
и планы удаляются, чтобы было видно, если под фильтр попал не тот ник.
"""
import argparse
import json
import os
import shutil
import time


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("db", nargs="?", default="/var/lib/kultyaska/kultyaska.json")
    ap.add_argument("--dry-run", action="store_true", help="только показать, что будет удалено")
    args = ap.parse_args()

    with open(args.db, encoding="utf-8") as fh:
        db = json.load(fh)

    users = db.get("users") or []
    plans = db.get("plans") or {}
    doomed_users = [u for u in users if str(u.get("nick", "")).lower().startswith("smoke")]
    doomed = {u.get("id") for u in doomed_users if u.get("id")}
    kept_users = [u for u in users if u.get("id") not in doomed]

    def plans_of(uid):
        return [pid for pid, p in plans.items() if p.get("user_id") == uid]

    doomed_plans = []
    plan_counts = {}
    print("К удалению:")
    for u in doomed_users:
        pids = plans_of(u.get("id"))
        plan_counts[u.get("id")] = len(pids)
        doomed_plans.extend(pids)
        print("  - %s (id %s, планов: %d)" % (u.get("nick"), u.get("id"), len(pids)))
    if not doomed_users:
        print("  — тестовых аккаунтов нет")
    if args.dry_run:
        print("dry-run: база не тронута")
        return 0
    if not doomed_users:
        print("тестовых аккаунтов нет — база не тронута")
        return 0

    for pid in doomed_plans:
        del plans[pid]

    sessions = db.get("sessions") or {}
    for token in [t for t, s in sessions.items() if s.get("user_id") in doomed]:
        del sessions[token]

    backup = f"{args.db}.bak-{time.strftime('%Y%m%d-%H%M%S')}"
    shutil.copy2(args.db, backup)
    db["users"] = kept_users
    db["plans"] = plans
    db["sessions"] = sessions
    # Скрипт обычно запускают через sudo, поэтому сохраняем прежние владельца и права:
    # приложение работает под своим пользователем и иначе не прочитает базу.
    st = os.stat(args.db)
    tmp = args.db + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(db, fh, ensure_ascii=False, indent=2)
    os.chmod(tmp, st.st_mode & 0o777)
    os.chown(tmp, st.st_uid, st.st_gid)
    os.replace(tmp, args.db)
    os.chown(backup, st.st_uid, st.st_gid)
    os.chmod(backup, st.st_mode & 0o777)

    print("Удалены аккаунты:")
    for u in doomed_users:
        print("  - %s (id %s, планов: %d)" % (u.get("nick"), u.get("id"), plan_counts.get(u.get("id"), 0)))
    print(f"удалено аккаунтов: {len(doomed_users)}, планов: {len(doomed_plans)}; копия: {backup}")
    print("остались:")
    for u in kept_users:
        print(f"  - {u.get('nick')} (планов: {len(plans_of(u.get('id')))})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
