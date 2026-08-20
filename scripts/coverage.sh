#!/usr/bin/env bash
#
# Quality gate de cobertura de código.
#
# Corre los tests (unitarios + integración, si hay MySQL disponible) con
# `-coverpkg=./...` para que la cobertura sea cross-package (los tests de
# internal/service ejercitan internal/repository a través de sqlmock), filtra
# los archivos que no pueden testearse (ver scripts/coverage_ignore.txt) y
# falla con código de salida != 0 si el porcentaje queda por debajo de
# MIN_COVERAGE.
#
# Uso:
#   MIN_COVERAGE=85 ./scripts/coverage.sh
#
# Variables:
#   MIN_COVERAGE      Umbral mínimo en porcentaje (default: 85).
#   COVERAGE_OUT      Ruta del profile de salida (default: coverage.out).
#
set -euo pipefail

MIN_COVERAGE="${MIN_COVERAGE:-85.0}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERAGE_OUT="${COVERAGE_OUT:-$ROOT_DIR/coverage.out}"
COVERAGE_FILTERED="${COVERAGE_OUT%.out}.filtered.out"
COVERAGE_HTML="${COVERAGE_OUT%.out}.html"
IGNORE_FILE="${COVERAGE_IGNORE_FILE:-$ROOT_DIR/scripts/coverage_ignore.txt}"

cd "$ROOT_DIR"

echo "==> Corriendo tests con cobertura cross-package (coverpkg=./...)..."

# >/dev/null para no inundar el log; el detalle va al profile.
go test ./... -coverpkg=./... -coverprofile="$COVERAGE_OUT" >/dev/null

echo "==> Filtrando archivos del ignore ($IGNORE_FILE)..."

# Arma un grep -E con los patrones del ignore (uno por línea, `#` = comentario).
# Los paths del profile llevan el prefijo del módulo (`optipay/...`),
# así que un patrón anclado con `^` se reescribe a `(^|/)` para que siga
# matcheando tras el prefijo.
grep_args=()
while IFS= read -r line || [[ -n "$line" ]]; do
  pat="${line%%#*}"            # quita comentarios
  pat="${pat#"${pat%%[![:space:]]*}"}"  # trim inicial
  pat="${pat%"${pat##*[![:space:]]}"}"  # trim final
  [[ -z "$pat" ]] && continue
  if [[ "$pat" == ^* ]]; then
    pat="(^|/)${pat#^}"
  fi
  grep_args+=(-e "$pat")
done < "$IGNORE_FILE"

# Conserva el header `mode:` y elimina los bloques de archivos ignorados.
grep -Ev "${grep_args[@]}" "$COVERAGE_OUT" > "$COVERAGE_FILTERED"

echo "==> Calculando cobertura filtrada..."

# La última línea de `go tool cover -func` es el total general: "... 69.8%".
total_line="$(go tool cover -func="$COVERAGE_FILTERED" | tail -1)"
COVERAGE_PCT="$(printf '%s' "$total_line" | awk '{print $NF}')"
COVERAGE_PCT="${COVERAGE_PCT%\%}"

echo "==> Cobertura: ${COVERAGE_PCT}% (mínimo exigido: ${MIN_COVERAGE}%)"
echo "    $total_line"

if awk -v p="$COVERAGE_PCT" -v m="$MIN_COVERAGE" 'BEGIN { exit !(p + 0 < m + 0) }'; then
  echo "ERROR: la cobertura (${COVERAGE_PCT}%) está por debajo del mínimo exigido (${MIN_COVERAGE}%)." >&2
  echo "       Revisá coverage.filtered.out para ver los bloques descubiertos." >&2
  exit 1
fi

echo "==> Generando reporte HTML: $COVERAGE_HTML"
go tool cover -html="$COVERAGE_FILTERED" -o "$COVERAGE_HTML"

echo "==> OK: cobertura por encima del mínimo."
