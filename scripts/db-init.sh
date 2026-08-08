#!/usr/bin/env bash
#
# Inicializa la base de datos para desarrollo:
#   1. Crea la base (si no existe) con utf8mb4 / utf8mb4_unicode_ci.
#   2. Aplica las migraciones desde migrations/*.up.sql con golang-migrate.
#
# La app NO migra al arrancar (no hay SQL embebido en main.go); este script
# (o `make migrate-up`) es el único camino para crear/esquematizar la base.
#
# Uso:
#   ./scripts/db-init.sh                 # usa DATABASE_URL de env.secrets
#   DATABASE_URL="..." ./scripts/db-init.sh
#   MYSQL_USER=... MYSQL_PASSWORD=... MYSQL_DATABASE=... ./scripts/db-init.sh
#
# Variables:
#   DATABASE_URL    DSN de MySQL (local). Si no se definen MYSQL_*, se parsea de
#                   esta variable (formato user:pass@tcp(host:port)/db?params).
#   MYSQL_HOST      Host (default: 127.0.0.1).     MYSQL_PORT  Puerto (default: 3306).
#   MYSQL_USER      Usuario para crear la base (default: el de DATABASE_URL).
#   MYSQL_PASSWORD  Password (default: el de DATABASE_URL).
#   MYSQL_DATABASE  Nombre de la base (default: el de DATABASE_URL).
#   MIGRATE_VERSION Versión fija de golang-migrate (default: v4.18.3).
#
# Protecciones:
#   - Idempotente: CREATE DATABASE IF NOT EXISTS + versionado golang-migrate.
#   - golang-migrate pinneado (nunca @latest), con tracking de versión/dirty.
#   - La password se pasa por MYSQL_PWD (no visible en `ps`), nunca por CLI.
#   - El nombre de la base se valida antes de interpolar en el SQL.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/env.secrets"

# Cargar env.secrets si existe. Se usa `set -a; source` (NO `export $(cat env.secrets)`)
# porque el DSN contiene `&` que el shell interpretaría como background.
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "$ENV_FILE"
  set +a
fi

# --- Conexión: o bien se dan MYSQL_*, o bien se parsea DATABASE_URL ---------
USE_MYSQL_VARS=0
for v in MYSQL_HOST MYSQL_PORT MYSQL_USER MYSQL_PASSWORD MYSQL_DATABASE; do
  [[ -n "${!v:-}" ]] && USE_MYSQL_VARS=1
done

if [[ "$USE_MYSQL_VARS" == "1" ]]; then
  DB_HOST="${MYSQL_HOST:-127.0.0.1}"
  DB_PORT="${MYSQL_PORT:-3306}"
  DB_USER="${MYSQL_USER:-root}"
  DB_PASSWORD="${MYSQL_PASSWORD:-}"
  DB_NAME="${MYSQL_DATABASE:-finanzas}"
  MIGRATE_DSN="${DB_USER}:${DB_PASSWORD}@tcp(${DB_HOST}:${DB_PORT})/${DB_NAME}?parseTime=true&multiStatements=true&charset=utf8mb4&loc=Local"
else
  [[ -n "${DATABASE_URL:-}" ]] || {
    echo "error: definí DATABASE_URL en env.secrets o las variables MYSQL_*" >&2
    exit 1
  }
  MIGRATE_DSN="$DATABASE_URL"

  # Parseo del DSN: user[:pass]@tcp(host:port)/dbname?params
  # (no acepta sockets unix ni passwords con '@' sin escapar: falla temprano).
  if [[ "$DATABASE_URL" =~ ^([^:@]*)(:([^@]*))?@tcp\(([^:]+):([^/]+)\)/([^?]+) ]]; then
    DB_USER="${BASH_REMATCH[1]}"
    DB_PASSWORD="${BASH_REMATCH[3]:-}"
    DB_HOST="${BASH_REMATCH[4]}"
    DB_PORT="${BASH_REMATCH[5]}"
    DB_NAME="${BASH_REMATCH[6]}"
  else
    echo "error: DATABASE_URL no parseable (se espera user:pass@tcp(host:port)/db?params)" >&2
    exit 1
  fi
fi

# --- Validaciones -----------------------------------------------------------
if [[ ! "$DB_NAME" =~ ^[A-Za-z0-9_]+$ ]]; then
  echo "error: nombre de base inválido: '$DB_NAME' (solo [A-Za-z0-9_])" >&2
  exit 1
fi

# --- 1) Crear la base si no existe -------------------------------------------
mysql_args=(mysql --protocol=tcp -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER"
            --batch --skip-column-names)
if [[ -n "$DB_PASSWORD" ]]; then
  export MYSQL_PWD="$DB_PASSWORD"
fi
echo "==> Creando base \`$DB_NAME\` (si no existe)..."
"${mysql_args[@]}" -e "CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

# Transición desde la vieja migración inline: golang-migrate espera UNA sola
# fila en schema_migrations (su DELETE usa LIMIT 1). Si la base existente
# quedó con varias filas (el código viejo insertaba v1 y v2 por separado),
# las colapsamos a MAX(version). Es idempotente y no toca datos de negocio.
row_count="$("${mysql_args[@]}" --batch --skip-column-names "$DB_NAME" \
  -e "SELECT COUNT(*) FROM information_schema.tables
      WHERE table_schema = '$DB_NAME' AND table_name = 'schema_migrations'")"
if [[ "$row_count" == "1" ]]; then
  echo "==> Normalizando schema_migrations (colapso a MAX(version))..."
  "${mysql_args[@]}" --batch --skip-column-names "$DB_NAME" \
    -e "DELETE FROM schema_migrations
        WHERE version < (SELECT v FROM (SELECT MAX(version) AS v FROM schema_migrations) AS t)"
fi
unset MYSQL_PWD

# --- 2) Aplicar migraciones desde los archivos -------------------------------
MIGRATE_VERSION="${MIGRATE_VERSION:-v4.18.3}"
echo "==> Aplicando migraciones (golang-migrate $MIGRATE_VERSION)..."
go run -tags 'mysql' "github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION}" \
  -source "file://$ROOT_DIR/migrations" \
  -database "mysql://$MIGRATE_DSN" up

echo "==> Base lista. Ahora podés correr: make run"
