# 📘 Guía para entender el código de Administración Financiera

Documento pensado para que un **desarrollador Full-Stack Jr** entienda rápido cómo
funciona esta aplicación: qué hace, cómo está organizada, cómo viaja una petición,
cómo funciona la tokenización (JWT) y dónde vive cada parte del código.

> **En una frase:** es un monolito en Go que sirve una app web de finanzas personales
> (dashboard, transacciones, costos fijos, balance mensual), con una API JSON y una UI
> server-rendered que usa **HTMX** para ser interactiva sin escribir JavaScript.

---

## 1. Índice de contenidos

1. [Stack tecnológico](#2-stack-tecnológico)
2. [Mapa mental de la aplicación](#3-mapa-mental-de-la-aplicación)
3. [Estructura de carpetas](#4-estructura-de-carpetas)
4. [Arquitectura en capas](#5-arquitectura-en-capas)
5. [Ciclo de vida de una petición](#6-ciclo-de-vida-de-una-petición)
6. [Tokenización con JWT (lo más importante)](#7-tokenización-con-jwt-lo-más-importante)
7. [Cómo se protegen las rutas](#8-cómo-se-protegen-las-rutas)
8. [Frontend: HTMX + html/template](#9-frontend-htmx--htmltemplate)
9. [Modelo de datos](#10-modelo-de-datos)
10. [Lógica de negocio clave](#11-lógica-de-negocio-clave)
11. [Errores: del dominio al HTTP](#12-errores-del-dominio-al-http)
12. [Guía para empezar a hacer cambios](#13-guía-para-empezar-a-hacer-cambios)

---

## 2. Stack tecnológico

| Capa | Tecnología | ¿Para qué? |
|------|-----------|------------|
| Lenguaje | **Go 1.24** | Backend completo (API + páginas + lógica) |
| Router HTTP | **chi** (`github.com/go-chi/chi/v5`) | Enrutado de rutas y middleware |
| Base de datos | **MySQL** + `database/sql` | Persistencia (SQL directo, sin ORM) |
| Autenticación | **JWT** (HS256) + **bcrypt** | Sesiones sin estado, hash de contraseñas |
| Frontend | **html/template** + **HTMX 2** + Chart.js | HTML server-rendered, sin SPA |
| Logs | `log/slog` (estándar de Go) | Logging estructurado |
| Build | Makefile + Dockerfile multi-stage | Compilar y desplegar |

**Dato importante:** no hay framework web tipo Gin/Echo ni ORM tipo GORM. Todo es
librería estándar de Go + chi + SQL a mano. Eso hace que el código sea explícito y fácil
de seguir.

---

## 3. Mapa mental de la aplicación

```
                      ADMINISTRACIÓN FINANCIERA (Monolito)
                                    │
        ┌──────────────┬────────────┼────────────────┬────────────────┐
        │              │            │                │                │
    USUARIOS      CATEGORÍAS    TRANSACCIONES   COSTOS FIJOS      MESES
    registro /       │            ingreso/          │          (abierto/cerrado)
    login (JWT)      │            egreso            │               │
        │            │               │              │               │
        └────────────┴───────┬───────┘              │               │
                             │                      ▼               ▼
                        DASHBOARD            al cerrar mes ──► precarga el mes siguiente
                   (resumen del mes)         + genera transacciones "pendientes"
                   (gráficos, métricas)      con los costos fijos activos
```

**Las 4 entidades centrales del negocio:**

1. **Usuario** — quien usa la app (dueño de todos sus datos).
2. **Categoría** — clasifica transacciones en `ingreso` o `egreso`.
3. **Transacción** — un movimiento de dinero (sueldo, alquiler, comida...).
4. **Costo Fijo** — gasto que se repite todos los meses (suscripciones, alquiler...).
5. **Mes** — un período `YYYY-MM` que agrupa transacciones y guarda métricas (superávit, tasa de ahorro, patrimonio).

---

## 4. Estructura de carpetas

```
Administracion_financiera/
├── cmd/server/main.go          ← ENTRY POINT. Levanta todo, define rutas
├── internal/
│   ├── config/config.go        ← Lee variables de entorno (PORT, DATABASE_URL, JWT_SECRET...)
│   ├── model/
│   │   ├── models.go           ← Structs de dominio: Usuario, Transaccion, Mes, ...
│   │   └── errors.go           ← Errores de negocio tipados (ErrNotFound, ErrMesCerrado...)
│   ├── middleware/
│   │   ├── auth.go             ← Valida el JWT y guarda el userID en el contexto
│   │   ├── htmx.go             ← Detecta si la petición viene de HTMX
│   │   └── logging.go          ← Loggea cada request
│   ├── repository/             ← Capa de DATOS: SQL directo contra MySQL
│   │   ├── usuario_repo.go
│   │   ├── transaccion_repo.go
│   │   ├── costofijo_repo.go
│   │   ├── categoria_repo.go
│   │   └── mes_repo.go
│   ├── service/                ← Capa de NEGOCIO: validaciones y reglas
│   │   ├── auth_service.go     ← registro, login, genera JWT
│   │   ├── transaccion_service.go
│   │   ├── costofijo_service.go
│   │   ├── mes_service.go      ← cierre de mes, recálculo, precarga
│   │   └── dashboard_service.go
│   └── handler/                ← Capa HTTP: recibe requests, llama services
│       ├── helpers.go          ← JSON, cookies, redirecciones, manejo de errores
│       ├── template.go         ← carga los templates embebidos
│       ├── auth_handler.go
│       ├── transaccion_handler.go
│       ├── costofijo_handler.go
│       ├── mes_handler.go
│       ├── dashboard_handler.go
│       ├── categoria_handler.go
│       └── pages_handler.go    ← renderiza las PÁGINAS HTML
├── web/
│   ├── embed.go                ← "incrusta" los templates en el binario
│   └── templates/*.html        ← layout + páginas (dashboard, transacciones, ...)
├── migrations/001_init.*.sql   ← esquema SQL de referencia
├── docs/                       ← esta guía, API.md, diagrama excalidraw
├── Makefile                    ← atajos (make run, make build, make test)
└── Dockerfile
```

> **Consejo:** el punto de entrada real es `cmd/server/main.go`. Todo lo demás se
> instancia ahí y se conecta. Empezá leyendo ese archivo: es el "mapa del tesoro".

---

## 5. Arquitectura en capas

El código sigue una arquitectura en capas (estilo Clean Architecture simplificado):

```
Browser (HTMX / formularios)
         │
         │  HTTP request
         ▼
┌────────────────────────────────────────────────────────────┐
│  [handler]   → recibe el request, decodifica, responde      │
│  (internal/handler)                                         │
├────────────────────────────────────────────────────────────┤
│  [service]   → reglas de negocio, validaciones              │
│  (internal/service)                                         │
├────────────────────────────────────────────────────────────┤
│  [repository] → SQL, acceso a MySQL                         │
│  (internal/repository)                                      │
├────────────────────────────────────────────────────────────┤
│  [model]     → tipos de datos compartidos (structs)         │
│  (internal/model)                                           │
└────────────────────────────────────────────────────────────┘
         │
         ▼
       MySQL
```

**La regla de oro:** cada capa SOLO conoce a la que está inmediatamente debajo, y la
llama siempre a través de sus tipos/funciones públicas.

- `handler` → llama a `service` (nunca hace SQL directo, salvo `categoria_handler.go`
  que usa el repo directamente — excepción menor).
- `service` → llama a `repository`.
- `repository` → habla con la base de datos.
- `model` → define los structs que las capas se pasan entre sí.

**Ejemplo de dependencia real** (`cmd/server/main.go`):

```go
usuarioRepo := repository.NewUsuarioRepo(db)          // repo necesita la conexión db
authSvc := service.NewAuthService(usuarioRepo, ...)   // service necesita el repo
authH := handler.NewAuthHandler(authSvc)              // handler necesita el service
```

Es como apilar bloques: se construye de abajo hacia arriba y cada bloque depende del
que está debajo.

---

## 6. Ciclo de vida de una petición

Tomemos como ejemplo "crear una transacción" desde la UI (formulario HTMX):

```
 1. El usuario llena el form y HTMX hace POST /api/transacciones
 2. Middleware se ejecuta en orden:
    RequestID → Recoverer → Logging → CORS → DetectHTMX → Timeout
 3. Como la ruta está dentro del grupo protegido, corre JWTAuth:
    → lee el token (header / query / cookie)
    → si es válido, inyecta el userID en el contexto
 4. llega al handler TransaccionHandler.Create
 5. el handler decodifica el body (decodeBody)
 6. el handler llama transSvc.Create(userID, input)
 7. el service valida (monto>0, tipo correcto), resuelve/crea el Mes,
    verifica que el mes no esté cerrado, arma el struct
 8. el service llama transaccionRepo.Create(t) → INSERT en MySQL
 9. vuelve al handler: respondMutation → como es HTMX, responde
    con header "HX-Redirect: /api/transacciones/page"
10. HTMX navega a esa página y se muestra la lista actualizada
```

**Diagrama visual:**

```
 POST /api/transacciones
    │
    ▼
┌──middleware (todos)──────────────────────────────┐
│ RequestID │ Recoverer │ Logging │ CORS │ HTMX │  │
└───────────┴───────────┴─────────┴──────┴──────┘  │
    │                                               │
    ▼                                               │
┌── JWTAuth (solo rutas protegidas) ──────────────  │
│ token → válido? → sí → ctx = {userID: 5}        │
└───────────────────────────────────────────────────┘
    │
    ▼
 handler.Create
    │  decodeBody(JSON o form)
    ▼
 service.Create
    │  validar → FindOrCreate(mes) → ¿mes cerrado? → repo.Create
    ▼
 repository.Create  ──►  INSERT INTO transacciones ...  ──►  MySQL
    │
    ▼ (respuesta hacia arriba)
 handler → respondMutation
    │
    ├─ HTMX?        → HX-Redirect header
    ├─ form normal? → 303 redirect
    └─ API JSON?    → {"id": 42, ...}
```

---

## 7. Tokenización con JWT (lo más importante)

Esta es la parte que más se pregunta: **cómo sabe el servidor quién sos sin mantener
sesiones en memoria ni en la base de datos.** La respuesta: un **JWT** (JSON Web Token).

### 7.1 Qué es un JWT

Es un texto que tiene **tres partes separadas por puntos**, codificadas en Base64:

```
HEADER . PAYLOAD . SIGNATURE

eyJhbGciOiJIUzI1NiJ9 . eyJzdWIiOjUsImV4cCI6MTc... . 4xU8qF8Y...
```

| Parte | Contenido | Para qué sirve |
|-------|-----------|----------------|
| **Header** | `{"alg":"HS256","typ":"JWT"}` | Dice el algoritmo de firma |
| **Payload** | `{"sub":5,"exp":...,"iat":...}` | Los "claims" (datos) del token |
| **Signature** | hash(header.payload + secreto) | Prueba de que nadie lo falsificó |

En esta app el token guarda 3 claims (`internal/service/auth_service.go:87`):

```go
claims := jwt.MapClaims{
    "sub": float64(userID),                       // "subject" = ID del usuario
    "exp": time.Now().Add(s.jwtExpiration).Unix(), // expiración (JWT_EXPIRATION_HOURS)
    "iat": time.Now().Unix(),                      // emitido a las (issued at)
}
```

- `sub` → quién es el usuario (el ID).
- `exp` → cuándo expira (por defecto 72 hs).
- `iat` → cuándo se emitió.

### 7.2 Cómo se crea (firma)

En `auth_service.go`, función `generateToken`:

```go
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
return token.SignedString(s.jwtSecret)
```

- Se usa el algoritmo **HS256** (HMAC + SHA-256), que es **simétrico**: hay UNA sola
  clave secreta (`JWT_SECRET`) que sirve para firmar y para verificar.
- La firma es un hash de `header.payload` calculado con esa clave secreta.
- Cualquiera puede *leer* las tres partes (no están cifradas), pero **nadie puede
  alterarlas** sin conocer el secreto, porque el hash dejaría de coincidir.

```
firma = HMAC-SHA256( secreto,  "header.payload" )
```

### 7.3 Cómo se verifica (middleware)

En `internal/middleware/auth.go`, función `JWTAuth`. Los pasos:

1. **Extrae el token** de 3 lugares posibles, en orden:
   - header `Authorization: Bearer <token>`
   - query param `?token=...`
   - cookie `token` (así funciona el navegador)
2. **Lo parsea** y le pasa un callback que:
   - exige que el método de firma sea HMAC (`*jwt.SigningMethodHMAC`); si el atacante
     manda un token firmado con otro algoritmo (ej. `alg:none`), se rechaza. Esto
     previene el ataque "algorithm confusion".
   - devuelve la clave secreta para verificar la firma.
3. **Valida la firma y la expiración** (`token.Valid`).
4. **Extrae `sub`** y lo convierte a `int64`.
5. **Guarda el userID en el contexto** con `context.WithValue`.

```go
ctx := context.WithValue(r.Context(), UserIDKey, int64(sub))
next.ServeHTTP(w, r.WithContext(ctx))
```

A partir de ahí, cualquier handler puede recuperar el usuario con:

```go
uid := middleware.UserIDFromContext(r.Context())
```

### 7.4 Por qué es seguro (resumen)

| Ataque | Cómo lo detiene |
|--------|----------------|
| Modificar el payload (`sub`) | La firma deja de coincidir → rechazado |
| Usar `alg:none` u otro algoritmo | El callback exige `SigningMethodHMAC` |
| Token viejo / robado | Expira por `exp` (72 hs) |
| Adivinar el secreto | `JWT_SECRET` largo y aleatorio en producción |

#### Generar / rotar el `JWT_SECRET`

La app exige al arrancar que `JWT_SECRET` tenga **al menos 32 caracteres** y que
**no** sea un valor conocido (los placeholders tipo `secret-super-seguro-cambiar-en-produccion`
o `test-secret` se rechazan con fail-fast). No hay default.

Para generarlo (64 bytes aleatorios):

```bash
openssl rand -base64 48
```

- **Local** (`env.secrets`, gitignored) — reemplaza la línea sin dejar el valor en el historial:
  ```bash
  NEW="$(openssl rand -base64 48)" && sed -i '' "s|^JWT_SECRET=.*|JWT_SECRET=${NEW}|" env.secrets
  # después: docker compose --env-file env.secrets up -d --force-recreate app
  ```
  Recordá que el DSN de MySQL contiene `&`; cargalo con `set -a; source env.secrets; set +a`, nunca con `export $(cat env.secrets)`.
- **Producción**: inyectalo como *secret* del entorno (secret manager, Vault, secreto de k8s...). **Nunca** en un archivo versionado ni embebido en la imagen Docker:
  ```bash
  export JWT_SECRET="$(openssl rand -base64 48)"
  ```

> ⚠️ Rotación: al cambiar `JWT_SECRET`, todas las firmas dejan de coincidir → **todos los tokens
> emitidos se invalidan** (los usuarios pierden la sesión). Planearlo como un cambio con
> ventana de mantenimiento, no como algo silencioso en caliente.

> ⚠️ El token **no está cifrado**: viaja legible en Base64. Por eso NUNCA debe llevar
> datos sensibles (contraseñas, email, etc.), solo el `sub`. Y por eso la cookie se crea
> con `HttpOnly: true` (ver `auth_handler.go:setAuthCookie`): JavaScript no puede leerla,
> protegiendo contra XSS.

### 7.5 El ciclo completo de sesión

```
REGISTRO / LOGIN
    │  POST /api/auth/{register|login}
    ▼
AuthService: verifica bcrypt (login) o hashea (registro)
    ▼
generateToken(userID) → firma JWT con JWT_SECRET
    ▼
setAuthCookie(w, token) → cookie "token" HttpOnly
    ▼
respuesta: HX-Redirect → /api/dashboard/page

CADA REQUEST POSTERIOR (navegador)
    │  cookie "token" viaja automáticamente
    ▼
middleware JWTAuth: firma OK? exp OK? → userID en contexto
    ▼
handler usa UserIDFromContext(ctx) para filtrar datos del usuario
```

### 7.6 Páginas protegidas y redirección

En `main.go` hay dos rutas clave:

```go
r.Get("/", func(w, r) {
    if _, err := r.Cookie("token"); err == nil {
        http.Redirect(w, r, "/api/dashboard/page", ...) // ya logueado → dashboard
    }
    http.Redirect(w, r, "/login", ...)                  // no logueado → login
})
```

Y en el router de `/api`, todo lo que está dentro del `r.Group` con `JWTAuth` está
protegido (transacciones, costos fijos, meses, dashboard, páginas). Fuera de ese grupo
quedan solo `/api/auth/register`, `/api/auth/login` y `/health`.

---

## 8. Cómo se protegen las rutas

```
/api/auth/register  ── público
/api/auth/login     ── público
/health             ── público
/login, /register   ── público
─────────────────────────────────────────────
/api/transacciones/*        │
/api/costos-fijos/*         │
/api/meses/*                ├─ r.Group { JWTAuth }  → TODAS protegidas
/api/dashboard              │
/api/categorias             │
/api/*/page                 │
─────────────────────────────────────────────
```

Cada handler que toca datos de un usuario **siempre filtra por `usuario_id`** en la
consulta SQL. Por ejemplo, `transaccion_repo.go`:

```sql
WHERE t.id = ? AND t.usuario_id = ?   -- así un usuario NO puede ver datos de otro
```

Eso se llama **authorization por tenancy**: aunque un token sea válido, solo podés
acceder a TU información.

---

## 9. Frontend: HTMX + html/template

### 9.1 Cómo se renderizan las páginas

- Los templates viven en `web/templates/` y se **embeben en el binario** con
  `//go:embed` (`web/embed.go`). Por eso el Dockerfile copia el binario y los templates
  y el server funciona sin archivos externos.
- `handler/template.go` los carga a memoria y `handler/helpers.go:renderTemplate`
  combina `layout.html` (el esqueleto con nav + CSS) con la página que corresponde.
- Las páginas se definen con `{{define "content"}}...{{end}}` y el layout las inserta
  en `<main id="main-content">{{block "content" .}}{{end}}</main>`.

### 9.2 Qué hace HTMX

HTMX (cargado en `layout.html` vía CDN) te permite hacer peticiones AJAX **desde
atributos HTML**, sin escribir JavaScript:

```html
<form hx-post="/api/auth/login" hx-swap="none" ...>
```

| Atributo | Efecto |
|----------|--------|
| `hx-post` / `hx-get` / `hx-put` / `hx-delete` | Hace la petición con ese verbo |
| `hx-boost="true"` (en `<body>`) | Todas las navegaciones se vuelven AJAX |
| `hx-swap` | Cómo se inserta la respuesta en el DOM |

Cuando HTMX dispara una petición, agrega el header **`HX-Request: true`**. El
middleware `DetectHTMX` lo detecta y lo guarda en el contexto; luego
`respondMutation` usa eso para decidir cómo responder (ver sección 6).

### 9.3 Los dos mundos que comparte el mismo backend

| Mundo | Cómo llega | Cómo responde |
|-------|-----------|---------------|
| **API JSON** | `curl`, apps, o JS | JSON (`respondJSON`) |
| **UI web** | HTMX (header `HX-Request`) | Redirección vía `HX-Redirect` header |
| **UI web (sin JS)** | form `application/x-www-form-urlencoded` | Redirección HTTP 303 |

`decodeBody` acepta JSON **y** formularios, así que el mismo endpoint sirve a los dos
clientes. Ese es el "secreto" de que la app funcione sin SPA y sin duplicar código.

### 9.4 Gráficos

`dashboard.html` inyecta los datos (labels y valores) dentro del `<script>` y usa
**Chart.js** (CDN) para dibujar un gráfico de dona de "gastos por categoría". Los datos
vienen del server, no hay fetch desde JS.

---

## 10. Modelo de datos

### 10.1 Diagrama de entidades

```
 usuarios
 ├── id BIGINT PK
 ├── nombre, email UNIQUE, password_hash, moneda_default
 │
 ├── 1 ──── N  categorias (usuario_id nullable; es_personalizada)
 │            └── nombre, tipo[ingreso|egreso], icono
 │
 ├── 1 ──── N  meses (usuario_id, periodo 'YYYY-MM' UNIQUE)
 │            └── estado[abierto|cerrado], ingresos_total, egresos_total,
 │                superavit, tasa_ahorro, ahorro_acumulado,
 │                pasivos_total, patrimonio
 │
 ├── 1 ──── N  transacciones
 │            ├── tipo[ingreso|egreso], monto, fecha, descripcion,
 │            │   medio_pago, es_fijo, cuotas_*, estado
 │            ├── N──1 categorias (categoria_id FK)
 │            └── N──1 meses (mes_id FK, nullable)
 │
 └── 1 ──── N  costos_fijos
               ├── descripcion, monto_estimado, dia_vencimiento,
               │   activo, tipo_periodo[mensual|bimestral|anual]
               └── N──1 categorias (categoria_id FK)

  ── 1 ──── N  deudas (migración 002, estado en 004, categoria/medio_pago en 005)
               ├── tipo[tarjeta_credito|prestamo|hipoteca|personal|otro],
               │   entidad, descripcion, monto_total, proximo_vencimiento,
               │   estado[pendiente|pagada],
               │   categoria_id (FK→categorias, NULL = sin asignar: categoría
               │   de egreso usada por defecto al pagar), medio_pago
               └── indizados por usuario_id y estado
```

### 10.2 Reglas importantes del esquema

- **`transacciones.mes_id`** enlaza el movimiento al mes que le corresponde (se calcula
  a partir de la fecha: `fecha[:7]` = `YYYY-MM`).
- **`categorias.es_personalizada`**: las categorías de sistema tienen `usuario_id = NULL` y
  `es_personalizada = FALSE`; las del usuario tienen su `usuario_id`. La query las mezcla:
  `WHERE es_personalizada = FALSE OR usuario_id = ?`.
- **`estado`** en transacciones: `pendiente` (generadas automáticamente por costos
  fijos), `confirmado` (normal), `ajuste` (correcciones contables).
- **`deudas.estado`** (`pendiente|pagada`): al marcar una deuda como pagada se
  registra un egreso (misma fecha, categoría elegida) y pasa a `pagada`; deja de
  sumar a `pasivos_total` y de aparecer en el feed del balance general.
- Las FKs tienen `ON DELETE CASCADE`: borrar un usuario borra todos sus datos.

---

## 11. Lógica de negocio clave

### 11.1 El ciclo del mes (`internal/service/mes_service.go`)

Un mes nace cuando se crea la primera transacción (`FindOrCreate`) o cuando se cierra el
anterior. Cuando el usuario llama a **`POST /api/meses/{id}/cerrar`** (`Cerrar`):

1. Suma todos los ingresos y egresos del período.
2. Calcula `superavit = ingresos - egresos`.
3. Calcula `tasa_ahorro = superavit / ingresos * 100`.
4. Hereda el patrimonio del último mes cerrado (`ahorro_acumulado + superavit`).
5. Marca el mes como `cerrado` → ya **no se puede modificar** (inmutable).
6. **Crea el mes siguiente** y lo deja `abierto`.
7. **Precarga los costos fijos activos** del usuario como transacciones
   `pendiente` en el mes siguiente (`CreateTransaccionesFromFijos`).

```
 Cerrar julio 2026
   │
   ├─ sumar transacciones → superávit, tasa de ahorro
   ├─ estado = 'cerrado'  (nadie más puede editarlo)
   ├─ crear mes 2026-08 (abierto)
   └─ insertar transacciones 'pendiente':
        "Internet" 12.000 (Servicios) 2026-08-01
        "Alquiler" 45.000 (Alquiler)  2026-08-01
```

**Regla asociada (en `transaccion_service.go`):** al crear/editar/borrar una
transacción, primero verifica que su mes no esté `cerrado`; si lo está, devuelve
`ErrMesCerrado` y el handler traduce a HTTP 409.

### 11.2 El dashboard (`internal/service/dashboard_service.go`)

`GetDashboard(ctx, usuarioID, periodo)`:
1. Busca (o crea) el mes actual.
2. Suma ingresos y egresos reales del mes.
3. Calcula superávit y tasa de ahorro "en vivo".
4. Agrupa los egresos por categoría con sus porcentajes.
5. Arma el feed `ultimos_movimientos`: une transacciones y deudas
   (`unirMovimientos`) ordenadas por fecha desc. Por defecto muestra los
   últimos 10 días (`rango10Dias`); si se pasa `periodo` (YYYY-MM), la ventana
   es ese mes completo. Cada deuda aparece como un movimiento con su
   `monto_total` y fecha de alta; las deudas **no** suman a los egresos. Las
   deudas con `estado = 'pagada'` se excluyen del feed (`deuda_repo.go:
   FindByRango`): al pagarse quedaron representadas por su egreso.
6. Devuelve el mes anterior para comparar.

### 11.2.1 Marcar deuda como pagada (`deuda_service.go: MarcarPagada`)

`MarcarPagada(ctx, usuarioID, deudaID, categoriaID, fecha, medioPago)`:
1. Carga la deuda (debe ser del usuario y estar `pendiente`; si ya está pagada → `ErrInvalidInput`).
2. Valida que la categoría recibida sea de tipo `egreso` (del usuario o de sistema) vía `categorias`, y que `fecha` (opcional, vacía ⇒ hoy) tenga formato `YYYY-MM-DD`.
3. Si `categoriaID == 0` usa la categoría guardada en la deuda; si `medioPago` viene vacío usa el de la deuda. Delega en `transSvc.Create` para registrar el **egreso** por `monto_total` con la fecha indicada y esa categoría/forma de pago (reusa `FindOrCreate` de mes y la regla de mes cerrado → `ErrMesCerrado` si el mes cae en uno cerrado).
4. Marca la deuda como `pagada` en la BD.

El fragmento `DeudaPagoForm` (`pages_handler.go`) precarga la fecha del egreso:
- mes actual abierto → hoy;
- mes actual cerrado → una fecha en el **primer mes abierto** posterior (el que deja `Cerrar`) con un aviso en el modal. Así el pago nunca queda bloqueado por un mes cerrado.

Consecuencias (todas en `deuda_repo.go`):
- `SumMontoTotal` suma solo `estado != 'pagada'` → deja de contar en `pasivos_total`.
- `FindByRango` excluye `pagada` → no aparece en el feed.
- `FindByUsuarioID` / `FindByID` **sí** la devuelven → queda visible con badge "Pagada".
- El desglose de pasivos del balance (`pages_handler.go: BalancePage`) lista solo pendientes.

### 11.3 Autenticación (`internal/service/auth_service.go`)

- **Registro:** valida campos → hashea la contraseña con `bcrypt` (costo por defecto)
  → inserta en `usuarios` → emite JWT.
- **Login:** busca por email → compara el hash con `bcrypt.CompareHashAndPassword` →
  emite JWT.
- El hash bcrypt es **irreversible**: ni siquiera el server conoce la contraseña plana;
  solo puede verificar que el hash coincida.

---

## 12. Errores: del dominio al HTTP

Los errores de negocio viven en `internal/model/errors.go` como variables tipadas:

| Error de dominio | HTTP resultante | Caso |
|------------------|-----------------|------|
| `ErrNotFound` | 404 | no existe el recurso |
| `ErrUnauthorized` | 401 | email/contraseña mal |
| `ErrInvalidInput` | 400 | datos inválidos |
| `ErrEmailExiste` | 409 | email ya registrado |
| `ErrMesCerrado` | 409 | editar un mes cerrado |

El puente lo hace `handleServiceError` en `internal/handler/helpers.go`:

```go
case errors.Is(err, model.ErrMesCerrado):
    respondError(w, http.StatusConflict, "el mes está cerrado, no se puede modificar")
```

Así, el service devuelve errores de *dominio* (independientes de HTTP) y el handler
decide cómo exponerlos. Eso mantiene la lógica de negocio desacoplada de la web.

---

## 13. Guía para empezar a hacer cambios

### 13.1 Cómo correr el proyecto

```bash
# env.secrets (gitignored, no está en el repo) debe tener un JWT_SECRET real:
#   NEW="$(openssl rand -base64 48)" && sed -i '' "s|^JWT_SECRET=.*|JWT_SECRET=${NEW}|" env.secrets
set -a; source env.secrets; set +a  # cargar secretos locales (DATABASE_URL, JWT_SECRET...)
make db-init                  # crea la base (si falta) y aplica migrations/*.up.sql
make run                      # compila y corre en :8080 (la app NO migra sola)
```

### 13.2 Flujo típico para agregar una feature

Siguiendo el patrón del proyecto, para agregar algo nuevo (ej. "metas de ahorro"):

1. **`model/models.go`**: agregar la struct (`MetaAhorro` ya existe como modelo).
2. **`migrations/`** (`NNN_nombre.up.sql` + `.down.sql`): crear la tabla y correr `make migrate-up`.
3. **`internal/repository/`**: nuevo repo con métodos SQL (`Create`, `FindByID`,
   `FindByUsuarioID`, ...) siempre filtrando por `usuario_id`.
4. **`internal/service/`**: nueva service con las reglas de negocio y validaciones.
5. **`internal/handler/`**: nuevo handler (o métodos nuevos) que conecte HTTP → service.
6. **`cmd/server/main.go`**: instanciar repo → service → handler y registrar las rutas.
7. **`web/templates/`**: agregar la página y un link en `layout.html`.
8. **`internal/handler/pages_handler.go`**: agregar el método que renderiza la página.

> 💡 **Receta para no perderte:** "handler no toca SQL" y "repository no sabe de HTTP".
> Si un archivo mezcla ambas cosas, está mal separado.

### 13.3 Comandos útiles

```bash
make build     # compila a bin/server
make test      # go test ./... -v
go vet ./...   # análisis estático
```

### 13.4 Errores comunes de un dev Jr

| Problema | Explicación |
|----------|-------------|
| "Siempre responde 401" | El token expiró, o estás probando desde curl sin `Authorization: Bearer` |
| "No veo mis datos" | Todas las queries filtran por `usuario_id`; usá el userID que sale del JWT |
| "El form no hace nada" | Falta el header `HX-Request` → la app responde como JSON/redirect |
| "No puedo editar una transacción" | El mes está `cerrado` → es inmutable a propósito |

---

## Anexo A: Mapa archivo → responsabilidad

| Si querés tocar... | Archivo(s) |
|--------------------|-----------|
| Rutas y arranque | `cmd/server/main.go` |
| Variables de entorno | `internal/config/config.go`, `env.secrets` |
| Login / registro / JWT | `internal/service/auth_service.go`, `internal/handler/auth_handler.go`, `internal/middleware/auth.go` |
| Crear/editar transacciones | `transaccion_handler.go`, `transaccion_service.go`, `transaccion_repo.go` |
| Costos fijos | `costofijo_handler.go`, `costofijo_service.go`, `costofijo_repo.go` |
| Cierre de mes | `mes_handler.go`, `mes_service.go`, `mes_repo.go` |
| Dashboard / métricas | `dashboard_handler.go`, `dashboard_service.go` |
| Categorías | `categoria_handler.go`, `categoria_repo.go` |
| Páginas HTML | `pages_handler.go` + `web/templates/*.html` |
| Layout / estilos / HTMX | `web/templates/layout.html` |
| Errores de negocio | `internal/model/errors.go`, `handler/helpers.go:handleServiceError` |
