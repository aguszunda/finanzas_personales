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
`entidad` (obligatoria) y `monto_total` (> 0) requeridos; `tipo` opcional (default `"otro"`), debe ser una de: `tarjeta_credito`, `prestamo`, `hipoteca`, `personal`, `otro`. Opcionales: `categoria_id` (categoría de egreso que se usará por defecto al pagar) y `medio_pago` (`efectivo`, `debito`, `credito`, `transferencia`; se aplica al egreso generado al pagar).

```bash
curl -s -X POST http://localhost:8080/api/deudas \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{
    "tipo": "tarjeta_credito",
    "entidad": "Visa",
    "descripcion": "Cuota 3 meses",
    "monto_total": 150000,
    "categoria_id": 6,
    "medio_pago": "debito",
    "proximo_vencimiento": "2026-08-15"
  }'
```

**Respuesta 201:**
```json
{
  "id": 1, "usuario_id": 1, "tipo": "tarjeta_credito",
  "entidad": "Visa", "descripcion": "Cuota 3 meses",
  "monto_total": 150000, "categoria_id": 6, "medio_pago": "debito",
  "proximo_vencimiento": "2026-08-15",
  "estado": "pendiente", "created_at": "2026-07-30T12:00:00Z"
}
```

`estado` es `"pendiente"` (nueva) o `"pagada"` (ver `POST /api/deudas/{id}/pagar`).

### `GET /api/deudas/{id}`
`404` si no existe o no pertenece al usuario.

### `PUT /api/deudas/{id}`
Mismo body que POST. `404` si no existe o no pertenece al usuario. El `estado` actual se conserva.

### `DELETE /api/deudas/{id}`
`204 No Content` en éxito.

### `POST /api/deudas/{id}/pagar`
Marca la deuda como `"pagada"` y registra automáticamente una transacción de **egreso** por su `monto_total` con la categoría y fecha elegidas. A partir de ese momento la deuda deja de contar en `pasivos_total` y de aparecer en `ultimos_movimientos` (en su lugar figura el egreso).

Body: `{"categoria_id": 7}` y opcionalmente `"medio_pago"` y `"fecha": "2026-09-10"`. Si `categoria_id` es `0` (u omitida) se usa la categoría guardada en la deuda (y falla si la deuda no tiene); si `medio_pago` está vacío se usa el de la deuda. Ambos se aplican al egreso generado. La categoría debe ser de tipo `egreso` (del usuario o de sistema). La fecha es opcional: vacía ⇒ **hoy**, y no puede caer en un mes cerrado.

- `200` con la deuda actualizada (`estado: "pagada"`).
- `400` si la categoría no es de egreso, no existe, la deuda no tiene categoría y no se mandó, la deuda ya está pagada o la fecha tiene formato inválido.
- `409` si el mes de la fecha (o el actual, si no se manda fecha) está cerrado (`ErrMesCerrado`).
- `404` si la deuda no existe o no pertenece al usuario.

> El fragmento de confirmación (`/api/deudas/{id}/pagar/form`) precarga la categoría y la forma de pago de la deuda (editable) y la fecha: hoy si el mes actual está abierto, o una fecha en el primer mes abierto (el que deja `Cerrar`) con un aviso cuando el mes actual está cerrado.

> Las deudas pendientes alimentan los indicadores `pasivos_total` y `patrimonio` de cada `Mes` al recalcular/cerrar; las pagadas ya no.

---

## Balance General (protegido)

### `GET /api/dashboard`
```bash
curl -s http://localhost:8080/api/dashboard -H "Authorization: Bearer $TOKEN"
curl -s "http://localhost:8080/api/dashboard?periodo=2026-07" -H "Authorization: Bearer $TOKEN"
```

`?periodo=YYYY-MM` es opcional: filtra `ultimos_movimientos` al mes completo. Sin el parámetro se muestran los `ultimos_movimientos` de los últimos 10 días.

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
  "ultimos_movimientos": [
    {"id": 1, "origen": "transaccion", "tipo": "ingreso", "monto": 150000, "fecha": "2026-07-02", "categoria_nombre": "Sueldo", "descripcion": "Sueldo julio"},
    {"id": 2, "origen": "deuda", "tipo": "deuda", "monto": 60000, "fecha": "2026-07-05", "categoria_nombre": "Visa", "descripcion": "Celular"}
  ]
}
```

`mes_anterior` viene `omitempty` (ausente si no hay mes del período anterior). `ultimos_movimientos` es un feed único y ordenado por fecha: une **transacciones** (`origen: "transaccion"`) con **deudas** (`origen: "deuda"`, cada una como movimiento con su `monto_total` y fecha de alta), tanto las del mes actual como las del período filtrado. Las deudas pagadas **no** aparecen en el feed (su egreso las reemplaza). Las deudas **no** suman a los `egresos_total`.

---

## Páginas HTML (HTMX)

| Ruta | Contenido |
|------|-----------|
| `/login` · `/register` | Formularios de autenticación |
| `/api/dashboard/page` | Balance General (con `?periodo=YYYY-MM`, default últimos 10 días) |
| `/api/transacciones/page` | CRUD transacciones (`?periodo=YYYY-MM`, default `all`) |
| `/api/costos-fijos/page` | CRUD costos fijos |
| `/api/meses/page` | Lista de meses |
| `/api/deudas/page` | CRUD deudas + botón "Pagar" |
| `/api/balance/page` | Balance del mes actual (imprimible) |
| `/api/balance/{id}/page` | Balance de mes específico |
| `/api/transacciones/form[?edit_id={id}]` | Fragmento HTML del formulario (modo crear o editar) |
| `/api/deudas/form[?edit_id={id}]` | Fragmento HTML del formulario (modo crear o editar) |
| `/api/deudas/{id}/pagar/form` | Fragmento HTML de confirmación de pago (elige categoría del egreso) |

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