DROP INDEX idx_usuarios_token_verificacion ON usuarios;

ALTER TABLE usuarios
  DROP COLUMN token_expiracion,
  DROP COLUMN token_verificacion,
  DROP COLUMN email_verificado;
