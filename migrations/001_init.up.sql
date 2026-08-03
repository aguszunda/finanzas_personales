CREATE TABLE IF NOT EXISTS usuarios (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    nombre VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    moneda_default VARCHAR(10) DEFAULT 'ARS',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS categorias (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    nombre VARCHAR(255) NOT NULL,
    tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('ingreso', 'egreso')),
    icono VARCHAR(50) DEFAULT '',
    es_personalizada BOOLEAN DEFAULT FALSE,
    usuario_id BIGINT REFERENCES usuarios(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS meses (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    periodo VARCHAR(7) NOT NULL,
    estado VARCHAR(20) DEFAULT 'abierto' CHECK (estado IN ('abierto', 'cerrado')),
    ingresos_total DECIMAL(15,2) DEFAULT 0,
    egresos_total DECIMAL(15,2) DEFAULT 0,
    superavit DECIMAL(15,2) DEFAULT 0,
    tasa_ahorro DECIMAL(5,2),
    ahorro_acumulado DECIMAL(15,2) DEFAULT 0,
    pasivos_total DECIMAL(15,2) DEFAULT 0,
    patrimonio DECIMAL(15,2) DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(usuario_id, periodo)
);

CREATE TABLE IF NOT EXISTS transacciones (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('ingreso', 'egreso')),
    monto DECIMAL(15,2) NOT NULL,
    fecha DATE NOT NULL,
    categoria_id BIGINT NOT NULL REFERENCES categorias(id),
    descripcion TEXT,
    medio_pago VARCHAR(50) DEFAULT '',
    es_fijo BOOLEAN DEFAULT FALSE,
    cuotas_total INT,
    cuota_actual INT,
    estado VARCHAR(20) DEFAULT 'confirmado' CHECK (estado IN ('pendiente', 'confirmado', 'ajuste')),
    mes_id BIGINT REFERENCES meses(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS costos_fijos (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    categoria_id BIGINT NOT NULL REFERENCES categorias(id),
    descripcion VARCHAR(255) NOT NULL,
    monto_estimado DECIMAL(15,2) NOT NULL,
    dia_vencimiento INT NOT NULL CHECK (dia_vencimiento BETWEEN 1 AND 31),
    activo BOOLEAN DEFAULT TRUE,
    tipo_periodo VARCHAR(20) DEFAULT 'mensual' CHECK (tipo_periodo IN ('mensual', 'bimestral', 'anual')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_transacciones_usuario_fecha ON transacciones(usuario_id, fecha DESC);
CREATE INDEX idx_transacciones_mes ON transacciones(mes_id);
CREATE INDEX idx_costos_fijos_usuario ON costos_fijos(usuario_id);
CREATE INDEX idx_meses_usuario_periodo ON meses(usuario_id, periodo);

INSERT IGNORE INTO categorias (nombre, tipo, icono, es_personalizada) VALUES
    ('Sueldo', 'ingreso', '💰', FALSE),
    ('Freelance', 'ingreso', '💻', FALSE),
    ('Ventas', 'ingreso', '📦', FALSE),
    ('Otros Ingresos', 'ingreso', '📥', FALSE),
    ('Alquiler', 'egreso', '🏠', FALSE),
    ('Servicios', 'egreso', '💡', FALSE),
    ('Comida', 'egreso', '🍽️', FALSE),
    ('Transporte', 'egreso', '🚗', FALSE),
    ('Salud', 'egreso', '🏥', FALSE),
    ('Educación', 'egreso', '📚', FALSE),
    ('Entretenimiento', 'egreso', '🎬', FALSE),
    ('Suscripciones', 'egreso', '📱', FALSE),
    ('Imprevistos', 'egreso', '⚠️', FALSE);
