ALTER TABLE usuarios
  ADD COLUMN password_reset_token CHAR(64) NULL,
  ADD COLUMN password_reset_expiracion DATETIME NULL;

CREATE INDEX idx_usuarios_password_reset_token ON usuarios (password_reset_token);
