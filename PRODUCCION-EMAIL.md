# Producción: envío de emails (verificación de cuenta y recupero de contraseña)

La app ya tiene el código de envío implementado (`internal/service/mailer.go`):
SMTP sobre STARTTLS (587) o TLS implícito (465), con o sin autenticación, y dos
flujos que lo usan (verificación de cuenta + reset de contraseña). No hace falta
modificar código: todo se configura por variables de entorno.

Este documento cubre el modo local de prueba y la puesta en producción.

## Estado del código (resumen)

- `internal/service/mailer.go` — interfaz `Mailer` con dos implementaciones:
  - `smtpMailer`: envío SMTP real (587 STARTTLS / 465 TLS), autentica si `SMTP_USER` está seteado.
  - `logMailer`: modo dev sin `SMTP_HOST`; imprime el link por stdout.
- `internal/service/auth_service.go` — arma el link, lo envía y loguea `envío de email ... falló` si falla.
- `internal/config/config.go` — lee `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `MAIL_FROM`, `APP_BASE_URL`. `MAIL_FROM` es obligatoria si `SMTP_HOST` no está vacía.
- `docker-compose.yml` — `SMTP_HOST` está parametrizada (`${SMTP_HOST:-mailpit}`): por defecto usa el servicio `mailpit` del stack (dev).

## Proveedor transaccional: Brevo (producción)

### Datos de Brevo

| Variable | Valor Brevo |
|---|---|
| `SMTP_HOST` | `smtp-relay.brevo.com` |
| `SMTP_PORT` | `587` |
| `SMTP_USER` | el login SMTP de Brevo (ej. `b70c2b001@smtp-brevo.com`) — se muestra en **Settings → SMTP & API → SMTP** |
| `SMTP_PASS` | la **SMTP Key** (empieza con `xsmtpsib-...`) — no la API key ni la contraseña de cuenta |
| `MAIL_FROM` | dirección remitente verificada en **Inicio → Remitentes e IP → Remitentes** |
| `APP_BASE_URL` | URL pública de la app (ver tabla de pruebas abajo) |

> **Nota de seguridad:** la SMTP Key es un secreto como una contraseña. Nunca commitear `env.secrets` (está en `.gitignore`). Activá 2FA en la cuenta Brevo. Opcional: restringí la SMTP Key por IP del servidor en el panel.

### Verificar remitente (paso necesario antes de enviar)

1. Brevo dashboard → **Inicio → Remitentes e IP → Remitentes → + Add a sender**.
2. Cargar email (`agustin.zunda@gmail.com`) + nombre (`Optipay`) → guardar.
3. Brevo manda un mail de verificación a esa casilla → abrir → clickear **Confirm**.
4. La dirección debe quedar en **verde "Verificado"** en la lista.

### Generar SMTP Key

1. Brevo dashboard → **Avatar/perfil → Configuración → SMTP & API** (o directamente `https://app.brevo.com/settings/keys/smtp`).
2. Asegurate de estar en la pestaña **SMTP** (NO "API keys").
3. **Generate a new SMTP key** → nombre `optipay-prod` → configurar expiración (o "sin expiración") → **Generate**.
4. La key completa se muestra **una sola vez** → copiala ahí mismo.

> ⚠️ **API key ≠ SMTP key (trampa común).**
> Brevo tiene dos claves distintas:
> - **API key** (`xkeysib-...`): se usa en el header `api-key:` de la **API REST**. NO sirve para SMTP.
> - **SMTP key** (`xsmtpsib-...`): es la que va en `SMTP_PASS`. Se genera en la pestaña **SMTP** de `settings/keys/smtp`.
>
> Si cargás la API key en `SMTP_PASS`, el SMTP responde `535 5.7.8 Authentication failed` (aunque la API con esa misma clave funcione con HTTP 200). Síntoma: la API anda pero el SMTP rechaza la auth.

> **Bloqueo por IP (opcional):** si Brevo te lo pide, autorizá la IP de envío en **Avatar → Settings → Security → Authorized IPs** (`https://app.brevo.com/security/authorised_ips`) y activá el bloqueo para SMTP. Para SMTP no hay "fase de aprendizaje" automática: hay que autorizar manualmente los IPs **antes** de activarlo. Listá la IP de tu servidor de producción (además de la de dev).

## Estado actual (cómo se probó)

El envío con Brevo ya quedó funcionando y verificado:

- Autenticación SMTP confirmada con respuesta `235 Authentication succeeded`.
- Envío real de email de verificación (`"Confirmá tu cuenta en Optipay"`) completado sin errores.
- El servidor del stack compose (`app`) está corriendo healthy con la SMTP key de Brevo.

Queda pendiente: confirmar en el inbox real el link de verificación (cuando se registre un usuario con un email real) y definir la URL pública de producción para `APP_BASE_URL`.

## Dev local (prueba con Mailpit)

> El `env.secrets` actual del repo apunta a **Brevo real** (SMTP enviando de
> verdad). Para probar en local capturando mails, cambiá el bloque a Mailpit o
> levantá solo el servicio:

Todo el stack se levanta con:

```bash
docker compose --env-file env.secrets up -d --build
```

Bloque Mailpit (alternativa de dev para capturar mails sin enviarlos de verdad):

```bash
SMTP_HOST=localhost     # make run local; compose lo setea a `mailpit` por default
SMTP_PORT=1025
# SMTP_USER / SMTP_PASS: omitidas => sin AUTH (Mailpit no pide credenciales)
MAIL_FROM=no-reply@optipay.local
APP_BASE_URL=http://localhost:8080
```

- UI de Mailpit con los mails capturados: http://localhost:8025
- `docker compose logs -f app`: log del servidor; cualquier error de envío aparece ahí.

## Puesta en producción (envío real)

### Variables de producción en `env.secrets`

Reemplazar el bloque de Mailpit/dev con estos valores (Brevo):

```bash
SMTP_HOST=smtp-relay.brevo.com
SMTP_PORT=587
SMTP_USER=b70c2b001@smtp-brevo.com          # login SMTP de Brevo (Settings → SMTP & API → SMTP)
SMTP_PASS=xsmtpsib-...                       # SMTP key (NO la API key; ver advertencia arriba)
MAIL_FROM=agustin.zunda@gmail.com            # dirección verificada como remitente en Brevo
APP_BASE_URL=https://tudominio.com           # URL pública estable de la app
```

> **APP_BASE_URL es crítico:** el link del mail de verificación se arma con ese valor (`auth_service.go` hace `baseURL + "/api/auth/verificar?token=..."`). Si queda en `localhost`, el link no funciona desde ningún inbox. En el repo actual apunta a un túnel cloudflared temporal — hay que cambiarlo a la URL pública real antes de producción.

> **Sobre el túnel cloudflared actual:** el `APP_BASE_URL` en el repo apunta a un subdominio temporal de `trycloudflare.com`. Esto sirve **solo para la prueba** (exponer localhost al mundo). No dejarlo corriendo con datos reales: en producción usar la URL del hosting.

### Seguridad: claves de producción

- **`JWT_SECRET`**: generar uno nuevo aleatorio de 64 caracteres para producción. El que se usó en dev viajó en el repo/sesión y no debe reutilizarse.
- **`CORS_ORIGIN`**: configurar el origen real del sitio, no `*`.
- No hay cambios de código ni de migraciones.

### Sobre el deploy

- Con compose: `SMTP_HOST` ya es configurable desde `env.secrets` (parametrizado como `${SMTP_HOST:-mailpit}` en `docker-compose.yml`). Al desplegar en prod se puede dejar el servicio `mailpit` (inafecta por no recibir mails) o sacarlo; la app solo lo usa si `SMTP_HOST=mailpit`.
- Sin compose (`make run` / binario): reemplazar las variables de `env.secrets` directamente; requiere MySQL accesible desde el server.

### Checklist de prueba

1. Registrar un usuario nuevo → debe llegar "Confirmá tu cuenta en Optipay" al inbox real.
2. Click en el enlace → página "Email confirmado. Ya podés iniciar sesión.".
3. Login OK. Login antes de verificar → 409 `ErrEmailNoVerificado` (bloqueo correcto).
4. Pop-up de verificación → "Reenviar email" → llega reenvío con link nuevo.
5. `/forgot-password` → llega "Recuperá tu contraseña en Optipay" → reset → login con la nueva contraseña.
6. Revisar `docker compose logs -f app` o los logs del proceso: cualquier falla de envío se loguea.

## Recibir correos (futuro)

Hoy la app solo envía. No se necesita recepción para verificación/reset. Si más
adelante se quiere procesar mail entrante (p. ej. registrar gastos por mail), se
agrega un **inbound webhook** del proveedor (Brevo/SendGrid/Mailgun hacen POST a
un endpoint de la app con el mail parseado) o un cliente IMAP. Es una feature
nueva e independiente.