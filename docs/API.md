# API — Administración Financiera

Base URL: `http://localhost:8080`

Formato: JSON. Autenticación: `Authorization: Bearer <token>`.

---

## Autenticación

### `POST /api/auth/register`
Registra un usuario y devuelve token JWT.

```bash
curl -s -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"nombre":"Agustin","email":"agustin@example.com","password":"secreto123","moneda_default":"ARS"}'
```

**Respuesta 201:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "usuario": {
    "id": 1, "nombre": "Agustin", "email": "agustin@example.com",
    "moneda_default": "ARS", "created_at": "2026-07-30T12:00:00Z"
  }
}
```

### `POST /api/auth/login`

```bash
curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"agustin@example.com","password":"secreto123"}'
```

**Respuesta 200:** igual que register (token + usuario).

---

## Categorías (protegido)

### `GET /api/categorias`
```bash
curl -s http://localhost:8080/api/categorias -H "Authorization: Bearer $TOKEN"
```

**Respuesta 200:** array de `{id, nombre, tipo, icono, es_personalizada, usuario_id, created_at}`.

---

## Transacciones (protegido)

### `GET /api/transacciones`
Query params: `limit` (default 50), `offset`, `periodo` (YYYY-MM).

```bash
curl -s "http://localhost:8080/api/transacciones?limit=50" -H "Authorization: Bearer $TOKEN"
curl -s "http://localhost:8080/api/transacciones?periodo=2026-07" -H "Authorization: Bearer $TOKEN"
```

### `POST /api/transacciones`
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

**Respuesta 201:** transacción creada. Si el mes del período está **cerrado** → `409` "el mes está cerrado, no se puede modificar".

### `GET /api/transacciones/{id}`

```bash
curl -s http://localhost:8080/api/transacciones/1 -H "Authorization: Bearer $TOKEN"
```

### `PUT /api/transacciones/{id}`
Mismo body que POST. `404` si no existe o no pertenece al usuario.

### `DELETE /api/transacciones/{id}`
`204 No Content` en éxito. `409` si el mes está cerrado.

---

## Costos Fijos (protegido)

### `GET /api/costos-fijos`
```bash
curl -s http://localhost:8080/api/costos-fijos -H "Authorization: Bearer $TOKEN"
```

### `POST /api/costos-fijos`
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
### `PATCH /api/costos-fijos/{id}/toggle`
Activa/desactiva (flipea `activo`).
### `DELETE /api/costos-fijos/{id}`

---

## Meses (protegido)

### `GET /api/meses`
Lista meses ordenados desc. Incluye indicadores calculados.

### `GET /api/meses/current`
Devuelve (o crea) el mes del período actual.

### `GET /api/meses/{id}`

### `POST /api/meses/{id}/cerrar`
Cierra el mes (inmutable) y precarga los costos fijos activos como transacciones `pendientes` del próximo mes. Ejecuta el flujo de la regla 5.1.

```bash
curl -s -X POST http://localhost:8080/api/meses/1/cerrar -H "Authorization: Bearer $TOKEN"
```

### `POST /api/meses/{id}/recalcular`
Recalcula ingresos/egresos/superávit/tasa sin cerrar.

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
    "id": 3, "periodo": "2026-07", "estado": "abierto",
    "ingresos_total": 150000, "egresos_total": 45000,
    "superavit": 105000, "tasa_ahorro": 70
  },
  "mes_anterior": { "...": "..." },
  "gastos_por_categoria": [
    {"categoria_id": 5, "categoria": "Alquiler", "monto": 45000, "porcentaje": 100, "icono": "🏠"}
  ],
  "ultimos_movimientos": [ "...", "..." ]
}
```

---

## Páginas HTML (HTMX)

| Ruta | Contenido |
|------|-----------|
| `/api/dashboard/page` | Dashboard |
| `/api/transacciones/page` | CRUD transacciones |
| `/api/costos-fijos/page` | CRUD costos fijos |
| `/api/balance/page` | Balance del mes actual (imprimible) |
| `/api/balance/{id}/page` | Balance de mes específico |

Las páginas usan HTMX: envían `HX-Request: true` y el servidor responde HTML (o JSON para los fragments de dashboard). El balance tiene CSS `@media print`: `Ctrl+P` → PDF limpio.

---

## Códigos de Error

| HTTP | Caso |
|------|------|
| 400 | JSON inválido o datos inválidos |
| 401 | Token faltante/inválido o credenciales incorrectas |
| 404 | Recurso no encontrado |
| 409 | Email duplicado / mes cerrado |
| 500 | Error interno |
