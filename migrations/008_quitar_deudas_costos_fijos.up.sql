DROP TABLE IF EXISTS costos_fijos;
DROP TABLE IF EXISTS deudas;

ALTER TABLE transacciones
    DROP COLUMN es_fijo,
    DROP COLUMN cuotas_total,
    DROP COLUMN cuota_actual;

ALTER TABLE meses
    DROP COLUMN pasivos_total,
    DROP COLUMN patrimonio;