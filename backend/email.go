package main

import (
	"bytes"
	"fmt"
	"log"
	"net/smtp"
)

func (s *Server) getSMTPConfig() (enabled bool, host, port, user, password, from, fromName string, tls bool) {
	enabledStr, _ := s.getSetting("smtp_enabled")
	enabled = enabledStr == "1"
	if !enabled {
		return
	}
	host, _ = s.getSetting("smtp_host")
	portStr, _ := s.getSetting("smtp_port")
	user, _ = s.getSetting("smtp_user")
	password, _ = s.getSetting("smtp_password")
	from, _ = s.getSetting("smtp_from")
	fromName, _ = s.getSetting("smtp_from_name")
	tlsStr, _ := s.getSetting("smtp_tls")
	tls = tlsStr != "0"
	_ = tls // avoid unused variable warning (TLS is not currently implemented)
	if portStr == "" {
		portStr = "587"
	}
	port = portStr
	return
}

func (s *Server) sendEmail(to, subject, bodyHTML string) error {
	enabled, host, port, user, password, from, fromName, tls := s.getSMTPConfig()
	if !enabled {
		return fmt.Errorf("SMTP not enabled")
	}
	if from == "" {
		from = "noreply@conflux.local"
	}
	if fromName == "" {
		fromName = "Conflux"
	}
	auth := smtp.PlainAuth("", user, password, host)
	addr := host + ":" + port

	fromAddr := fmt.Sprintf("%s <%s>", fromName, from)
	headers := make(map[string]string)
	headers["From"] = fromAddr
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=utf-8"

	var msg bytes.Buffer
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n" + bodyHTML)

	err := smtp.SendMail(addr, auth, from, []string{to}, msg.Bytes())
	return err
}

func (s *Server) sendWelcomeEmail(user *User) {
	subject := "Welcome to Conflux!"
	body := fmt.Sprintf(`<h1>Welcome, %s!</h1><p>Your account has been created. Start following feeds and connecting with others.</p><p><a href="%s">Visit Conflux</a></p>`,
		user.DisplayName, s.cfg.PublicBaseURL)
	if err := s.sendEmail(user.Email, subject, body); err != nil {
		log.Printf("failed to send welcome email: %v", err)
	}
}

func (s *Server) sendPasswordResetEmail(email, token string) {
	resetLink := s.cfg.PublicBaseURL + "/#/reset-password?token=" + token
	subject := "Reset your Conflux password"
	body := fmt.Sprintf(`<h1>Reset your password</h1><p>Click the link below to set a new password. This link expires in 1 hour.</p><p><a href="%s">%s</a></p>`, resetLink, resetLink)
	if err := s.sendEmail(email, subject, body); err != nil {
		log.Printf("failed to send password reset email: %v", err)
	}
}
