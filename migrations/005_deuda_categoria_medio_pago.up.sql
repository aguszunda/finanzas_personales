ALTER TABLE deudas
    ADD COLUMN categoria_id BIGINT NULL REFERENCES categorias(id) AFTER monto_total,
    ADD COLUMN medio_pago VARCHAR(50) NOT NULL DEFAULT '' AFTER categoria_id;