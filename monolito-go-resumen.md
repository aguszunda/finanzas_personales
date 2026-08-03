# Monolito Go + Frontend: Guía de Arquitectura y Stack Tecnológico

## 1. Monolito vs Microservicios

| Aspecto | Monolito | Microservicios |
|---------|----------|----------------|
| Equipo | Pequeño (<10 devs) | Varios equipos independientes |
| Complejidad operativa | Baja | Alta (red, fallos, despliegue) |
| Deploy | 1 binario | N servicios independientes |
| Escalabilidad | Vertical / réplicas completas | Horizontal granular por servicio |
| Time-to-market | Rapido al inicio | Lento (infraestructura) |
| Go recommendation | `cmd/` + `internal/` | Modulos separados |

**Conclusion**: empieza como monolo bien estructurado. Extrae microservicios solo cuando el monolito realmente duele (Fowler, *Production-Ready Microservices*).

---

## 2. Estructura de Directorios (Go Standard Layout)

```
mi-app/
├── cmd/
│   └── server/
│       └── main.go              # Entry point, HTTP server
├── internal/
│   ├── handler/                  # Capa HTTP (request/response)
│   ├── service/                  # Logica de negocio (interfaces)
│   ├── repository/              # Acceso a datos (interfaces)
│   ├── model/                   # Tipos de dominio compartidos
│   └── middleware/              # Auth, logging, CORS, etc.
├── web/                         # Frontend
│   ├── templates/               # html/template (si usas HTMX)
│   ├── static/                  # CSS, JS, imagenes
│   └── dist/                    # Build de SPA (si aplica)
├── migrations/                  # SQL migrations
├── go.mod
├── Dockerfile
└── Makefile
```

### Pipeline de dependencias (Clean Architecture)

```
[Handler] → [Service interface] → [Service impl] → [Repository interface] → [Repository impl]
                                                                                    │
                                                                              [Postgres/MySQL]

Regla: cada capa solo conoce la inmediatamente inferior, siempre via interfaces.
```

---

## 3. Stack Tecnologico Recomendado

```
┌─────────────────────────────────────────────┐
│               FRONTEND                       │
│  Opcion A: HTMX + html/template             │  ← Sin JS framework
│  Opcion B: Svelte/Vue/React + //go:embed    │  ← SPA embebido en binario
├─────────────────────────────────────────────┤
│               BACKEND (Go)                   │
│  chi (router)   validator   slog / zerolog  │
│  go-sql-driver/mysql     golang-migrate   asynq      │
│  golang-jwt     OpenTelemetry               │
├─────────────────────────────────────────────┤
│              INFRAESTRUCTURA                │
│  Docker multi-stage   MySQL   Redis      │
└─────────────────────────────────────────────┘
```

### Detalle por capa

| Capa | Tecnologia | Por que |
|------|-----------|---------|
| **Router** | [chi](https://github.com/go-chi/chi) | 100% compatible con `net/http`, ~1000 LOC, middleware composable, 22.6k ⭐ |
| **DB Driver** | [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) | Driver MySQL oficial, `database/sql`, sin ORM |
| **Queries** | [sqlc](https://github.com/sqlc-dev/sqlc) | Escribis SQL, genera Go type-safe. Zero overhead de ORM |
| **Migraciones** | [golang-migrate](https://github.com/golang-migrate/migrate) o [goose](https://github.com/pressly/goose) | SQL puro, maduros, simples |
| **Validacion** | [go-playground/validator](https://github.com/go-playground/validator) | Estandar de facto, tags en structs |
| **Logging** | `log/slog` (stdlib Go 1.21+) o [zerolog](https://github.com/rs/zerolog) | Sin dependencias o zero allocations |
| **Auth** | [golang-jwt/jwt](https://github.com/golang-jwt/jwt) | JWT stateless, facil de escalar |
| **Config** | [envconfig](https://github.com/kelseyhightower/envconfig) o `os.Getenv` | KISS, 12factor, variables de entorno |
| **Cache / Cola** | [asynq](https://github.com/hibiken/asynq) (Redis) | Cola de tareas con dashboard. Alternativa simple: gorutinas |
| **Testing** | `testing` + [testify](https://github.com/stretchr/testify) + [go-sqlmock](https://github.com/DATA-DOG/go-sqlmock) | Sin frameworks magicos |
| **Frontend** | [HTMX](https://htmx.org) + `html/template` | HTML desde el servidor, sin JS framework, sin CORS, un solo binario |
| **Contenedor** | Docker multi-stage | `golang:alpine` → `scratch`, binario ~15MB |

---

## 4. Frontend: Dos Caminos

### Opcion A: HTMX + html/template (Recomendada)

```
Browser ←─── HTML ─────→ Go Server (chi + templates)
    ↑                         │
    │  Respuesta: HTML        │  Logica en Go, no en JS
    │  (no JSON)              │
    └── hx-get, hx-post ──────┘
```

- Un solo binario: frontend y backend viajan juntos
- Zero CORS, deploy atomico
- `html/template` incluye auto-escaping (XSS prevention)
- Sin bundle, sin build step de frontend

### Opcion B: SPA embebido (React / Svelte / Vue)

```go
//go:embed web/dist/*
var webFS embed.FS

func main() {
    r := chi.NewRouter()
    r.Handle("/*", http.FileServerFS(webFS))
    // Rutas API en /api/*
}
```

- Usar cuando se necesita interaccion rich-client real
- Build de frontend separado, resultado embebido en el binario Go

---

## 5. Ejemplo `go.mod` y dependencias minimas

```
module github.com/tuusuario/miapp

go 1.24

require (
    github.com/go-chi/chi/v5       v5.x
    github.com/go-sql-driver/mysql v1.x
    github.com/go-playground/validator/v10  v10.x
    github.com/golang-jwt/jwt/v5    v5.x
    github.com/hibiken/asynq        v0.x
    github.com/golang-migrate/migrate/v4  v4.x
    github.com/rs/zerolog           v1.x    // o usar log/slog (stdlib)
)
```

---

## 6. Middleware: Implementación y Ubicación

### Middlewares globales (router-level, se ejecutan en todas las rutas)

```go
package main

import (
    "net/http"
    "github.com/go-chi/chi/v5"
    chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func main() {
    r := chi.NewRouter()
    r.Use(chiMiddleware.RequestID)        // X-Request-ID
    r.Use(chiMiddleware.RealIP)           // IP real tras proxy
    r.Use(chiMiddleware.Logger)           // logging de requests
    r.Use(chiMiddleware.Recoverer)        // panic → 500
    r.Use(chiMiddleware.Timeout(30 * time.Second)) // timeout global

    r.Get("/health", healthHandler)
    // ...
}
```

### Middlewares por grupo de rutas

```go
// Grupo público: no requiere autenticación
r.Group(func(r chi.Router) {
    r.Use(corsMiddleware)          // solo si SPA
    r.Use(htmxMiddleware)          // detecta HX-Request
    r.Get("/", home)
    r.Get("/products", listProducts)
})

// Grupo autenticado: requiere JWT válido
r.Group(func(r chi.Router) {
    r.Use(jwtAuth)                 // valida token, inyecta userID en context
    r.Post("/products", createProduct)
    r.Put("/products/{id}", updateProduct)
    r.Delete("/products/{id}", deleteProduct)
})

// Grupo admin: requiere JWT + rol admin
r.Group(func(r chi.Router) {
    r.Use(jwtAuth)
    r.Use(requireRole("admin"))
    r.Get("/admin/users", listUsers)
})
```

### Implementación de middlewares custom

Cada middleware en su propio archivo dentro de `internal/middleware/`, siguiendo la firma `func(http.Handler) http.Handler`:

```go
// internal/middleware/jwt.go
package middleware

import (
    "context"
    "net/http"
    "strings"
    "github.com/golang-jwt/jwt/v5"
)

type contextKey string
const UserIDKey contextKey = "userID"

func JWTAuth(secret []byte) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tokenStr := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            if tokenStr == "" {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
                return secret, nil
            })
            if err != nil || !token.Valid {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            claims := token.Claims.(jwt.MapClaims)
            ctx := context.WithValue(r.Context(), UserIDKey, claims["sub"])
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

```go
// internal/middleware/htmx.go
package middleware

import "net/http"

func DetectHTMX(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        isHTMX := r.Header.Get("HX-Request") == "true"
        ctx := context.WithValue(r.Context(), "htmx", isHTMX)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

```go
// internal/middleware/cors.go
package middleware

import "net/http"

func CORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### Cuándo cortar el flujo vs pasar al siguiente

| Situación | Acción |
|-----------|--------|
| Token inválido/faltante | `http.Error(w, "unauthorized", 401)` — **corta** |
| Rate limit excedido | `http.Error(w, "too many requests", 429)` — **corta** |
| Request válido | `next.ServeHTTP(w, r)` — **pasa** |
| Logging | `next.ServeHTTP` antes y después medir tiempo — **pasa** |
| CORS preflight (OPTIONS) | Responder 204 sin `next` — **corta** |
| Inyectar datos en context | Setear context, `next.ServeHTTP` — **pasa** |

### Pipeline completo del request

```
┌──────────┐   ┌──────────┐   ┌───────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐
│ RequestID │ → │ Recoverer │ → │ CORS  │ → │ JWTAuth  │ → │   Handler│ → │  Service  │
└──────────┘   └──────────┘   └───────┘   └──────────┘   └──────────┘   └──────────┘
                                                                                │
                                                                          ┌─────▼─────┐
                                                                          │ Repository │
                                                                          └───────────┘
```

---

## 7. Principios Clave

1. **`internal/`** protege tu codigo del exterior (Go compiler-enforced)
2. **`cmd/`** permite agregar workers, CLIs o crons sin tocar el server
3. **Interfaces** en service/repository desde el dia 1 (facilita testear y futuro split a microservicios)
4. **Un solo binario** con `//go:embed` para el frontend
5. **sqlc > ORM**: escribis SQL, obtenes codigo type-safe, cero runtime overhead
6. **12factor app**: config por entorno, logging a stdout, stateless

---

## 8. Referencias

- [Go Project Layout (Oficial)](https://go.dev/doc/modules/layout)
- [chi Router](https://github.com/go-chi/chi)
- [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql)
- [sqlc](https://github.com/sqlc-dev/sqlc)
- [HTMX](https://htmx.org)
- [Production-Ready Microservices - Susan Fowler](Microservicios_Resumen.md)
