# Guía de despliegue: finanzas (optipay) en Back4app Containers + Aiven — 100% gratuito, sin tarjeta

Guía paso a paso para desplegar la app **optipay** (monolito Go + MySQL + JWT + HTMX)
en **hosting gratuito por siempre, sin tarjeta de crédito**, dividiendo el stack en
dos servicios libres:

- **App Go** → **Back4app Containers** (free tier, deploy de un `Dockerfile` desde GitHub).
- **Base MySQL** → **Aiven** (free tier, MySQL gestionado, sin caducidad).

> **Por qué es necesario dividir:** ninguna plataforma free-sin-tarjeta ofrece a la
> vez app + MySQL en un mismo plan. Back4app free despliega contenedores pero no
> incluye MySQL; Aiven free sí ofrece MySQL. Esta guía junta ambos y arma a mano el
> DSN MySQL (`@tcp(...)`) que la app exige. **No toca una sola línea de código Go.**

Complementa a `GUIA-desplegar-finanzas-go-mysql.md` (despliegue con Docker Compose
propio) y a `PRODUCCION-EMAIL.md` (email Brevo). Esta guía es la variante de
**cero costo / cero tarjeta**.

> Nota de historial: la variante original de esta guía usaba **Koyeb** como host de la
> app, pero quedó descartado (2026-09-01): la plataforma fue adquirida por Mistral y
> el free tier dejó de estar abierto a usuarios nuevos. Back4app Containers es el
> reemplazo equivalente: $0 sin tarjeta, deploy por `Dockerfile` desde GitHub.

> **Estado: DESPLEGADO EN PRODUCCIÓN (2026-09-01).** La app corre en
> `https://optipay-7zvs7tgt.b4a.run`, con DB Aiven migrada, email Brevo funcionando
> (SMTP 587 saliendo bien desde el contenedor) y conexión a la DB desde MySQL
> Workbench verificada.

---

## 1. Resumen del stack en la nube

| Componente | Proveedor | Plan | Costo | Tarjeta |
|---|---|---|---|---|
| App (Go) | Back4app Containers | Free | $0 | No (explícito: "no credit card required") |
| DB (MySQL) | Aiven | Free tier | $0 | No |
| Email | Brevo | SMTP 587 | $0 (cuota gratuita) | Ya configurado |
| Migraciones | golang-migrate CLI | una vez, desde tu PC | — | — |

### Límites del free tier (a tener presentes)

**Back4app Containers Free:**
- 0.25 CPU / 256 MB RAM / 100 GB transferencia / CPU compartida / región USA.
- 1 web/container app por plan gratuito.
- **Deploy por `Dockerfile`** desde GitHub (no necesita docker-compose).
- **URL pública `https://<app>-<hash>.b4a.run` con TLS (HTTPS) automático.**
- **No pide tarjeta** (confirmado en su FAQ de pricing).
- **Egress de salida:** Back4app corre los contenedores sobre AWS. AWS por defecto
  bloquea el puerto **25**, pero **permite 465 y 587** (SMTP Submission con TLS).
  Con `SMTP_PORT=587` hacia Brevo debería funcionar; **se valida empíricamente**
  tras el deploy enviando un email real (ver Paso 4).

**Aiven MySQL free:**
- 1 GB storage, 1 GB RAM, 1 CPU; backups diarios; sin caducidad.
- Host público `*.f.aivencloud.com`, puerto no estándar (p. ej. `:15639`).
- **Requiere TLS obligatorio**, pero firma el certificado con su **propia CA**
  (no está en el trust store del sistema ni en `ca-certificates` de Alpine). Por eso
  el DSN de la app debe usar `tls=skip-verify`: la conexión queda **cifrada**, solo
  omite la validación de identidad del certificado. Es la vía sin tocar código
  (go-sql-driver no registraría un CA custom sin modificar `main.go`).
- La DB se apaga tras un período de inactividad (se avisa por mail; hobby OK).
- Usuario por defecto `avnadmin`, base `defaultdb`.

---

## 2. DSN de la app para Aiven (la clave del truco)

La app (`internal/config/config.go`) exige una DSN MySQL que contenga `@tcp(...)`.
Aiven entrega host/puerto/usuario/base; hay que armar el DSN con el formato que la
app ya usa, con `tls=skip-verify` (Aiven usa CA propia; la conexión queda cifrada
igual, solo no valida la identidad del cert — es la forma sin tocar código):

```bash
export DATABASE_URL='AVN_USER:AVN_PASSWORD@tcp(HOST.f.aivencloud.com:PUERTO)/defaultdb?parseTime=true&multiStatements=true&charset=utf8mb4&loc=Local&tls=skip-verify'
```

> La **base Aiven ya está creada y migrada** (ver Paso 0 y Paso 1). El DSN real de
> este proyecto es:
> ```
> avnadmin:@tcp(finanzas-db-agustin-7da4.e.aivencloud.com:15639)/defaultdb?parseTime=true&multiStatements=true&charset=utf8mb4&loc=Local&tls=skip-verify
> ```
> (completar la contraseña `avnadmin`; no se guarda en el repo).

Este DSN:
- Contiene `@tcp(host:puerto)` → pasa la validación de `config.Load` (config.go:77).
- `parseTime=true` → `DATETIME` como `time.Time` (igual que dev/compose).
- `multiStatements=true` → migraciones con múltiples statements.
- `charset=utf8mb4` + `loc=Local` → igual que el resto del stack.
- `tls=skip-verify` → conexión cifrada ignorando la firma de la CA propia de Aiven.

---

## 3. Paso 0: crear la base en Aiven (una vez) — YA HECHO

> Esta base ya existe, migrada y verificada. Solo se documenta por si hay que
> recrearla desde cero.

1. Crear cuenta en [console.aiven.io](https://console.aiven.io) (GitHub/Google, sin tarjeta).
2. **Create service → MySQL** → plan **Free (hobbyist)** → crear.
   - No se elige región en el free tier (la asigna el proveedor).
3. Esperar a que el estado pase a **Running**.
4. En la pestaña **Overview**, anotar (o copiar de "Quick connect"):
   - `HOST` (algo como `nombre-proyecto-NNNN.f.aivencloud.com`).
   - `PUERTO` (un puerto alto, p. ej. `15639`).
   - Usuario `avnadmin` y su password (se revelan en "Service settings"/credenciales).
   - Base por defecto `defaultdb`.
5. Valor de ejemplo:
   ```
   DATABASE_URL='avnadmin:PASS@tcp(HOST.f.aivencloud.com:PUERTO)/defaultdb?parseTime=true&multiStatements=true&charset=utf8mb4&loc=Local&tls=skip-verify'
   ```

---

## 4. Paso 1: migrar la base (una vez, desde tu PC) — YA HECHO

> Las 7 migraciones (001..007) ya se aplicaron a Aiven y se verificó el schema
> (`SHOW TABLES` → categorias, costos_fijos, deudas, meses, schema_migrations,
> transacciones, usuarios). Solo se documenta el procedimiento por si hay que
> repetirlo tras una migración nueva.

La app **no migra al arrancar** (por diseño, AGENTS.md). Como Back4app free es un
solo contenedor (sin tarea `migrate` separada), se aplican las migraciones **una
sola vez** contra Aiven con golang-migrate desde tu máquina:

```bash
export DATABASE_URL='avnadmin:PASS@tcp(HOST.f.aivencloud.com:PUERTO)/defaultdb?parseTime=true&multiStatements=true&charset=utf8mb4&loc=Local&tls=skip-verify'
make migrate-up
```

Verificar:
```bash
mysql --user avnadmin --password=PASS --host HOST.f.aivencloud.com --port PUERTO defaultdb -e "SHOW TABLES;"
```

> **Gotcha local:** el toolchain de Go local está roto por un GOROOT desalineado en
> `~/.bashrc:122` (`compile: version "go1.21.6" does not match go tool version
> "go1.26.7"`), así que `make migrate-up` (que usa `go run`) **falla** en esta
> máquina. Workaround: usar el binario pre-compilado de golang-migrate v4.18.3 en
> `/tmp/opencode/migrate` (incluye driver mysql). Si se pierde, se re-descarga:
> ```
> MIGRATE_VERSION=v4.18.3
> curl -L -o /tmp/opencode/migrate \
>   "https://github.com/golang-migrate/migrate/releases/download/v${MIGRATE_VERSION}/migrate.linux-amd64.tar.gz"
> tar -xzf <descarga> -C /tmp/opencode/ migrate
> /tmp/opencode/migrate -source file://migrations -database "mysql://$(DATABASE_URL)" up
> ```

### Conexión a la DB desde MySQL Workbench (verificado)

```
Connection Method: Standard (TCP/IP)
Hostname:          finanzas-db-agustin-7da4.e.aivencloud.com
Port:              15639
Username:          avnadmin
Password:          <password de avnadmin>
Default Schema:    defaultdb
SSL tab:           Require / Skip Verification   (CA de Aiven no está en el trust store)
```

Aiven exige TLS; elegir **Skip Verification** en la pestaña SSL de Workbench evita el
error `certificate signed by unknown authority`.

---

## 5. Paso 2: preparar el deploy de la app en Back4app Containers

La app ya trae un `Dockerfile` multi-stage (`golang:1.25-alpine` → binario → `alpine`
con `ca-certificates` y `wget`). Se reutiliza tal cual. (Los templates/static/images
van embebidos en el binario vía `go:embed`, así que el `COPY --from=builder
/app/web/templates /web/templates` del Dockerfile es redundante pero inofensivo).

1. Crear cuenta en [back4app.com](https://www.back4app.com/signup) (GitHub/Google, sin tarjeta).
2. Instalar la app de GitHub **"Back4App Containers"** en el repo de optipay
   (settings del repo → Integrations → GitHub Apps, o el flujo que te guíe el dashboard).
3. En el dashboard de Back4app → **Containers** → **Create App**:
   - **App name**: `optipay` (o similar).
   - **Branch**: `main`.
   - **Root**: `/` (el `Dockerfile` está en la raíz).
   - **Auto deployment**: ON (redeploy automático en cada push).
4. Back4app detecta el `Dockerfile` en la raíz y lo usa para construir.
5. Back4app asigna el **puerto de escucha** vía la variable `PORT`; la app ya lee
   `PORT` (default 8080) → no hace falta mapeo manual.
6. Al terminar, Back4app te da una **URL pública** `https://<app>-<hash>.b4a.run`
   con HTTPS automático. Esa es tu `APP_BASE_URL`.

> **Deploy real de este proyecto:** `https://optipay-7zvs7tgt.b4a.run` (la URL la
> asigna Back4app, formato `https://<app>-<hash>.b4a.run`).

---

## 6. Paso 3: variables de entorno en Back4app

En Back4app → tu app → **Settings → Environment variables / Secrets**, definir:

```
PORT=8080
DATABASE_URL=<EL DSN CON @tcp(...) y tls=skip-verify del paso 0>
JWT_SECRET=<NUEVO_SECRET_64_CARACTERES>      # openssl rand -hex 32
JWT_EXPIRATION_HOURS=72
CORS_ORIGIN=<https://<app>-<hash>.b4a.run>
LOG_LEVEL=info

# Email (Brevo) — se valida que 587 NO esté bloqueado en Back4app
SMTP_HOST=smtp-relay.brevo.com
SMTP_PORT=587
SMTP_USER=b70c2b001@smtp-brevo.com
SMTP_PASS=<SMTP_KEY_xsmtpsib-...>            # SMTP key, NO la API key
MAIL_FROM=agustin.zunda@gmail.com
APP_BASE_URL=https://<app>-<hash>.b4a.run
```

> `APP_BASE_URL` es **crítico**: arma el link del mail de verificación y de reset de
> contraseña (`/api/auth/verificar?token=...`). Debe apuntar a la URL pública HTTPS
> que te dio Back4app (`https://<app>-<hash>.b4a.run`, sin `/login` ni slash final).

---

## 7. Paso 4: verificar el despliegue

> Este paso está **COMPLETADO** en producción (2026-09-01): todos los puntos
> siguientes fueron verificados contra `https://optipay-7zvs7tgt.b4a.run`. Se dejan
> los pasos documentados por si hay que repetir la validación tras un redeploy.

1. Abrir la URL pública de Back4app → `https://<app>-<hash>.b4a.run/health` → `200`.
2. **Registrar** usuario → debe llegar "Confirmá tu cuenta en Optipay" al inbox real
   (**valida que Brevo por 587 funciona desde el contenedor de Back4app**). Si el
   mail no llega, mirar los logs del contenedor por `envío de email ... falló`
   (timeout de conexión a 587 = puerto bloqueado).
3. Click en el link → "Email confirmado. Ya podés iniciar sesión."
4. **Login** OK. Login sin verificar → `409 ErrEmailNoVerificado`.
5. `/forgot-password` → llega "Recuperá tu contraseña" → reset → login.
6. Revisar logs del contenedor en Back4app (panel **Running Logs / Monitoring**).

---

## 8. Checklist de validación

> Estado real al 2026-09-01: todo verificado en producción.

- [x] `DATABASE_URL` contiene `@tcp(...)` y `tls=skip-verify`.
- [x] Base Aiven migrada (001..007) — verificado.
- [x] App desplegada desde GitHub con el `Dockerfile` de la raíz, puerto vía `PORT`.
- [x] Health check `/health` = 200 en Back4app.
- [x] Variables cargadas en Back4app (secretos), incluido `APP_BASE_URL` HTTPS.
- [x] Email de verificación real llega (Brevo 587 desde el contenedor de Back4app).
- [x] Reset de contraseña funciona (segundo email real).
- [x] Conexión a la DB desde MySQL Workbench (TLS / Skip Verification).

---

## 9. Mantenimiento

### Migración nueva (schema)
Aplicarla la primera vez en local contra Aiven con `make migrate-up` (igual que el
Paso 1), o con el binario standalone si el toolchain local sigue roto. La app
arranca sin migrar, así que no hay riesgo de arrancar con schema incompleto (sí de
error SQL si las tablas faltan).

### Backup
Aiven free hace **backups diarios** automáticos (7 días de retención).

### Actualizar la app
`git push` a la rama conectada → el **auto-deploy** de Back4app rebuild + redeploy
automáticamente, sin downtime.

### Idle / cold start
Back4app free no anuncia scale-to-zero explícito para Containers, pero al ser free
puede haber inactividad del contenedor; el primer request tras una pausa puede tardar
unos segundos (aceptable para uso personal). Aiven free también puede apagarse tras
inactividad; se despierta con la próxima conexión.

---

## 10. Notas y advertencias

- **SMTP 587 no está garantizado al 100% por documentación:** Back4app corre sobre
  AWS, y AWS solo bloquea el puerto 25 por defecto (no 465/587), así que debería
  funcionar, pero es el único punto que hay que **validar empíricamente** (Paso 4).
  Si 587 estuviera bloqueado, la salida sin tocar código es imposible; la alternativa
  sería migrar el envío a la API REST HTTPS de Brevo (puerto 443, siempre abierto),
  lo cual **sí** implicaría tocar el código del service de email.
- **Aiven free se apaga tras inactividad**: es un servicio de hobby; si necesitás
  disponibilidad 24/7 garantizada hay que pasar a plan de pago.
- Mantené `JWT_SECRET` y las credenciales de Aiven fuera del repo: en **secrets** de
  Back4app, nunca como variable de entorno plana ni en git.
- `env.secrets` del repo **no aplica** a Back4app (Back4app se configura en su
  dashboard); no hace falta subir credenciales ahí.
