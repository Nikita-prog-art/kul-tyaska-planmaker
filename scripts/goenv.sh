# Локальные кэши Go: домашний каталог смонтирован read-only,
# поэтому GOPATH/GOMODCACHE/GOCACHE живут внутри проекта (см. .gitignore).
# Использование: source scripts/goenv.sh
export GOPATH="${PWD}/.cache/gopath"
export GOMODCACHE="${PWD}/.cache/gomod"
export GOCACHE="${PWD}/.cache/gobuild"
export GOFLAGS="-mod=mod"
