package service

import (
	"context"
	"crypto/tls"
	"html"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Mailer abstrae el envío del mail de verificación de cuenta. La interfaz vive
// en service para que AuthService no dependa de una implementación concreta.
type Mailer interface {
	SendVerificacion(ctx context.Context, email, nombre, link string) error
}

// SmtpConfig agrupa la configuración de envío. Con Host vacío NewMailer
// devuelve un mailer que solo loguea el link (modo desarrollo sin SMTP).
type SmtpConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

const smtpTimeout = 10 * time.Second

func NewMailer(cfg SmtpConfig) Mailer {
	if cfg.Host == "" {
		return &logMailer{}
	}
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	return &smtpMailer{cfg: cfg}
}

// smtpMailer envía por SMTP: TLS implícito en el puerto 465 y STARTTLS en el
// resto. Autentica solo si hay credenciales configuradas.
type smtpMailer struct {
	cfg SmtpConfig
}

func (m *smtpMailer) SendVerificacion(_ context.Context, email, nombre, link string) error {
	msg := buildVerificacionEmail(m.cfg.From, email, nombre, link)
	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)

	var client *smtp.Client
	var err error
	if m.cfg.Port == "465" {
		client, err = dialSMTPS(addr, m.cfg.Host)
	} else {
		client, err = dialSMTPStartTLS(addr, m.cfg.Host)
	}
	if err != nil {
		return err
	}
	defer client.Close()

	if m.cfg.User != "" {
		auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(m.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(email); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func dialSMTPS(addr, host string) (*smtp.Client, error) {
	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return nil, err
	}
	c := tls.Client(conn, &tls.Config{ServerName: host})
	return smtp.NewClient(c, host)
}

func dialSMTPStartTLS(addr, host string) (*smtp.Client, error) {
	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return nil, err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			c.Close()
			return nil, err
		}
	}
	return c, nil
}

// buildVerificacionEmail arma el mensaje MIME (HTML + fallback texto plano).
func buildVerificacionEmail(from, to, nombre, link string) []byte {
	nombreSeguro := html.EscapeString(nombre)
	linkSeguro := html.EscapeString(link)
	subject := "MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"From: Optipay <" + from + ">\r\n" +
		"To: " + to + "\r\n" +
		"Subject: Confirmá tu cuenta en Optipay\r\n" +
		"\r\n"
	body := "<p>Hola " + nombreSeguro + ",</p>" +
		"<p>Confirmá tu email haciendo clic en el siguiente botón:</p>" +
		"<p><a href=\"" + linkSeguro + "\" style=\"display:inline-block;padding:10px 20px;background:#2563eb;color:#ffffff;text-decoration:none;border-radius:6px;font-family:sans-serif;\">Confirmar mi email</a></p>" +
		"<p>Si el botón no funciona, copiá y pegá este enlace en tu navegador:</p>" +
		"<p><a href=\"" + linkSeguro + "\">" + linkSeguro + "</a></p>" +
		"<p style=\"color:#6b7280;font-family:sans-serif\">El enlace vence en 48 horas. Si no creaste esta cuenta, ignorá este mensaje.</p>"
	return []byte(subject + body)
}

// logMailer es el modo desarrollo: sin SMTP configurado, imprime el link por
// stdout para poder completar el flujo manualmente durante pruebas locales.
type logMailer struct{}

func (m *logMailer) SendVerificacion(_ context.Context, email, _, link string) error {
	slog.Info("[mailer modo dev] link de verificación", "email", email, "link", strings.TrimSpace(link))
	return nil
}
