package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ResendMailer sends mail through the Resend HTTP API
// (https://resend.com/docs/api-reference/emails/send-email).
// No SDK needed: one POST per message.
type ResendMailer struct {
	APIKey  string
	From    string // e.g. "VideoDelite <noreply@mail.yourdomain.com>"
	BaseURL string // default https://api.resend.com
	HTTP    *http.Client
}

func NewResendMailer(apiKey, from string) *ResendMailer {
	return &ResendMailer{
		APIKey:  apiKey,
		From:    from,
		BaseURL: "https://api.resend.com",
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

type resendEmailResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Name    string `json:"name"`
}

// SendVerificationCode delivers the 6-digit code via Resend.
func (m *ResendMailer) SendVerificationCode(to, code string) error {
	if m.APIKey == "" {
		return fmt.Errorf("resend api key is empty")
	}
	if m.From == "" {
		return fmt.Errorf("resend from address is empty")
	}
	payload := resendEmailRequest{
		From:    m.From,
		To:      []string{to},
		Subject: "VideoDelite 邮箱验证码",
		Text: fmt.Sprintf(
			"您好！\r\n\r\n您的 VideoDelite 邮箱验证码是：\r\n\r\n    %s\r\n\r\n验证码 %d 分钟内有效。如果这不是您本人的操作，请忽略本邮件。\r\n\r\n—— VideoDelite",
			code, int(emailTokenTTL.Minutes())),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", m.BaseURL+"/emails", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("resend api: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend api %d: %s", resp.StatusCode, truncateStr(string(body), 200))
	}
	return nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
