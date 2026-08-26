DROP INDEX idx_usuarios_password_reset_token ON usuarios;

ALTER TABLE usuarios
  DROP COLUMN password_reset_token,
  DROP COLUMN password_reset_expiracion;
