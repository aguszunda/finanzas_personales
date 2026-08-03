# Análisis: App de Control de Costos Personales / Pymes

## Perfil: Asesor Financiero Experto

---

## 1. Problema de Negocio

Personas y pequeños negocios no tienen visibilidad real de a dónde se va su dinero. El 65% de las personas no sabe cuánto gastó el mes pasado. Las herramientas existentes (Excel, apps contables) son demasiado complejas (contabilidad de doble entrada) o demasiado simples (solo trackean gastos sin contexto).

**Oportunidad:** Una app que hable en lenguaje humano (gané, gasté, me sobró, invertí) pero que tenga la rigurosidad suficiente para imprimir un balance y usarlo para tomar decisiones.

---

## 2. Conceptos Financieros Clave (Traducción a Software)

### Para el usuario — Lenguaje natural

| Término usuario | Concepto financiero | Definición |
|----------------|-------------------|------------|
| "Lo que gané" | **Ingresos** | Todo el dinero que entra (sueldo, freelance, ventas, honorarios) |
| "Lo que gasté" | **Egresos / Costos** | Todo el dinero que sale, categorizado |
| "Plata fija que siempre pago" | **Costos Fijos** | Alquiler, servicios, suscripciones, cuotas fijas |
| "Gastos variables" | **Costos Variables** | Comida, salidas, compras no planificadas |
| "Lo que me queda" | **Superávit / Resultado Neto** | Ingresos - Egresos |
| "Lo que ahorré" | **Ahorro** | Dinero reservado, no gastado. Puede estar en una meta específica |
| "Lo que invertí" | **Inversión** | Dinero puesto a trabajar (plazos fijos, cedears, crypto, etc.) |
| "Mi deuda" | **Pasivos** | Tarjetas de crédito, préstamos, cuotas pendientes |
| "Lo que tengo" | **Patrimonio** | Activos - Pasivos. Lo que realmente valgo hoy |
| "Presupuesto" | **Presupuesto / Techo** | Límite que me autoimpongo por categoría |

### Reglas de negocio fundamentales

```
Patrimonio = Activos - Pasivos (instantáneo)
Resultado Neto = Ingresos - Egresos (período)
Tasa de Ahorro = Ahorro / Ingresos (objetivo: >15%)
Cobertura de Gastos = Ahorro Total / Gastos Mensuales (meses de reserva)
```

---

## 3. Perfiles de Usuario

### Perfil A: Persona física ("Usuario Común")
- Edad: 20-50 años
- Ingresos: 1-3 fuentes (sueldo, changas, monotributo)
- Objetivo: entender a dónde se va la plata, dejar de llegar a fin de mes ajustado
- Capacidad técnica: baja-media
- Dispositivo: celular principalmente

### Perfil B: Pyme / Emprendedor chico
- Facturación baja-media, 1-5 empleados o solo
- Objetivo: saber si el negocio es rentable, separar cuentas personales de las del negocio
- Necesita: balance imprimible para presentar a contador o para ver ganancia real
- Capacidad técnica: media

### Perfil C: Familia / Hogar compartido
- 2+ personas que comparten gastos (pareja, roommates)
- Objetivo: dividir gastos comunes, saber cuánto aporta cada uno
- Necesita: balances compartidos

---

## 4. Funcionalidades Requeridas (Priorizadas)

### MVP (Fase 1)

#### 4.1 Registro y autenticación multi-usuario
- Login con email + contraseña (JWT)
- Perfil con nombre, moneda predeterminada (ARS, USD, etc.)
- Sesión por usuario, datos aislados

#### 4.2 Gestión de transacciones (ingresos y egresos)
- Registrar transacciones individuales:
  - Tipo: ingreso / egreso
  - Monto
  - Fecha
  - Categoría (💰 Ingresos: Sueldo, Freelance, Ventas, Otros | 💸 Egresos: Alquiler, Servicios, Comida, Transporte, Salud, Educación, Entretenimiento, Suscripciones, Imprevistos)
  - Descripción libre
  - Medio de pago (efectivo, débito, crédito, transferencia)
  - **Para crédito**: cuotas (total y cuota actual), fecha de cierre
- Importar transacciones desde CSV de banco/billetera
- CRUD completo (crear, editar, eliminar con confirmación)

#### 4.3 Costos Fijos vs Variables
- Cada egreso se marca como **fijo** (se repite automáticamente) o **variable**
- Costos fijos: permiten configurar:
  - Día de vencimiento (ej: el 5 de cada mes)
  - Monto fijo o variable (ej: luz puede variar)
  - Activo/inactivo
- El sistema al iniciar un nuevo mes precarga los costos fijos activos pendientes

#### 4.4 Balance Mensual (Imprimible)
- Generación automática al cerrar el mes (o manual)
- Secciones en el balance:
  ```
  ┌─────────────────────────────────────┐
  │  BALANCE MENSUAL - ENERO 2026       │
  ├─────────────────────────────────────┤
  │  INGRESOS                    $ 1000 │
  │    Sueldo                    $ 800  │
  │    Freelance                 $ 200  │
  │                                   │
  │  EGRESOS                     $ 700  │
  │    Costos Fijos              $ 500  │
  │      Alquiler                $ 300  │
  │      Servicios               $ 100  │
  │      Suscripciones           $ 100  │
  │    Costos Variables           $ 200  │
  │      Comida                  $ 120  │
  │      Transporte              $ 50   │
  │      Entretenimiento         $ 30   │
  │                                   │
  │  RESULTADO NETO              $ 300  │  ← positivo = superávit
  │  TASA DE AHORRO              30%   │
  │                                   │
  │  AHORRO ACUMULADO            $ 5000 │
  │  INVERSIONES                 $ 2000 │
  │  DEUDAS (TC, préstamos)      $ 500  │
  │  PATRIMONIO NETO             $ 6500 │
  └─────────────────────────────────────┘
  ```
- Diseñado para impresión: versión HTML con CSS `@media print`, sin colores de fondo, sin botones, paginación limpia
- Exportable a PDF desde el navegador (Ctrl+P / window.print())

#### 4.5 Dashboard / Resumen visual
- Mes actual vs mes anterior (comparación)
- Gráfico de torta por categoría de egresos
- Tarjetas principales:
  - Ingresos del mes
  - Gastos del mes
  - Resultado neto (verde si positivo, rojo si negativo)
  - Tasa de ahorro
- Lista de últimos movimientos

### Fase 2

#### 4.6 Metas de Ahorro
- Crear meta (nombre, monto objetivo, fecha límite)
- Asignar parte del superávit a la meta
- Progreso visual

#### 4.7 Presupuestos por Categoría
- Definir límite mensual por categoría (ej: Comida $200/mes)
- Barra de progreso, alerta al superar 80% y 100%
- Ayuda a controlar gastos variables

#### 4.8 Gestión de Deudas / Tarjetas de Crédito
- Registrar tarjetas (límite, fecha de cierre, fecha de pago)
- Agregar consumos en cuotas
- Calculadora: cuánto pagar este mes para minimizar intereses
- Alerta de vencimientos

#### 4.9 Reportes y Exportación
- Balance anual (ingresos vs egresos mes a mes)
- Exportar a CSV/Excel
- Exportar balance a PDF server-side

### Fase 3

#### 4.10 Inversiones (Tracking simple)
- Registrar depósitos en instrumentos (plazo fijo, fondos, cedears, crypto)
- Valor actualizado (input manual o desde API)
- Rentabilidad acumulada

#### 4.11 Multi-moneda
- Transacciones en ARS, USD, EUR con conversión
- Balance consolidado con cotización al cierre del mes

#### 4.12 Gastos Compartidos
- Grupos de 2+ personas
- Dividir gastos (igual o porcentaje)
- Balance entre miembros: "le debés $X a Y"

---

## 5. Reglas de Negocio Detalladas

### 5.1 Cierre Mensual
- El mes se "cierra" automáticamente al primer día del mes siguiente
- Durante el cierre:
  1. Se copian los costos fijos activos al nuevo mes como transacciones pendientes (con monto estimado)
  2. Se genera el balance del mes cerrado (inmutable)
  3. Se calculan indicadores (tasa de ahorro, variación vs mes anterior)
- El usuario puede cerrar manualmente antes (ej: 28 de enero)
- Balance cerrado = no se puede modificar (solo lectura). Si se necesita corregir, se crea un ajuste en el mes actual.

### 5.2 Correcciones
- No se permite eliminar transacciones de meses cerrados
- Se permite agregar una transacción de "ajuste" con descripción obligatoria
- El balance refleja la diferencia como "Ajustes"

### 5.3 Categorías
- El sistema tiene categorías predefinidas (lista fija)
- El usuario puede crear subcategorías (ej: Comida → "Supermercado", "Delivery", "Restaurantes")
- Una transacción pertenece a exactamente una categoría/subcategoría

### 5.4 Indicadores Clave
```
Tasa de Ahorro (%)     = Superávit / Ingresos * 100
Gastos Fijos Ratio (%) = Gastos Fijos / Ingresos * 100
Cobertura (meses)      = Ahorro Total / Gastos Mensuales Promedio
Variación Mensual (%)  = (MesActual - MesAnterior) / MesAnterior * 100
```

---

## 6. Modelo de Datos (Entidades Principales)

```
Usuario
  id, nombre, email, password_hash, moneda_default, created_at

Transaccion
  id, usuario_id, tipo (ingreso|egreso), monto, fecha, categoria_id,
  subcategoria_id, descripcion, medio_pago, es_fijo, cuotas_total,
  cuota_actual, estado (pendiente|confirmado|ajuste),
  mes_id (nullable), created_at, updated_at

CostoFijo
  id, usuario_id, categoria_id, descripcion, monto_estimado,
  dia_vencimiento, activo, tipo_periodo (mensual|bimestral|anual),
  created_at

Categoria
  id, nombre, tipo (ingreso|egreso), icono, es_personalizada,
  usuario_id (nullable), created_at

Subcategoria
  id, categoria_id, nombre, icono, created_at

Mes
  id, usuario_id, periodo (YYYY-MM), estado (abierto|cerrado),
  ingresos_total, egresos_total, superavit, tasa_ahorro,
  ahorro_acumulado, pasivos_total, patrimonio, created_at

MetaAhorro
  id, usuario_id, nombre, monto_objetivo, monto_actual,
  fecha_limite, created_at

Presupuesto
  id, usuario_id, categoria_id, mes_id, monto_limite, created_at

Deuda / Tarjeta
  id, usuario_id, tipo (tarjeta|prestamo|otro), entidad,
  monto_total, cuotas_restantes, tasa_interes, fecha_cierre,
  fecha_pago, created_at

Inversion
  id, usuario_id, tipo_instrumento, monto_invertido,
  valor_actual, rentabilidad, created_at
```

---

## 7. UX / Diseño

### Flujo principal del usuario

```
Login → Dashboard (mes actual)
         ├── Ver balance del mes
         │     └── Imprimir / PDF
         ├── Agregar transacción rápida (desde cualquier pantalla)
         ├── Ver/Categorizar costos fijos
         ├── Comparar con mes anterior
         └── Configurar presupuestos
```

### Principios de diseño
- **Una acción principal por pantalla.** Sin overload de opciones.
- **Input de transacción lo más rápido posible:** 3 taps: monto → categoría → guardar.
- **Idioma español** (moneda: $, fechas: dd/mm/aaaa).
- **Modo oscuro** opcional.
- **Imprimible:** CSS `@media print` desde el día 1. El balance en papel debe verse profesional.

---

## 8. Stack Tecnológico (Basado en el análisis previo)

### Backend (Go + chi)
- `internal/handler/` — endpoints REST/HTMX
- `internal/service/` — lógica: cierre mensual, cálculo de indicadores, presupuestos
- `internal/repository/` — database/sql queries (SQL directo, sin ORM)
- `internal/middleware/` — JWT, logging, CORS, rate limit
- `internal/model/` — tipos de dominio

### Frontend (HTMX + html/template)
- `web/templates/` — templates Go con componentes reutilizables
- CSS vanilla con diseño tipo dashboard (Tailwind o custom) + soporte print
- Sin JS framework: interacciones vía HTMX (`hx-get`, `hx-post`, `hx-trigger`)
- Gráficos: chart.js vía CDN (es la única dependencia JS, y vale la pena)

### Base de datos
- **MySQL** (database/sql + go-sql-driver/mysql)
- Schema con migraciones (golang-migrate o goose)

### Despliegue
- Docker multi-stage → binario único con frontend embebido
- Puerto único, cero dependencias de infra

---

## 9. Glosario (para el equipo de desarrollo)

| Término | Definición técnica |
|---------|-------------------|
| Superávit | Ingresos - Egresos del período. Puede ser negativo (déficit) |
| Ahorro | Dinero acumulado de períodos anteriores, disponible en cuentas/efectivo |
| Tasa de ahorro | Porcentaje del ingreso que se ahorra. Saludable >15%, ideal >25% |
| Costo fijo | Egreso recurrente predecible (alquiler, internet, suscripción) |
| Costo variable | Egreso no predecible (comida, entretenimiento) |
| Patrimonio | Todo lo que tiene (activos) menos todo lo que debe (pasivos) |
| Balance | Estado financiero de un período específico (mensual) |
| Cierre de mes | Proceso que congela el mes e inicia el siguiente con costos fijos |
| Ajuste | Transacción de corrección en un mes cerrado |
| Cobertura | Meses que podría vivir sin ingresos usando su ahorro actual |
| Presupuesto | Límite autoimpuesto por categoría para controlar gastos |

---

## 10. Próximos Pasos Recomendados

1. **Definir MVP exacto** (de la sección 4, elegir qué entra en v1.0)
2. **Diseñar schema SQL** y migraciones iniciales (sqlc)
3. **Crear estructura del proyecto Go** (cmd/server, internal/handler, etc.)
4. **Implementar autenticación + transacciones + dashboard** (núcleo)
5. **Implementar balance imprimible** (con CSS print)
6. **Implementar costos fijos y precarga mensual**
7. **Testing y deploy**

---

*Documento generado desde perfil de asesor financiero con experiencia en productos digitales para Pymes y personas físicas.*
