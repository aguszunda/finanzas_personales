CREATE TABLE IF NOT EXISTS deudas (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    tipo VARCHAR(50) NOT NULL DEFAULT 'otro',
    entidad VARCHAR(255) NOT NULL,
    descripcion TEXT,
    monto_total DECIMAL(15,2) NOT NULL,
    saldo_pendiente DECIMAL(15,2) NOT NULL,
    tasa_interes DECIMAL(6,2) DEFAULT 0,
    proximo_vencimiento DATE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_deudas_usuario ON deudas(usuario_id);
