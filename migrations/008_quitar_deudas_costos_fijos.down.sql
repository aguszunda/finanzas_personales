CREATE TABLE IF NOT EXISTS deudas (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    tipo VARCHAR(50) NOT NULL DEFAULT 'otro',
    entidad VARCHAR(255) NOT NULL,
    descripcion TEXT,
    monto_total DECIMAL(15,2) NOT NULL,
    categoria_id BIGINT NULL,
    medio_pago VARCHAR(50) NOT NULL DEFAULT '',
    proximo_vencimiento DATE,
    estado VARCHAR(20) NOT NULL DEFAULT 'pendiente',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_deudas_categoria FOREIGN KEY (categoria_id) REFERENCES categorias(id)
);

CREATE INDEX idx_deudas_usuario ON deudas(usuario_id);
CREATE INDEX idx_deudas_estado ON deudas(estado);

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

CREATE INDEX idx_costos_fijos_usuario ON costos_fijos(usuario_id);

ALTER TABLE transacciones
    ADD COLUMN es_fijo BOOLEAN DEFAULT FALSE AFTER medio_pago,
    ADD COLUMN cuotas_total INT AFTER es_fijo,
    ADD COLUMN cuota_actual INT AFTER cuotas_total;

ALTER TABLE meses
    ADD COLUMN pasivos_total DECIMAL(15,2) DEFAULT 0 AFTER ahorro_acumulado,
    ADD COLUMN patrimonio DECIMAL(15,2) DEFAULT 0 AFTER pasivos_total;