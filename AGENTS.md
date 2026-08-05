# AGENTS.md

Go 1.24 monolith: personal-finance web app. One binary serving a JSON API + server-rendered pages (HTMX + `html/template`). No framework, no ORM, no DI container — everything is wired by hand in `cmd/server/main.go`.

## Commands

- Run: `make run` (builds `bin/server` and runs on `:8080`). Requires a local MySQL instance with a `finanzas` database.
- Tests: `make test` (= `go test ./... -v`).
  - **Unit tests** (no DB needed): `internal/service` (via `go-sqlmock`), `internal/middleware`, `internal/handler/helpers`.
  - **Functional tests** (`cmd/server/integration_test.go`): exercise the full HTTP stack against a throwaway MySQL database (`finanzas_test_*`). They **auto-skip** if MySQL isn't reachable, so `go test ./...` still passes on a machine without a DB.
  - The router is built by `buildRouter` in `cmd/server/router.go`, shared by `main.go` and the integration tests.
- Verify: `go vet ./... && go build ./...`
- Env: copy `.env.example` to `.env`. Load it with `set -a; source .env; set +a` — NOT `export $(cat .env)` because the MySQL DSN contains `&` which the shell would mis-parse.
- Commit hooks: `make git-hooks` (validates Conventional Commits via `.githooks/commit-msg`).

## Commits (Conventional Commits)

Every commit MUST follow [Conventional Commits v1.0.0](https://www.conventionalcommits.org/). The `commit-msg` hook (`.githooks/`, install with `make git-hooks`) rejects commits that don't comply, so run it before committing.

Format: `<type>(<scope>): <subject>`

- Type (required): `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `style`, `perf`, `build`, `ci`.
- Scope (optional): the affected area, lowercase snake-case, e.g. `feat(transacciones): ...`.
- Subject: lowercase, imperative, no trailing period, < 72 chars. Spanish.
- Body (optional): after a blank line, explains the *what* and *why* (not the how).
- No emojis. Breaking changes: add `!` before `:` or `BREAKING CHANGE:` in the body.

Examples:
- `feat(meses): cerrar mes con recalculo de ahorro`
- `fix(transacciones): validar monto positivo`
- `refactor: move database migrations out of main.go`
- `docs: document conventional commits for all commits`

## Setup gotchas

- Create + migrate the DB first: `make db-init` (creates the `finanzas` database with `utf8mb4_unicode_ci` if missing and applies `migrations/*.up.sql`). The app does NOT create the database nor run migrations.
- Templates are embedded with `//go:embed` (`web/embed.go`), so template edits require rebuilding the binary.

## Migrations (important)

- The app does **not** migrate at startup. `cmd/server/main.go` has no SQL — the only schema source of truth is `migrations/*.sql`.
- `make db-init` (`scripts/db-init.sh`): creates the DB (if missing, `utf8mb4_unicode_ci`) and runs golang-migrate up. It also collapses duplicate `schema_migrations` rows left behind by the legacy inline migration (golang-migrate expects a single row).
- `make migrate-up` / `make migrate-down`: golang-migrate pinned to `MIGRATE_VERSION=v4.18.3`; the CLI needs `-tags 'mysql'` or the driver isn't registered.
- If you change the schema: add a new `NNN_nombre.up.sql` + `NNN_nombre.down.sql` pair (bump the number). Never edit an applied migration. Then run `make migrate-up`.
- Integration tests exercise the real migration files through golang-migrate (`cmd/server/migrate_helper_test.go`), so tests always run the same schema as dev/prod.

## Architecture

Layered, dependency flow is strictly downward, constructed bottom-up in `main.go`:

```
handler → service → repository → MySQL   (all via concrete types; model holds shared structs)
```

- `internal/model/`: domain structs (`models.go`) and typed errors (`errors.go`).
- `internal/middleware/auth.go`: JWT validation + injects userID into context; handlers read it via `middleware.UserIDFromContext(ctx)`.
- `internal/handler/helpers.go`: `decodeBody` (accepts JSON **and** `x-www-form-urlencoded`), `respondMutation` (HTMX vs form vs JSON), `handleServiceError` (maps domain errors → HTTP status).

### Conventions that differ from defaults

- **Tenancy is mandatory**: every repository query filters by `usuario_id`. New queries must too, or users can read/modify each other's data.
- **HTMX dual-mode**: requests carry `HX-Request: true`; `respondMutation` answers with an `HX-Redirect` header for HTMX, 303 for plain forms, JSON for API clients. Same endpoint serves both — preserve this.
- **Domain errors, not HTTP errors, in services**: add sentinel errors to `internal/model/errors.go` and map them in `helpers.go:handleServiceError`.
- **Pages vs API**: API handlers respond JSON; page rendering lives in `internal/handler/pages_handler.go`, which renders templates that `{{define "content"}}` (layout `web/templates/layout.html` wraps them).
- **Money**: `float64` in Go structs, `DECIMAL(15,2)` in MySQL.
- **Categories**: system categories have `es_personalizada = FALSE, usuario_id = NULL`; queries mix them with user ones via `WHERE es_personalizada = FALSE OR usuario_id = ?`.
- **Month closure rule**: transacciones in a `cerrado` mes are immutable — `transaccion_service` returns `ErrMesCerrado` (→ HTTP 409). Preserve this on any new write path.

## Auth / JWT

- HS256, symmetric secret from `JWT_SECRET`. Claims: `sub` (userID), `exp`, `iat`. Never put sensitive data in claims (tokens are base64-readable, not encrypted).
- Session cookie `token` is `HttpOnly` + `SameSite=Lax`; middleware accepts token from Authorization header, `?token=` query, or cookie.
- New protected routes go inside the `r.Group(func(r chi.Router) { r.Use(middleware.JWTAuth(...)) })` block in `main.go`.

## Adding a feature (house pattern)

1. struct in `model/models.go` → 2. schema (`migrations/NNN_nombre.up.sql` + `.down.sql`) → 3. repo methods (filter by `usuario_id`) → 4. service with validations → 5. handler → 6. wire + routes in `cmd/server/main.go` → 7. template + nav link in `layout.html`.
