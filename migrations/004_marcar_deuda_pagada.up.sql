ALTER TABLE deudas ADD COLUMN estado VARCHAR(20) NOT NULL DEFAULT 'pendiente' AFTER proximo_vencimiento;

CREATE INDEX idx_deudas_estado ON deudas(estado);