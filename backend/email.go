package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"time"
)

func (s *Server) getSMTPConfig() (enabled bool, host, port, user, password, from, fromName string, tlsEnabled bool) {
	enabledStr, _ := s.getSetting("smtp_enabled")
	enabled = enabledStr == "1"
	if !enabled {
		return
	}
	host, _ = s.getSetting("smtp_host")
	portStr, _ := s.getSetting("smtp_port")
	if portStr == "" {
		portStr = "587"
	}
	port = portStr
	user, _ = s.getSetting("smtp_user")
	password, _ = s.getSetting("smtp_password")
	from, _ = s.getSetting("smtp_from")
	fromName, _ = s.getSetting("smtp_from_name")
	tlsStr, _ := s.getSetting("smtp_tls")
	tlsEnabled = tlsStr == "1"
	return
}

func (s *Server) sendEmail(to, subject, bodyHTML string) error {
	enabled, host, port, user, password, from, fromName, tlsEnabled := s.getSMTPConfig()
	if !enabled {
		return fmt.Errorf("SMTP not enabled")
	}
	if host == "" {
		return fmt.Errorf("SMTP host not configured")
	}
	if from == "" {
		from = "noreply@conflux.local"
	}
	if fromName == "" {
		fromName = "Conflux"
	}

	// Build message
	headers := make(map[string]string)
	headers["From"] = fmt.Sprintf("%s <%s>", fromName, from)
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=utf-8"

	var msg bytes.Buffer
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n" + bodyHTML)

	// Set up authentication
	auth := smtp.PlainAuth("", user, password, host)

	// Dial the SMTP server
	addr := net.JoinHostPort(host, port)
	var client *smtp.Client
	var err error

	if tlsEnabled {
		// Use TLS (either direct TLS or STARTTLS)
		tlsConfig := &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		// Try direct TLS connection on port 465 (SMTPS) if port is 465; otherwise use STARTTLS
		if port == "465" {
			conn, err := tls.Dial("tcp", addr, tlsConfig)
			if err != nil {
				return fmt.Errorf("failed to dial TLS: %w", err)
			}
			client, err = smtp.NewClient(conn, host)
			if err != nil {
				return fmt.Errorf("failed to create SMTP client: %w", err)
			}
		} else {
			// Plain TCP then STARTTLS
			conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
			if err != nil {
				return fmt.Errorf("failed to dial: %w", err)
			}
			client, err = smtp.NewClient(conn, host)
			if err != nil {
				return fmt.Errorf("failed to create SMTP client: %w", err)
			}
			if err = client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
		}
	} else {
		// Plain connection (no TLS) – not recommended
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("failed to dial: %w", err)
		}
		client, err = smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
	}
	defer client.Close()

	// Auth
	if user != "" && password != "" {
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	// From
	if err = client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}
	// To
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO failed: %w", err)
	}
	// Data
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}
	if _, err = w.Write(msg.Bytes()); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close data: %w", err)
	}
	return nil
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
