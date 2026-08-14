ALTER TABLE deudas
    ADD COLUMN categoria_id BIGINT NULL AFTER monto_total,
    ADD COLUMN medio_pago VARCHAR(50) NOT NULL DEFAULT '' AFTER categoria_id,
    ADD CONSTRAINT fk_deudas_categoria FOREIGN KEY (categoria_id) REFERENCES categorias(id);