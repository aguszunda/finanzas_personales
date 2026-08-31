# Guía de despliegue: finanzas (optipay) — Go + MySQL

Guía paso a paso para poner en **producción** la app de finanzas personales
(monolito Go 1.24 + MySQL 8.4 + JWT + HTMX). El flujo de **envío de mails**
(verificación de cuenta y reset de contraseña) ya está implementado y verificado
con Brevo; acá se consolida cómo desplegar el servicio en un host real.

Complementa a `PRODUCCION-EMAIL.md` (que detalla solo la parte de email). Esta
guía cubre el despliegue completo del stack.

---

## 1. Resumen del stack

| Componente | Tecnología | Notas |
|---|---|---|
| App | Go 1.24 (binario compilado) | Dockerfile `golang:1.25-alpine` → binario |
| DB | MySQL 8.4 | volúmenes persistentes, `utf8mb4_unicode_ci` |
| Migraciones | golang-migrate v4.18.3 | corre una vez, la app NO migra al arrancar |
| API/Páginas | chi + HTMX + html/template | mismo binario sirve JSON + HTML |
| Auth | JWT HS256 | `JWT_SECRET` obligatorio (sin default) |
| Email | Brevo SMTP (`smtp-relay.brevo.com:587`) | verificación + reset |
| Dev mail | Mailpit (opcional, no en prod) | solo si `SMTP_HOST=mailpit` |

Todo el stack dev (app + db + migrate + mailpit) corre con un solo comando
(`docker compose`). Para producción reutilizamos compose pero apuntando la app a
un MySQL y un relay de mail reales.

---

## 2. Prerrequisitos en el host de producción

- **Docker + Docker Compose** (v2). El binario se construye dentro del contenedor
  (multi-stage `Dockerfile` con `golang:1.25-alpine`); no hace falta Go instalado.
- **Acceso a internet** desde el host: para `docker build` (pull de imágenes) y
  para el SMTP de Brevo (`smtp-relay.brevo.com:587`) saliente.
- (Recomendado) Dominio + DNS apuntando a la IP del host, para tener HTTPS.

---

## 3. Variables de producción (`env.secrets`)

> `env.secrets` está en `.gitignore` — **no se commitea**. Se carga con
> `docker compose --env-file env.secrets ...`.

Bloque mínimo de producción (reemplazar valores de dev):

```bash
PORT=8080

# --- MySQL (interno al stack compose) ---
MYSQL_DATABASE=finanzas
MYSQL_USER=finanzas
MYSQL_PASSWORD=<PASS_FUERTE>
MYSQL_ROOT_PASSWORD=<PASS_FUERTE_ROOT>

# --- App ---
JWT_SECRET=<NUEVO_SECRET_64_CARACTERES>   # openssl rand -hex 32
JWT_EXPIRATION_HOURS=72
CORS_ORIGIN=<origen-real-ej https://finanzas.midominio.com>
LOG_LEVEL=info

# --- Email (Brevo) ---
SMTP_HOST=smtp-relay.brevo.com
SMTP_PORT=587
SMTP_USER=b70c2b001@smtp-brevo.com        # login SMTP de Brevo (Settings → SMTP & API → SMTP)
SMTP_PASS=<SMTP_KEY_xsmtpsib-...>         # SMTP key, NO la API key xkeysib-...
MAIL_FROM=agustin.zunda@gmail.com         # remitente verificado en Brevo
APP_BASE_URL=<URL-publica-ej https://finanzas.midominio.com>  # CRÍTICO: arma el link del mail
```

> **APIs de referencia (solo en `docker-compose.yml`, no en `env.secrets`):**
> `DATABASE_URL` se construye en compose desde `MYSQL_*` (no se setea en
> `env.secrets`). La app local con `make run` usa `DATABASE_URL` apuntando al
> MySQL local directo.

---

## 4. Pasos de despliegue

### 4.1 Levantar el stack con compose

```bash
docker compose --env-file env.secrets up -d --build
```

Este comando:
1. Construye la imagen de la app (multi-stage Go).
2. Levanta MySQL (`db`).
3. Espera a que MySQL esté healthy.
4. Corre las migraciones (`migrate` → golang-migrate `up`).
5. Recién entonces arranca `app`.

Verificar:
```bash
docker compose ps                 # app + db deben estar "healthy"
curl -s http://localhost:8080/health   # → HTTP 200
docker compose logs -f app        # logs en vivo; errores de email se loguean acá
```

### 4.2 Usar SQLite/MySQL externo (opcional)

Si en lugar de la DB del stack querés usar un MySQL **externo** (p. ej. un
managed service tipo Railway/Render/AWS RDS), seteá en `env.secrets` un
`DATABASE_URL` que se pase directo al contenedor. El `docker-compose.yml` arma
`DATABASE_URL` desde `MYSQL_*`; para un externo hay que sobreescribir la variable
en el bloque `app.environment` (por ejemplo `DATABASE_URL: <dsn-externo>`) o
levantar solo `app` con un DSN apuntando afuera. En ese caso, corré las
migraciones vos con golang-migrate contra esa base (ver `make migrate-up`).

### 4.3 Configurar HTTPS (recomendado)

En producción vas a necesitar HTTPS para que los links del mail y los cookies
`Secure` funcionen bien. Opciones:
- **Reverse proxy** (Caddy, Traefik, Nginx) frente al puerto 8080 con Let's
  Encrypt automático.
- O una capa TLS del propio hosting (Railway/Render dan HTTPS propio).

El `APP_BASE_URL` debe reflejar el esquema https final (la app no tiene TLS
propio; confía en el proxy).

---

## 5. Flujo de email (nosotros ya lo dejamos armado)

El envío por **Brevo real** quedó funcionando y verificado:

- Autenticación SMTP: `235 Authentication succeeded`.
- Envío del mail de verificación ("Confirmá tu cuenta en Optipay"): sin errores.
- `env.secrets` apunta a Brevo (`SMTP_HOST=smtp-relay.brevo.com`, `SMTP_PORT=587`,
  `SMTP_PASS` = SMTP key).

> **Trampa conocida:** Brevo tiene **API key** (`xkeysib-...`, API REST) y
> **SMTP key** (`xsmtpsib-...`, SMTP). En `SMTP_PASS` va la **SMTP key**. Si cargás
> la API key, el SMTP responde `535 5.7.8 Authentication failed`.
> Detalle completo en `PRODUCCION-EMAIL.md`.

> **APP_BASE_URL**: el link del mail (`/api/auth/verificar?token=...`) se arma con
> este valor. Actualmente en el repo apunta a un **túnel cloudflared temporal**.
> Antes de dar producción, apagalo (`docker stop temp-tunnel`) y cambiá
> `APP_BASE_URL` a la URL pública real.

---

## 6. Checklist de validación post-deploy

1. `/health` responde `200`.
2. **Registrar** usuario → llega "Confirmá tu cuenta en Optipay" al inbox real.
3. Click en el link → "Email confirmado. Ya podés iniciar sesión."
4. **Login** OK. Login antes de verificar → 409 `ErrEmailNoVerificado`.
5. **Reenviar email** desde el pop-up → llega reenvío con link nuevo.
6. `/forgot-password` → llega "Recuperá tu contraseña" → reset → login con nueva pass.
7. Revisar `docker compose logs -f app`: cualquier falla de envío se loguea
   (`envío de email ... falló`).

---

## 7. Mantenimiento

### Migraciones nuevas
1. Agregar `migrations/NNN_nombre.up.sql` + `NNN_nombre.down.sql` (bump número).
2. `docker compose --env-file env.secrets run --rm migrate` para aplicar.

### Backup de la base
```bash
docker compose exec -T db sh -c 'exec mysqldump -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE"' > backup_$(date +%F).sql
```

### Actualizar imagen
```bash
docker compose --env-file env.secrets up -d --build
```

---

## 8. Notas de seguridad

- Mantener `env.secrets` fuera del control de versiones (✓ `.gitignore`).
- `JWT_SECRET` nuevo y largo para producción (no reutilizar el de dev).
- `CORS_ORIGIN` = origen real, no `*`.
- Activá 2FA en la cuenta Brevo; opcional, restringí la SMTP key por IP del host.
- El túnel cloudflared es **solo para pruebas** → no dejarlo corriendo con datos reales.
