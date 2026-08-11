ALTER TABLE deudas DROP FOREIGN KEY deudas_ibfk_2;
ALTER TABLE deudas
    DROP COLUMN medio_pago,
    DROP COLUMN categoria_id;