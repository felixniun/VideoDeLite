package server

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

var base64Std = base64.StdEncoding

// SMTPConfig carries the mail relay settings (config.json → "smtp").
type SMTPConfig struct {
	Host     string `json:"host"`     // e.g. smtp.example.com
	Port     int    `json:"port"`     // 587 (STARTTLS) or 465 (implicit TLS)
	Username string `json:"username"` // usually the from address
	Password string `json:"password"`
	From     string `json:"from"` // e.g. "VideoDelite <no-reply@videodelite.app>"
	// Mode: "starttls" (default) or "tls" for implicit TLS on connect.
	Mode string `json:"mode"`
	// TimeoutSec bounds the whole send operation. Default 15.
	TimeoutSec int `json:"timeoutSec"`
}

// Mailer sends account emails. The interface exists so tests can inject a
// fake relay and production can swap providers without touching handlers.
type Mailer interface {
	SendVerificationCode(to, code string) error
}

// SMTPMailer implements Mailer against a real relay.
type SMTPMailer struct {
	cfg     SMTPConfig
	timeout time.Duration
}

func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	t := time.Duration(cfg.TimeoutSec) * time.Second
	if t <= 0 {
		t = 15 * time.Second
	}
	return &SMTPMailer{cfg: cfg, timeout: t}
}

// SendVerificationCode composes and delivers the code email.
func (m *SMTPMailer) SendVerificationCode(to, code string) error {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	from := m.cfg.From
	if from == "" {
		from = m.cfg.Username
	}

	subject := "VideoDelite 邮箱验证码"
	body := fmt.Sprintf(
		"您好！\r\n\r\n您的 VideoDelite 邮箱验证码是：\r\n\r\n    %s\r\n\r\n验证码 %d 分钟内有效。如果这不是您本人的操作，请忽略本邮件。\r\n\r\n—— VideoDelite",
		code, int(emailTokenTTL.Minutes()))
	msg := m.buildMessage(from, to, subject, body)

	var conn net.Conn
	var err error
	d := net.Dialer{Timeout: m.timeout}
	if strings.EqualFold(m.cfg.Mode, "tls") {
		conn, err = tls.DialWithDialer(&d, "tcp", addr,
			&tls.Config{ServerName: m.cfg.Host})
	} else {
		conn, err = d.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp connect %s: %w", addr, err)
	}

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer client.Close()

	done := make(chan error, 1)
	go func() { done <- m.send(client, from, to, msg) }()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("smtp send: %w", err)
		}
		return nil
	case <-time.After(m.timeout):
		client.Close()
		return fmt.Errorf("smtp send timed out after %s", m.timeout)
	}
}

func (m *SMTPMailer) send(client *smtp.Client, from, to, msg string) error {
	if ok, _ := client.Extension("STARTTLS"); ok && !strings.EqualFold(m.cfg.Mode, "tls") {
		if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	var auth smtp.Auth
	if m.cfg.Username != "" {
		host := m.cfg.Host
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, host)
		if ok, params := client.Extension("AUTH"); ok {
			if !strings.Contains(params, "PLAIN") && strings.Contains(params, "LOGIN") {
				auth = &loginAuth{username: m.cfg.Username, password: m.cfg.Password}
			}
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// buildMessage produces an RFC 5322 message with UTF-8 subject encoded
// per RFC 2047 and a plain-text body.
func (m *SMTPMailer) buildMessage(from, to, subject, body string) string {
	// Simplified RFC 2047 B-encoding for the UTF-8 subject.
	b64 := base64Std.EncodeToString([]byte(subject))
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: =?UTF-8?B?%s?=\r\n", b64)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&b, "Content-Transfer-Encoding: 8bit\r\n")
	fmt.Fprintf(&b, "\r\n%s\r\n", body)
	return b.String()
}

// loginAuth implements SMTP AUTH LOGIN (common on legacy relays).
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(string(fromServer))
	switch {
	case strings.Contains(prompt, "username"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "password"):
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected SMTP LOGIN prompt: %q", fromServer)
}
