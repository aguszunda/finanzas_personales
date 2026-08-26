package service

import (
	"context"
	"strings"
	"testing"
)

func TestNewMailer_SinHostUsaModoDev(t *testing.T) {
	m := NewMailer(SmtpConfig{})
	if _, ok := m.(*logMailer); !ok {
		t.Fatalf("expected logMailer without SMTP_HOST, got %T", m)
	}
}

func TestNewMailer_ConHostUsaSMTP(t *testing.T) {
	m := NewMailer(SmtpConfig{Host: "smtp.gmail.com", From: "no-reply@test.com"})
	if _, ok := m.(*smtpMailer); !ok {
		t.Fatalf("expected smtpMailer with SMTP_HOST, got %T", m)
	}
	if m.(*smtpMailer).cfg.Port != "587" {
		t.Errorf("expected default port 587, got %q", m.(*smtpMailer).cfg.Port)
	}
}

func TestBuildVerificacionEmail(t *testing.T) {
	msg := string(buildVerificacionEmail("no-reply@optipay.local", "user@test.com", "Agus <script>", "http://localhost:8080/api/auth/verificar?token=abc"))
	for _, want := range []string{
		"From: Optipay <no-reply@optipay.local>",
		"To: user@test.com",
		"Subject: Confirmá tu cuenta en Optipay",
		"href=\"http://localhost:8080/api/auth/verificar?token=abc\"",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q", want)
		}
	}
	if strings.Contains(msg, "<script>") {
		t.Error("nombre must be HTML-escaped")
	}
	if !strings.Contains(msg, "Agus &lt;script&gt;") {
		t.Error("expected escaped nombre in body")
	}
}

func TestLogMailer_EnviaSinError(t *testing.T) {
	m := &logMailer{}
	if err := m.SendVerificacion(context.Background(), "a@test.com", "A", "http://x/verificar?t=1"); err != nil {
		t.Fatalf("logMailer must never fail, got %v", err)
	}
}

func TestLogMailer_SendPasswordReset_SinError(t *testing.T) {
	m := &logMailer{}
	if err := m.SendPasswordReset(context.Background(), "a@test.com", "A", "http://x/reset?t=1"); err != nil {
		t.Fatalf("logMailer.SendPasswordReset must never fail, got %v", err)
	}
}

func TestBuildPasswordResetEmail(t *testing.T) {
	msg := string(buildPasswordResetEmail("no-reply@optipay.local", "user@test.com", "Agus <script>", "http://localhost:8080/reset-password?token=abc"))
	for _, want := range []string{
		"From: Optipay <no-reply@optipay.local>",
		"To: user@test.com",
		"Subject: Recuperá tu contraseña en Optipay",
		"href=\"http://localhost:8080/reset-password?token=abc\"",
		"Restablecer contraseña",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q", want)
		}
	}
	if strings.Contains(msg, "<script>") {
		t.Error("nombre must be HTML-escaped")
	}
	if !strings.Contains(msg, "Agus &lt;script&gt;") {
		t.Error("expected escaped nombre in body")
	}
}
