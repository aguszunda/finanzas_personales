# API — Administración Financiera

Base URL: `http://localhost:8080`

Formato: JSON. Autenticación: el JWT se acepta por tres vías (basta con una):

1. `Authorization: Bearer <token>` (recomendado para API).
2. `?token=<token>` como query param.
3. Cookie `token` (HttpOnly → la setea el servidor en login/register).

```bash
curl -s http://localhost:8080/api/categorias -H "Authorization: Bearer $TOKEN"
```

---

## Endpoints públicos

### `GET /health`
Healthcheck (sin autenticación).

```bash
curl -s http://localhost:8080/health
```

**Respuesta 200:**
```json
{"status":"ok"}
```

### `GET /`
Redirige a `/api/dashboard/page` si hay cookie de sesión, `/login` en caso contrario.

### `GET /login` · `GET /register`
Páginas HTML (formulario de login/registro).

---

## Autenticación

### `POST /api/auth/register`
Registra un usuario, crea la sesión (cookie `token`) y devuelve token JWT.

```bash
curl -s -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"nombre":"Agustin","email":"agustin@example.com","password":"secreto123","moneda_default":"ARS"}'
```

**Respuesta 201:**
```json
{
  "token": "<TU_TOKEN_JWT>",
  "usuario": {
    "id": 1, "nombre": "Agustin", "email": "agustin@example.com",
    "moneda_default": "ARS", "created_at": "2026-07-30T12:00:00Z"
  }
}
```

La cookie `token` se fija con `Path=/`, `HttpOnly`, `SameSite=Lax`, `Secure` y `Max-Age` 72 h.

### `POST /api/auth/login`

```bash
curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"agustin@example.com","password":"secreto123"}'
```

**Respuesta 200:** igual que register (token + usuario + cookie). Si las credenciales no son válidas → `401`.

---

## Categorías (protegido)

### `GET /api/categorias`
Devuelve las categorías de sistema (`es_personalizada = false`) mezcladas con las del usuario.

```bash
curl -s http://localhost:8080/api/categorias -H "Authorization: Bearer $TOKEN"
```

**Respuesta 200:** array de `{id, nombre, tipo, icono, es_personalizada, usuario_id, created_at}`.

---

## Transacciones (protegido)

### `GET /api/transacciones`
Query params: `limit` (default 50), `offset`, `periodo` (YYYY-MM).

> Si se envía `periodo`, se ignora `limit`/`offset` y se devuelven todas las del período.

```bash
curl -s "http://localhost:8080/api/transacciones?limit=50" -H "Authorization: Bearer $TOKEN"
curl -s "http://localhost:8080/api/transacciones?periodo=2026-07" -H "Authorization: Bearer $TOKEN"
```

### `POST /api/transacciones`
`fecha` opcional (default: hoy). `monto > 0` y `tipo ∈ {ingreso, egreso}`. Si el mes del período está **cerrado** → `409`.

```bash
curl -s -X POST http://localhost:8080/api/transacciones \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{
    "tipo": "egreso",
    "monto": 45000,
    "fecha": "2026-07-12",
    "categoria_id": 5,
    "descripcion": "Alquiler julio",
    "medio_pago": "debito",
    "es_fijo": true,
    "cuotas_total": null,
    "cuota_actual": null
  }'
```

**Respuesta 201:** transacción creada.

### `GET /api/transacciones/{id}`
**404** si no existe o no pertenece al usuario.

### `PUT /api/transacciones/{id}`
Mismo body que POST (`fecha` opcional). `404` si no existe o no pertenece al usuario; `409` si el mes está cerrado.

### `DELETE /api/transacciones/{id}`
`204 No Content` en éxito. `409` si el mes está cerrado.

---

## Costos Fijos (protegido)

### `GET /api/costos-fijos`
### `GET /api/costos-fijos/{id}`

### `POST /api/costos-fijos`
`tipo_periodo` opcional (default `"mensual"`), `dia_vencimiento` entre 1 y 31. Materializa el costo fijo como transacción `pendiente` del mes en curso (se omite si está cerrado).

```bash
curl -s -X POST http://localhost:8080/api/costos-fijos \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{
    "categoria_id": 6,
    "descripcion": "Internet",
    "monto_estimado": 12000,
    "dia_vencimiento": 5,
    "tipo_periodo": "mensual"
  }'
```

### `PUT /api/costos-fijos/{id}`
Mismo body que POST.

### `PATCH /api/costos-fijos/{id}/toggle`
Activa/desactiva (flipea `activo`). Al reactivarlo vuelve a materializarse en el mes en curso.

### `DELETE /api/costos-fijos/{id}`
`204 No Content` en éxito.

---

## Meses (protegido)

El objeto `Mes` incluye indicadores calculados:

```json
{
  "id": 3, "usuario_id": 1, "periodo": "2026-07", "estado": "abierto",
  "ingresos_total": 150000, "egresos_total": 45000,
  "superavit": 105000, "tasa_ahorro": 70,
  "ahorro_acumulado": 105000, "pasivos_total": 0, "patrimonio": 105000,
  "created_at": "2026-07-01T00:00:00Z"
}
```

### `GET /api/meses`
Lista meses ordenados desc.

### `GET /api/meses/current`
Devuelve (o crea) el mes del período actual.

### `GET /api/meses/{id}`

### `POST /api/meses/{id}/cerrar`
Cierra el mes (inmutable), recalcula sus indicadores + acumulados y precarga los costos fijos activos como transacciones `pendientes` del próximo mes. `409` si ya está cerrado. Respuesta HTMX/303: redirige a `/api/balance/{id}/page`.

```bash
curl -s -X POST http://localhost:8080/api/meses/1/cerrar -H "Authorization: Bearer $TOKEN"
```

### `POST /api/meses/{id}/recalcular`
Recalcula ingresos/egresos/superávit/tasa + acumulados sin cerrar. `409` si el mes está cerrado.

---

## Deudas (protegido)

### `GET /api/deudas`
Lista de deudas del usuario.

### `POST /api/deudas`
`entidad` (obligatoria) y `monto_total` (> 0) requeridos; `tipo` opcional (default `"otro"`), debe ser una de: `tarjeta_credito`, `prestamo`, `hipoteca`, `personal`, `otro`.

```bash
curl -s -X POST http://localhost:8080/api/deudas \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{
    "tipo": "tarjeta_credito",
    "entidad": "Visa",
    "descripcion": "Cuota 3 meses",
    "monto_total": 150000,
    "proximo_vencimiento": "2026-08-15"
  }'
```

**Respuesta 201:**
```json
{
  "id": 1, "usuario_id": 1, "tipo": "tarjeta_credito",
  "entidad": "Visa", "descripcion": "Cuota 3 meses",
  "monto_total": 150000, "proximo_vencimiento": "2026-08-15",
  "created_at": "2026-07-30T12:00:00Z"
}
```

### `GET /api/deudas/{id}`
`404` si no existe o no pertenece al usuario.

### `PUT /api/deudas/{id}`
Mismo body que POST. `404` si no existe o no pertenece al usuario.

### `DELETE /api/deudas/{id}`
`204 No Content` en éxito.

> Las deudas alimentan los indicadores `pasivos_total` y `patrimonio` de cada `Mes` al recalcular/cerrar.

---

## Dashboard (protegido)

### `GET /api/dashboard`
```bash
curl -s http://localhost:8080/api/dashboard -H "Authorization: Bearer $TOKEN"
```

**Respuesta 200:**
```json
{
  "mes_actual": {
    "id": 3, "usuario_id": 1, "periodo": "2026-07", "estado": "abierto",
    "ingresos_total": 150000, "egresos_total": 45000,
    "superavit": 105000, "tasa_ahorro": 70,
    "ahorro_acumulado": 105000, "pasivos_total": 0, "patrimonio": 105000,
    "created_at": "2026-07-01T00:00:00Z"
  },
  "mes_anterior": { "...": "..." },
  "gastos_por_categoria": [
    {"categoria_id": 5, "categoria": "Alquiler", "monto": 45000, "porcentaje": 100, "icono": "🏠"}
  ],
  "ultimos_movimientos": [ "...", "..." ]
}
```

`mes_anterior` viene `omitempty` (ausente si no hay mes del período anterior). `ultimos_movimientos` son como máximo las 5 transacciones más recientes del mes actual.

---

## Páginas HTML (HTMX)

| Ruta | Contenido |
|------|-----------|
| `/login` · `/register` | Formularios de autenticación |
| `/api/dashboard/page` | Dashboard |
| `/api/transacciones/page` | CRUD transacciones (`?periodo=YYYY-MM`, default `all`) |
| `/api/costos-fijos/page` | CRUD costos fijos |
| `/api/meses/page` | Lista de meses |
| `/api/deudas/page` | CRUD deudas |
| `/api/balance/page` | Balance del mes actual (imprimible) |
| `/api/balance/{id}/page` | Balance de mes específico |
| `/api/transacciones/form[?edit_id={id}]` | Fragmento HTML del formulario (modo crear o editar) |
| `/api/deudas/form[?edit_id={id}]` | Fragmento HTML del formulario (modo crear o editar) |

Todas las páginas protegidas requieren auth. Los endpoints `*/form` devuelven un fragmento HTML (sin layout) usado por el modal: sin `edit_id` renderizan el form vacío con `hx-post`; con `edit_id` precargan el registro y responden con `hx-put`. El verbo y los valores se generan en el servidor para que HTMX los procese al hacer el swap. El balance tiene CSS `@media print`: `Ctrl+P` → PDF limpio.

---

## Comportamiento de las mutaciones (dual-mode)

`POST/PUT/PATCH/DELETE` responden según quién los invoque (mismo endpoint, tres comportamientos):

- **HTMX** (`HX-Request: true`): header `HX-Redirect: <url>` para que la página recargue.
- **Formulario** (`application/x-www-form-urlencoded`): redirect `303 See Other`.
- **API client** (JSON): cuerpo JSON (201/200) o `204 No Content` en DELETE.

Los body de las mutaciones aceptan igualmente JSON o `x-www-form-urlencoded`.

---

## Códigos de Error

| HTTP | Caso |
|------|------|
| 400 | Form/JSON inválido o datos inválidos (validación de service) |
| 401 | Token faltante/inválido o credenciales incorrectas |
| 404 | Recurso no encontrado (inclusive si pertenece a otro usuario) |
| 409 | Email duplicado / mes cerrado / mes ya cerrado |
| 500 | Error interno |

El body de error es `{"error": "<mensaje>"}`.

---

## Notas de implementación (revisión de endpoints)

- Todos los recursos consultan y mutan **solo** datos del usuario autenticado (`usuario_id`); cualquier `GET/{id}`/`PUT`/`DELETE` de un recurso ajeno responde `404`.
- **CORS**: `AllowedMethods` es `GET, POST, PUT, DELETE, OPTIONS`. Ojo: el endpoint `PATCH /api/costos-fijos/{id}/toggle` **no** está en la lista, por lo que un cliente cross-origin no podrá usarlo (la preflight fallaría). Same-origin (HTMX) no se ve afectado.
- **Cookie de sesión**: la cookie `token` se marca `Secure`, por lo que en `localhost` servido por HTTP puro los navegadores no la persisten; los clientes de API deben usar el header `Authorization: Bearer` (curl funciona igual).
- La inmutabilidad de meses cerrados está preservada en todos los write paths de transacciones/costos fijos/recalcular (`ErrMesCerrado` → `409`).