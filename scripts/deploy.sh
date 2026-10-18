#!/usr/bin/env bash
# Сборка и выкладка культяшки на сервер.
#
#   ./scripts/deploy.sh            # собрать и выложить
#   ./scripts/deploy.sh --no-build # выложить уже собранный бинарник
#
# Переменные: DEPLOY_HOST (по умолчанию nikita@162.254.85.195), KT_SSH_PROXY —
# локальный прокси для ssh, если провайдер режет 22-й порт
# (например, KT_SSH_PROXY=socks5://127.0.0.1:10808).
# Токен туннеля берётся из .env (CF_TUNNEL_TOKEN) и уезжает в /etc/kul-tyaska/tunnel.token.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$PWD"
HOST="${DEPLOY_HOST:-nikita@162.254.85.195}"
SSH_OPTS=(-F /dev/null -o BatchMode=yes -o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=4)
if [ -n "${KT_SSH_PROXY:-}" ]; then
  SSH_OPTS+=(-o "ProxyCommand=python3 $ROOT/scripts/ssh-relay.py %h %p")
fi
BUILD=1
[ "${1:-}" = "--no-build" ] && BUILD=0

retry() {
  local n=0
  until "$@"; do
    n=$((n + 1))
    if [ "$n" -ge 6 ]; then
      echo "не получилось после $n попыток: $*" >&2
      return 1
    fi
    echo "попытка $n не удалась, повтор через $((n * 3))с: $*" >&2
    sleep $((n * 3))
  done
}

rssh() { retry ssh "${SSH_OPTS[@]}" "$HOST" "$@"; }
rscp() { retry scp "${SSH_OPTS[@]}" "$@"; }

BIN="$ROOT/.cache/bin/kultyaska"

if [ "$BUILD" = 1 ]; then
  echo "== сборка статического бинарника =="
  # shellcheck disable=SC1091
  source "$ROOT/scripts/goenv.sh"
  mkdir -p "$ROOT/.cache/bin"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$BIN" .
  ls -la "$BIN"
fi

echo "== подготовка каталогов и пользователя на $HOST =="
rssh 'sudo mkdir -p /opt/kul-tyaska /etc/kul-tyaska /var/lib/kultyaska &&
      id -u kultyaska >/dev/null 2>&1 || sudo useradd --system --home /var/lib/kultyaska --shell /usr/sbin/nologin kultyaska;
      sudo chown -R kultyaska:kultyaska /var/lib/kultyaska &&
      sudo chmod 750 /var/lib/kultyaska /etc/kul-tyaska'

echo "== токен туннеля =="
# Копируем через файл, а не через stdin: rssh умеет повторять запрос,
# а повтор с уже вычитанным stdin записал бы пустой токен.
if [ -f "$ROOT/.env" ]; then
  TOKEN_FILE="$ROOT/.cache/tunnel.token"
  mkdir -p "$ROOT/.cache"
  umask 077
  sed -n 's/^CF_TUNNEL_TOKEN=//p' "$ROOT/.env" | tr -d '\r\n' > "$TOKEN_FILE"
  if [ ! -s "$TOKEN_FILE" ]; then
    echo "в .env нет CF_TUNNEL_TOKEN" >&2
    exit 1
  fi
  rscp "$TOKEN_FILE" "$HOST:/tmp/kt.token"
  rm -f "$TOKEN_FILE"
  rssh 'sudo install -m 0600 -o root -g root /tmp/kt.token /etc/kul-tyaska/tunnel.token &&
        sudo rm -f /tmp/kt.token &&
        printf "байт в токене: " && sudo wc -c /etc/kul-tyaska/tunnel.token'
else
  echo "нет .env с CF_TUNNEL_TOKEN — туннель останется без токена" >&2
fi

echo "== выкладка бинарника и юнитов =="
rscp "$BIN" "$HOST:/tmp/kultyaska.new"
rscp "$ROOT/deploy/kul-tyaska.service" "$HOST:/tmp/kul-tyaska.service"
rscp "$ROOT/deploy/kul-tyaska-tunnel.service" "$HOST:/tmp/kul-tyaska-tunnel.service"
rssh 'sudo install -m 0755 /tmp/kultyaska.new /opt/kul-tyaska/kultyaska &&
      rm -f /tmp/kultyaska.new &&
      sudo install -m 0644 /tmp/kul-tyaska.service /etc/systemd/system/kul-tyaska.service &&
      sudo install -m 0644 /tmp/kul-tyaska-tunnel.service /etc/systemd/system/kul-tyaska-tunnel.service &&
      rm -f /tmp/kul-tyaska.service /tmp/kul-tyaska-tunnel.service &&
      sudo systemctl daemon-reload'

echo "== запуск сервисов =="
rssh 'sudo pkill -x cloudflared 2>/dev/null || true;
      sudo systemctl enable --now kul-tyaska.service >/dev/null 2>&1;
      sudo systemctl restart kul-tyaska.service;
      sleep 2;
      sudo systemctl enable --now kul-tyaska-tunnel.service >/dev/null 2>&1;
      sudo systemctl restart kul-tyaska-tunnel.service;
      sleep 4;
      echo "--- культяшка ---"; sudo systemctl is-active kul-tyaska.service;
      echo "--- туннель ---"; sudo systemctl is-active kul-tyaska-tunnel.service;
      echo "--- проверка приложения ---"; curl -sS -o /dev/null -w "healthz: %{http_code}\n" http://127.0.0.1:8080/healthz;
      echo "--- журнал приложения ---"; sudo journalctl -u kul-tyaska.service -n 5 --no-pager;
      echo "--- журнал туннеля ---"; sudo journalctl -u kul-tyaska-tunnel.service -n 6 --no-pager | tail -6'

echo
echo "Готово. Приложение слушает 127.0.0.1:8080 на $HOST, туннель поднят."
