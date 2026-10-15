#!/usr/bin/env python3
"""Удаляет из базы тестовые аккаунты, созданные дымовой проверкой (nick начинается с smoke).

  sudo python3 prune-smoke.py /var/lib/kultyaska/kultyaska.json

Перед изменением делает резервную копию рядом с базой.
"""
import json
import os
import shutil
import sys
import time


def main() -> int:
    path = sys.argv[1] if len(sys.argv) > 1 else "/var/lib/kultyaska/kultyaska.json"
    with open(path, encoding="utf-8") as fh:
        db = json.load(fh)

    users = db.get("users") or []
    doomed = {u["id"] for u in users if str(u.get("nick", "")).lower().startswith("smoke")}
    kept_users = [u for u in users if u["id"] not in doomed]

    plans = db.get("plans") or {}
    doomed_plans = [pid for pid, p in plans.items() if p.get("user_id") in doomed]
    for pid in doomed_plans:
        del plans[pid]

    sessions = db.get("sessions") or {}
    for token in [t for t, s in sessions.items() if s.get("user_id") in doomed]:
        del sessions[token]

    if not doomed:
        print("тестовых аккаунтов нет — база не тронута")
        return 0

    backup = f"{path}.bak-{time.strftime('%Y%m%d-%H%M%S')}"
    shutil.copy2(path, backup)
    db["users"] = kept_users
    db["plans"] = plans
    db["sessions"] = sessions
    # Скрипт обычно запускают через sudo, поэтому сохраняем прежние владельца и права:
    # приложение работает под своим пользователем и иначе не прочитает базу.
    st = os.stat(path)
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(db, fh, ensure_ascii=False, indent=2)
    os.chmod(tmp, st.st_mode & 0o777)
    os.chown(tmp, st.st_uid, st.st_gid)
    os.replace(tmp, path)
    os.chown(backup, st.st_uid, st.st_gid)
    os.chmod(backup, st.st_mode & 0o777)

    print(f"удалено аккаунтов: {len(doomed)}, планов: {len(doomed_plans)}; копия: {backup}")
    print("остались:")
    for u in kept_users:
        n = sum(1 for p in plans.values() if p.get("user_id") == u["id"])
        print(f"  - {u.get('nick')} (планов: {n})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
