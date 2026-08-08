# Deuda pendiente — Análisis de seguridad y mantenimiento

> Documento vivo de lo que queda por corregir tras el análisis inicial.
> Se actualiza a medida que se cierran ítems (marcala como resuelta).

Prioridades: **P0** crítico · **P1** alto · **P2** medio · **P3** bajo.

---

## 1. Pendientes de seguridad

| # | Prioridad | Ítem | Detalle / riesgo | Estado |
|---|-----------|------|------------------|--------|
| S1 | P1 | Rate limiting en autenticación | `POST /api/auth/login` y `POST /api/auth/register` (`cmd/server/router.go:80-81`) son públicos y **no** tienen límite de intentos. Riesgo: fuerza bruta de contraseñas y abuso del registro (spam de cuentas). Sin lockout ni throttling por IP/usuario. | Pendiente |
| S2 | P1 | Límite de tamaño de body | `decodeBody` (`internal/handler/helpers.go:19-21`) decodifica JSON sin `http.MaxBytesReader`; el path JSON no tiene tope. Riesgo: DoS por bodies grandes (memoria/CPU). `ParseForm` sí tiene tope propio (~10MB), pero el JSON no. | Pendiente |
| S3 | P2 | CORS explícito en producción | `CORS_ORIGIN` default `*` (`internal/config/config.go`) con `AllowCredentials: true` (`router.go:56-61`): el middleware de chi refleja el origen pedido, o sea cualquier sitio puede hacer peticiones autenticadas por cookie desde el browser. En producción debe setearse un origen explícito y, en lo posible, rechazar `*` según entorno. | Pendiente |

### Resueltos en esta sesión (contexto)

| Ítem | Qué se hizo |
|------|-------------|
| `JWT_SECRET` con default conocido (`secret-super-seguro-cambiar-en-produccion`) | `config.Load()` ahora es fail-fast: obligatorio, ≥ 32 chars, rechaza placeholders conocidos. `main.go` aborta con error claro. |
| `DATABASE_URL` default `root:` sin password | Obligatorio + validación de formato DSN MySQL (`@tcp(`). |
| Secretos en `.env.example` / compose con valores conocidos | `.env.example` → `env.secrets` (gitignored). Compose exige `JWT_SECRET` (`${JWT_SECRET:?}`) y se carga con `--env-file env.secrets`. |
| Sin fail-fast en config | `Load()` devuelve `(*Config, error)`; sin env el proceso sale con status 1. |

---

## 2. Deuda técnica

| # | Prioridad | Ítem | Detalle | Estado |
|---|-----------|------|---------|--------|
| T1 | P3 | Umbral de cobertura duplicado | El `85` está hardcodeado en **3 lugares**: `scripts/coverage.sh:21` (default), `Makefile:18` y `.github/workflows/pr-validation.yml:137`. Cambiar el umbral exige tocar tres archivos y se desincronizan fácil. Centralizar en un único punto (default en `coverage.sh` + que Makefile/workflow lo lean, o una variable compartida). | Pendiente |
| T2 | P3 | `.env` viejo en el workspace | El `.env` local (con el `JWT_SECRET` placeholder viejo) ya no lo usa nada (compose y `db-init.sh` leen `env.secrets`). Es una trampa: correr `docker compose` sin `--env-file` lee `.env` y la app no arranca. Eliminarlo (es local, no está versionado). | Pendiente |

---

## 3. Notas operativas

- **Generar `JWT_SECRET`**: `openssl rand -base64 48` (ver `docs/GUIA-DESARROLLADOR.md` → "Generar / rotar el JWT_SECRET"). Nunca un valor del repo: la app lo rechaza.
- **Rotar `JWT_SECRET` invalida todas las sesiones** (firmas distintas). Requiere ventana de mantenimiento.
- **`env.secrets` es gitignored**: al clonar el repo hay que crearlo a mano con `DATABASE_URL` y `JWT_SECRET`.
- El DSN de MySQL contiene `&`: cargar secretos con `set -a; source env.secrets; set +a`, nunca `export $(cat env.secrets)`.

---

## 4. Sugerencia de orden de trabajo

1. **S1 + S2** (P1, seguridad): rate limiting y límite de body — mismo tipo de cambio (middleware en `buildRouter` + tests).
2. **S3** (P2): exigir `CORS_ORIGIN` explícito en producción.
3. **T2** (P3, un minuto): borrar el `.env` viejo local.
4. **T1** (P3, refactor menor): centralizar el umbral de cobertura.
