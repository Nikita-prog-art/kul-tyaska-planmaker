#!/usr/bin/env bash
# Все проверки проекта: Go, JS-движок, конструктор в jsdom.
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck disable=SC1091
source scripts/goenv.sh

echo "== go vet =="
go vet ./...

echo "== go test =="
go test ./...

echo "== JS-движок против эталона Go =="
node tests/engine.test.js

echo "== конструктор в jsdom =="
node tests/builder.test.js

echo "== переключатель темы =="
node tests/theme.test.js

echo
echo "Всё зелёное."
