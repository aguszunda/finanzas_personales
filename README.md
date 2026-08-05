# 💰 Administración Financiera — Monolito Go

Monolito de administración financiera personal / Pymes basado en los documentos de análisis:
`analisis-financiero-personal.md` (negocio y funcionalidades) y `monolito-go-resumen.md` (arquitectura y stack).

**Stack:** Go 1.24 · chi · go-sql-driver/mysql · JWT · HTMX + html/template · MySQL · Docker

---

## 1. Estructura del Proyecto

```
Administracion_financiera/
├── cmd/
│   └── server/
│       ├── main.go              # Entry point, wiring de dependencias
│       └── router.go            # Router chi (compartido con los tests de integración)
├── internal/
│   ├── config/                  # Configuración 12-factor (env vars)
│   │   └── config.go
│   ├── model/                   # Tipos de dominio compartidos
│   │   ├── models.go            # Usuario, Transaccion, CostoFijo, Categoria, Mes, Deuda...
│   │   └── errors.go            # Errores de dominio tipados
│   ├── middleware/              # Capa transversal HTTP
│   │   ├── auth.go              # JWT → inyecta userID en context
│   │   ├── logging.go           # Logging de requests con log/slog
│   │   └── htmx.go              # Detecta HX-Request
│   ├── repository/              # Acceso a datos (database/sql, SQL directo)
│   │   ├── usuario_repo.go
│   │   ├── transaccion_repo.go
│   │   ├── costofijo_repo.go
│   │   ├── categoria_repo.go
│   │   ├── mes_repo.go
│   │   └── deuda_repo.go
│   ├── service/                 # Lógica de negocio
│   │   ├── auth_service.go      # Registro, login, JWT
│   │   ├── transaccion_service.go
│   │   ├── costofijo_service.go
│   │   ├── mes_service.go       # Cierre mensual, recálculo, precarga costos fijos
│   │   ├── dashboard_service.go # Métricas y agregaciones
│   │   └── deuda_service.go     # Gestión de deudas
│   └── handler/                 # Capa HTTP (JSON API + páginas HTMX)
│       ├── helpers.go           # Respuestas JSON, manejo de errores
│       ├── template.go          # Carga de templates (embed)
│       ├── auth_handler.go
│       ├── transaccion_handler.go
│       ├── costofijo_handler.go
│       ├── mes_handler.go
│       ├── dashboard_handler.go
│       ├── categoria_handler.go
│       ├── deuda_handler.go
│       └── pages_handler.go     # Páginas HTML (dashboard, transacciones, balance)
├── web/
│   ├── embed.go                 # //go:embed templates → FS
│   └── templates/               # html/template + HTMX
│       ├── layout.html          # Base (nav, estilos, print CSS)
│       ├── login.html
│       ├── register.html
│       ├── dashboard.html
│       ├── transacciones.html
│       ├── costos_fijos.html
│       ├── balance.html
│       ├── meses.html
│       └── deudas.html
├── migrations/
│   ├── 001_init.up.sql / .down.sql
│   └── 002_deudas.up.sql / .down.sql
├── scripts/
│   ├── db-init.sh              # Crea la DB y aplica migraciones (make db-init)
│   └── coverage.sh             # Test coverage (make coverage)
├── docs/
│   ├── ARQUITECTURA.excalidraw  # Diagrama arquitectónico (abrir en excalidraw.com)
│   └── API.md                   # Documentación de endpoints con curls
├── .githooks/
│   └── commit-msg               # Valida Conventional Commits (make git-hooks)
├── Dockerfile                   # Multi-stage → binario único + templates
├── docker-compose.yml           # MySQL + migrate + app
├── Makefile
├── go.mod / go.sum
└── .env.example
```

---

## 2. Arquitectura (Clean Architecture en capas)

```
Browser (HTMX)
     │  HTML / hx-get / hx-post / hx-put / hx-delete
     ▼
[handler] ──► [service (interfaz de negocio)] ──► [repository (SQL)] ──► MySQL
     │  JSON API (/api/*)            │
     │  Páginas (/api/*/page)        │
     └────── JWT middleware ────────┘

Regla: cada capa solo conoce la inmediatamente inferior, siempre vía tipos.
```

**Pipeline del request:**

```
RequestID → Recoverer → Logging → CORS → DetectHTMX → Timeout
     └──────────────▶ (grupo público: /health, /login, /register)
     └──────────────▶ (grupo protegido: JWTAuth → handlers → services → repos)
```

### Rutas

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/health` | Healthcheck |
| GET | `/login`, `/register` | Páginas de auth |
| POST | `/api/auth/register` | Registro de usuario |
| POST | `/api/auth/login` | Login → JWT |
| GET | `/api/categorias` | Categorías del sistema + personalizadas |
| GET/POST | `/api/transacciones` | Listar / crear transacciones |
| GET/PUT/DELETE | `/api/transacciones/{id}` | CRUD transacción |
| GET/POST | `/api/costos-fijos` | Listar / crear costos fijos |
| GET/PUT/DELETE | `/api/costos-fijos/{id}` | CRUD costo fijo |
| PATCH | `/api/costos-fijos/{id}/toggle` | Activar/desactivar |
| GET | `/api/meses` | Meses del usuario |
| GET | `/api/meses/current` | Mes actual |
| POST | `/api/meses/{id}/cerrar` | Cerrar mes + precargar costos fijos |
| POST | `/api/meses/{id}/recalcular` | Recalcular indicadores |
| GET/POST | `/api/deudas` | Listar / crear deudas |
| GET/PUT/DELETE | `/api/deudas/{id}` | CRUD deuda |
| GET | `/api/dashboard` | Métricas JSON del dashboard |
| GET | `/api/dashboard/page` | Dashboard HTML |
| GET | `/api/transacciones/page` | Transacciones HTML |
| GET | `/api/costos-fijos/page` | Costos fijos HTML |
| GET | `/api/balance/page`, `/api/balance/{id}/page` | Balance imprimible |
| GET | `/api/meses/page`, `/api/deudas/page` | Páginas HTML de meses y deudas |

---

## 3. Funciones Principales

### `internal/service/mes_service.go` — Cierre mensual (regla de negocio 5.1)
- `Cerrar()`: congela el mes (estado `cerrado`), calcula ingresos/egresos/superávit/tasa de ahorro,
  acumula patrimonio, **precarga los costos fijos activos** como transacciones `pendientes` en el
  próximo mes y crea/abre el mes siguiente.
- `Recalcular()`: recalcula indicadores de un mes sin cerrarlo.

### `internal/service/transaccion_service.go`
- `Create()`: valida monto/tipo, resuelve/crea el `Mes` del período, **bloquea escritura si el mes está cerrado**.
- `Update()`/`Delete()`: rechazan operaciones sobre meses cerrados (regla 5.2).

### `internal/service/dashboard_service.go`
- `GetDashboard()`: calcula ingresos, egresos, resultado neto, tasa de ahorro, gastos por categoría
  (con porcentajes) y últimos movimientos del mes actual, comparando con el anterior.

### `internal/service/auth_service.go`
- `Register()` / `Login()`: hash bcrypt + emisión de JWT (HS256) con `sub = userID`.

### `internal/service/deuda_service.go`
- `Create()`: valida entidad, monto total > 0 y `saldo_pendiente` en `[0, monto_total]`; tipo por defecto `otro`
  (tipos válidos: `tarjeta_credito`, `prestamo`, `hipoteca`, `personal`, `otro`).
- `Update()`/`Delete()`: operan sobre deudas del usuario autenticado (tenancy por `usuario_id`).

### `internal/handler/helpers.go`
- `handleServiceError()`: traduce errores de dominio (`ErrNotFound`, `ErrMesCerrado`, …) a códigos HTTP.

---

## 4. Ejecución

### Requisitos
- Go 1.24+
- MySQL 8+ (o MariaDB)

### 1) Base de datos
Crear la base (si no existe) y aplicar las migraciones:
```bash
make db-init    # crea `finanzas` con utf8mb4_unicode_ci y corre migrations/*.up.sql
```
> La app NO migra al arrancar: `main.go` no tiene SQL. El único camino al esquema es `make db-init` (o `make migrate-up`). El script es idempotente y pinnea golang-migrate.

### 1.1) Conexión desde MySQL Workbench
El proyecto usa la misma instancia local de MySQL. Para ver y explorar los datos desde Workbench:

1. Abrí MySQL Workbench y creá una conexión nueva (`+` al lado de *MySQL Connections*).
2. Completá con estos datos:

| Campo | Valor |
|-------|-------|
| Connection Name | `finanzas-local` (o el que quieras) |
| Connection Method | `Standard (TCP/IP)` |
| Hostname | `127.0.0.1` |
| Port | `3306` |
| Username | `root` |
| Password | (vacío — guardalo en el *keychain* si te lo pide) |

3. Click **Test Connection** → **OK** → doble click en la conexión para abrirla.
4. En el panel izquierdo (*SCHEMAS*) aparece el esquema `finanzas`. Las tablas (`usuarios`, `categorias`, `meses`, `transacciones`, `costos_fijos`, `deudas`) se crean con `make db-init`.

> Si configuraste una contraseña para `root` (o usás otro usuario), los datos de la conexión tienen que coincidir con el `DATABASE_URL` de tu `.env`.

### 2) Configuración
```bash
cp .env.example .env
# editar DATABASE_URL si usás otro usuario/contraseña (formato DSN: user:pass@tcp(host:port)/db?params)
set -a; source .env; set +a
```
> Usá `source .env` (no `export $(...)`) porque el DSN de MySQL contiene `&` que el shell interpretaría.

### 3) Correr
```bash
make run        # o: go run ./cmd/server
```
> Antes del primer arranque corré `make db-init` (crea la base y aplica las migraciones).

### 4) Docker

**Opción A — Stack completo con Docker Compose** (MySQL + migraciones + app, recomendado):

```bash
make docker-up       # = docker compose up -d --build
# App en http://localhost:8080  ·  MySQL interno en la red de compose
docker compose logs -f app
make docker-down     # detener (conserva los datos en el volumen db_data)
```

La base `finanzas` se crea sola con `utf8mb4_unicode_ci` (variables `MYSQL_*` en `.env`). Un servicio `migrate` aplica `migrations/*.up.sql` antes de que arranque la app. El puerto de MySQL no se publica en el host por defecto; descomentá `ports` en `docker-compose.yml` si querés conectarte desde Workbench.

**Opción B — Sólo la imagen:**
```bash
make docker-build
make docker-run
```

---

## 5. Reglas de Negocio Implementadas

| Regla | Implementación |
|-------|----------------|
| Patrimonio = Activos − Pasivos | `mes_service` al cerrar; `patrimonio` en tabla `meses` |
| Resultado neto = Ingresos − Egresos | `mes_service.Cerrar`, `dashboard_service` |
| Tasa de ahorro = Superávit / Ingresos | `mes_service`, objetivo >15% (dashboard) |
| Mes cerrado = inmutable | `transaccion_service` valida `estado='cerrado'` |
| Costos fijos → precarga mensual | `mes_service.Cerrar` → `CreateTransaccionesFromFijos` |
| Cierre manual o automático | endpoint `POST /api/meses/{id}/cerrar` |
| Correcciones vía ajustes | `TransaccionRepo.CreateAjuste` (estado `ajuste`) |
| Categorías predefinidas + subcategorías | seed en migración; `es_personalizada` soportada |
| Cobertura de gastos (meses de reserva) | calculable desde `ahorro_acumulado / gasto mensual` |

---

## 6. Endpoints con cURL

> Todos los ejemplos usan `jq` para formatear. Reemplazá `$TOKEN` tras el login.

### Autenticación

```bash
# Registrar usuario
curl -s -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"nombre":"Agustin","email":"agustin@example.com","password":"secreto123","moneda_default":"ARS"}'

# Login → captura del token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"agustin@example.com","password":"secreto123"}' | jq -r .token)
echo "TOKEN=$TOKEN"
```

### Categorías

```bash
curl -s http://localhost:8080/api/categorias -H "Authorization: Bearer $TOKEN" | jq
```

### Transacciones

```bash
# Crear ingreso
curl -s -X POST http://localhost:8080/api/transacciones \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"tipo":"ingreso","monto":150000,"fecha":"2026-07-10","categoria_id":1,"descripcion":"Sueldo julio","medio_pago":"transferencia"}'

# Crear egreso
curl -s -X POST http://localhost:8080/api/transacciones \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"tipo":"egreso","monto":45000,"fecha":"2026-07-12","categoria_id":5,"descripcion":"Alquiler","medio_pago":"debito","es_fijo":true}'

# Listar todas
curl -s "http://localhost:8080/api/transacciones?limit=50" -H "Authorization: Bearer $TOKEN" | jq

# Listar por período
curl -s "http://localhost:8080/api/transacciones?periodo=2026-07" -H "Authorization: Bearer $TOKEN" | jq

# Ver una
curl -s http://localhost:8080/api/transacciones/1 -H "Authorization: Bearer $TOKEN" | jq

# Editar
curl -s -X PUT http://localhost:8080/api/transacciones/1 \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"tipo":"ingreso","monto":160000,"fecha":"2026-07-10","categoria_id":1,"descripcion":"Sueldo julio corregido","medio_pago":"transferencia"}'

# Eliminar
curl -s -X DELETE http://localhost:8080/api/transacciones/1 -H "Authorization: Bearer $TOKEN" -w "%{http_code}"
```

### Costos Fijos

```bash
# Crear
curl -s -X POST http://localhost:8080/api/costos-fijos \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"categoria_id":6,"descripcion":"Internet","monto_estimado":12000,"dia_vencimiento":5,"tipo_periodo":"mensual"}'

# Listar
curl -s http://localhost:8080/api/costos-fijos -H "Authorization: Bearer $TOKEN" | jq

# Activar/desactivar
curl -s -X PATCH http://localhost:8080/api/costos-fijos/1/toggle -H "Authorization: Bearer $TOKEN" | jq

# Editar
curl -s -X PUT http://localhost:8080/api/costos-fijos/1 \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"categoria_id":6,"descripcion":"Internet + Fibra","monto_estimado":14000,"dia_vencimiento":5,"tipo_periodo":"mensual"}'

# Eliminar
curl -s -X DELETE http://localhost:8080/api/costos-fijos/1 -H "Authorization: Bearer $TOKEN" -w "%{http_code}"
```

### Deudas

```bash
# Crear
curl -s -X POST http://localhost:8080/api/deudas \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"tipo":"tarjeta_credito","entidad":"Visa","descripcion":"Cuota notebook","monto_total":300000,"saldo_pendiente":180000,"tasa_interes":42.5,"proximo_vencimiento":"2026-08-15"}'

# Listar
curl -s http://localhost:8080/api/deudas -H "Authorization: Bearer $TOKEN" | jq

# Ver una
curl -s http://localhost:8080/api/deudas/1 -H "Authorization: Bearer $TOKEN" | jq

# Editar (ej: actualizar saldo pendiente)
curl -s -X PUT http://localhost:8080/api/deudas/1 \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"tipo":"tarjeta_credito","entidad":"Visa","descripcion":"Cuota notebook","monto_total":300000,"saldo_pendiente":120000,"tasa_interes":42.5,"proximo_vencimiento":"2026-08-15"}'

# Eliminar
curl -s -X DELETE http://localhost:8080/api/deudas/1 -H "Authorization: Bearer $TOKEN" -w "%{http_code}"
```

### Meses y Cierre

```bash
# Mes actual
curl -s http://localhost:8080/api/meses/current -H "Authorization: Bearer $TOKEN" | jq

# Todos los meses
curl -s http://localhost:8080/api/meses -H "Authorization: Bearer $TOKEN" | jq

# Cerrar mes (precarga costos fijos en el siguiente)
curl -s -X POST http://localhost:8080/api/meses/1/cerrar -H "Authorization: Bearer $TOKEN" | jq

# Recalcular
curl -s -X POST http://localhost:8080/api/meses/1/recalcular -H "Authorization: Bearer $TOKEN" | jq
```

### Dashboard

```bash
curl -s http://localhost:8080/api/dashboard -H "Authorization: Bearer $TOKEN" | jq
```

### Páginas HTML (navegador / HTMX)

Desde el navegador entrá a `http://localhost:8080/` (redirige a `/login`). Creá una cuenta en `/register` o usá el form de `/login`; la sesión se mantiene con una cookie JWT.

```bash
open "http://localhost:8080/"                          # Redirige a /login
open "http://localhost:8080/api/dashboard/page"        # Dashboard
open "http://localhost:8080/api/transacciones/page"    # Transacciones
open "http://localhost:8080/api/costos-fijos/page"     # Costos fijos
open "http://localhost:8080/api/meses/page"            # Meses
open "http://localhost:8080/api/deudas/page"           # Deudas
open "http://localhost:8080/api/balance/page"          # Balance (Ctrl+P para PDF)
open "http://localhost:8080/api/balance/1/page"        # Balance de un mes específico
```

> Los formularios de login/registro/alta aceptan tanto JSON como `application/x-www-form-urlencoded`, así que funcionan igual desde la UI (HTMX) y desde la API.

---

## 7. Modelo de Datos

```
usuarios(id, nombre, email UNIQUE, password_hash, moneda_default, created_at)
categorias(id, nombre, tipo[ingreso|egreso], icono, es_personalizada, usuario_id?)
meses(id, usuario_id, periodo 'YYYY-MM' UNIQUE, estado[abierto|cerrado],
      ingresos_total, egresos_total, superavit, tasa_ahorro,
      ahorro_acumulado, pasivos_total, patrimonio, created_at)
transacciones(id, usuario_id, tipo, monto, fecha, categoria_id, descripcion,
      medio_pago, es_fijo, cuotas_total, cuota_actual,
      estado[pendiente|confirmado|ajuste], mes_id?, created_at, updated_at)
costos_fijos(id, usuario_id, categoria_id, descripcion, monto_estimado,
      dia_vencimiento[1-31], activo, tipo_periodo[mensual|bimestral|anual], created_at)
deudas(id, usuario_id, tipo[tarjeta_credito|prestamo|hipoteca|personal|otro], entidad,
      descripcion, monto_total, saldo_pendiente, tasa_interes, proximo_vencimiento, created_at, updated_at)
```

Categorías por defecto (seed): Sueldo 💰, Freelance 💻, Ventas 📦, Otros Ingresos 📥,
Alquiler 🏠, Servicios 💡, Comida 🍽️, Transporte 🚗, Salud 🏥, Educación 📚,
Entretenimiento 🎬, Suscripciones 📱, Imprevistos ⚠️.

---

## 8. Verificación

```bash
go vet ./...
go build ./...
go test ./...
```

## 8.1 Commits

Todos los commits deben seguir [Conventional Commits](https://www.conventionalcommits.org/) (`<type>(<scope>): <subject>`). Instalá el hook que lo valida una vez por clon:

```bash
make git-hooks
```

Si el hook rechaza un mensaje, corregilo y volvé a commitear.

---

## 9. Próximos Pasos (Fase 2)

- [ ] Metas de Ahorro (`metas_ahorro`) y asignación de superávit
- [ ] Presupuestos por categoría con alertas 80%/100%
- [ ] Gestión de tarjetas/deudas con calculadora de intereses
- [ ] Reportes anuales y exportación CSV server-side
- [ ] Importación de CSV bancario
