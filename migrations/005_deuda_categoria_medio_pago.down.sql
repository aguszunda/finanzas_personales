ALTER TABLE deudas DROP FOREIGN KEY fk_deudas_categoria;
ALTER TABLE deudas
    DROP COLUMN medio_pago,
    DROP COLUMN categoria_id;