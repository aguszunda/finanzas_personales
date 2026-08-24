ALTER TABLE usuarios
  ADD COLUMN email_verificado BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN token_verificacion CHAR(64) NULL,
  ADD COLUMN token_expiracion DATETIME NULL;

CREATE INDEX idx_usuarios_token_verificacion ON usuarios (token_verificacion);

-- La verificación por email aplica a registros nuevos; los usuarios que ya
-- existían antes de esta migración quedan verificados para no perder acceso.
UPDATE usuarios SET email_verificado = TRUE;
