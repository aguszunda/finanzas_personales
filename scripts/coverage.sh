#!/usr/bin/env bash
# Medición de cobertura del proyecto.
#
# Corre la batería de tests con cobertura nativa de Go y genera:
#   coverage.filtered.out  -> reporte textual detalle por función (filtrado)
#   coverage.html          -> reporte HTML navegable
# Falla (exit 1) si la cobertura total de statements queda por debajo de
# MIN_COVERAGE (por defecto 85), y además valida el desglose por paquete.
#
# Uso:
#   ./scripts/coverage.sh              # umbral por defecto 85 %
#   MIN_COVERAGE=90 ./scripts/coverage.sh
#   GOTEST_FLAGS="-race" ./scripts/coverage.sh
#
# Nota: los reportes se escriben en la raíz del repo (igual que espera la CI en
# el artifact de GitHub Actions).
set -euo pipefail

MIN_COVERAGE="${MIN_COVERAGE:-85}"
GOTEST_FLAGS="${GOTEST_FLAGS:-}"

profile="$(mktemp)"
trap 'rm -f "$profile"' EXIT

# Coverear todos los packages del proyecto menos el wrapper principal de la
# app (cmd/server solo llama a main(); no aporta lógica medible y ensucia la
# métrica), igual que hace `go test ./...` con los tests.
module="$(go list -m)"
pkgs="$(go list ./... | grep -vx "$module" | tr '\n' ' ')"

echo "==> go test ${GOTEST_FLAGS:-} -covermode=atomic (min coverage: ${MIN_COVERAGE} %)"
# GOTEST_FLAGS y pkgs se expanden a propósito para permitir banderas extra y
# selección de packages.
# shellcheck disable=SC2086
go test ${GOTEST_FLAGS:-} -covermode=atomic -coverprofile="$profile" $pkgs >/dev/null

echo "==> generando coverage.filtered.out y coverage.html"
go tool cover -func="$profile" | grep -v '^total:' > coverage.filtered.out
go tool cover -html="$profile" -o coverage.html

total="$(go tool cover -func="$profile" | awk '/^total:/ {gsub("%","",$3); print $3}')"
echo "==> cobertura total: ${total} %"

if awk -v t="$total" -v m="$MIN_COVERAGE" 'BEGIN { exit !(t+0 < m+0) }'; then
	echo "FAIL: cobertura total ${total} % está por debajo del umbral ${MIN_COVERAGE} %" >&2
	exit 1
fi
echo "OK: cobertura total ${total} % supera el umbral ${MIN_COVERAGE} %"