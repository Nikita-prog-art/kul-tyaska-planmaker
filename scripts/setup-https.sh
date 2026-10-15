#!/usr/bin/env bash
# Бесплатный постоянный адрес: nip.io отдаёт домен, указывающий на IP сервера,
# а Let's Encrypt выдаёт под него настоящий сертификат. Аккаунт Cloudflare для
# этого не нужен — в отличие от публичного хоста туннеля.
#
#   ./scripts/setup-https.sh 162.254.85.195.nip.io
set -euo pipefail

cd "$(dirname "$0")/.."
HOST="${DEPLOY_HOST:-nikita@162.254.85.195}"
DOMAIN="${1:-162.254.85.195.nip.io}"
SSH_OPTS=(-F /dev/null -o BatchMode=yes -o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=4)

retry() {
  local n=0
  until "$@"; do
    n=$((n + 1))
    if [ "$n" -ge 6 ]; then
      echo "не получилось после $n попыток: $*" >&2
      return 1
    fi
    echo "попытка $n не удалась, повтор через $((n * 3))с" >&2
    sleep $((n * 3))
  done
}
rssh() { retry ssh "${SSH_OPTS[@]}" "$HOST" "$@"; }
rscp() { retry scp "${SSH_OPTS[@]}" "$@"; }

echo "== домен $DOMAIN =="
rssh "getent hosts $DOMAIN >/dev/null || { echo 'домен не резолвится в этот сервер'; exit 1; }"

echo "== ставим конфиг nginx =="
rscp deploy/nginx-kul-tyaska.conf "$HOST:/tmp/kul-tyaska.nginx"
rssh "sudo sed -i 's/server_name .*/server_name $DOMAIN;/' /tmp/kul-tyaska.nginx &&
      sudo install -m 0644 /tmp/kul-tyaska.nginx /etc/nginx/sites-available/kul-tyaska &&
      sudo ln -sfn /etc/nginx/sites-available/kul-tyaska /etc/nginx/sites-enabled/kul-tyaska &&
      sudo /usr/sbin/nginx -t &&
      sudo systemctl reload nginx &&
      echo 'nginx перечитан'"

echo "== сертификат Let's Encrypt =="
rssh "sudo certbot --nginx -d $DOMAIN --non-interactive --agree-tos \
        --register-unsafely-without-email --redirect --keep-until-expiring 2>&1 | tail -12"

echo "== проверка =="
rssh "sudo /usr/sbin/nginx -t &&
      systemctl is-active nginx &&
      curl -sS -o /dev/null -w 'https://$DOMAIN/ → %{http_code}\n' --max-time 25 https://$DOMAIN/ &&
      curl -sS -o /dev/null -w 'https://$DOMAIN/healthz → %{http_code}\n' --max-time 25 https://$DOMAIN/healthz"

echo
echo "Готово: https://$DOMAIN"
echo "Продление сертификата — таймером certbot (systemctl list-timers | grep certbot)."
