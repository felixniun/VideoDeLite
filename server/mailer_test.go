package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// TestResendMailer verifies the Resend HTTP path against a fake API server,
// including auth header, payload shape and error propagation.
func TestResendMailer(t *testing.T) {
	var gotAuth, gotTo, gotFrom, gotText string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" {
			http.Error(w, "not found", 404)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		var req resendEmailRequest
		_ = json.Unmarshal(raw, &req)
		gotTo = req.To[0]
		gotFrom = req.From
		gotText = req.Text
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(resendEmailResponse{ID: "email-123"})
	}))
	defer fake.Close()

	m := NewResendMailer("re_test_key_123", "VideoDelite <noreply@test.local>")
	m.BaseURL = fake.URL

	if err := m.SendVerificationCode("user@example.com", "123456"); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if gotAuth != "Bearer re_test_key_123" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotTo != "user@example.com" || gotFrom != "VideoDelite <noreply@test.local>" {
		t.Fatalf("to=%q from=%q", gotTo, gotFrom)
	}
	if gotText == "" || !contains(gotText, "123456") {
		t.Fatalf("body missing code: %q", gotText)
	}

	// API error surfaces with status + body.
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"message":"domain not verified"}`))
	}))
	defer errSrv.Close()
	m2 := NewResendMailer("k", "f")
	m2.BaseURL = errSrv.URL
	err := m2.SendVerificationCode("x@y.z", "000000")
	if err == nil || !contains(err.Error(), "422") || !contains(err.Error(), "domain not verified") {
		t.Fatalf("expected 422 error passthrough, got %v", err)
	}
}

// TestResendVerificationFlow runs register → resend → verify with a fake
// Resend relay wired through the full HTTP handler stack.
func TestResendVerificationFlow(t *testing.T) {
	var lastCode string
	fakeResend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req resendEmailRequest
		_ = json.Unmarshal(raw, &req)
		// Extract the 6-digit code from the body text.
		for i := 0; i+6 <= len(req.Text); i++ {
			c := req.Text[i : i+6]
			digits := true
			for j := 0; j < 6; j++ {
				if c[j] < '0' || c[j] > '9' {
					digits = false
					break
				}
			}
			if digits {
				lastCode = c
				break
			}
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(resendEmailResponse{ID: "e1"})
	}))
	defer fakeResend.Close()

	st, err := OpenDevSQLite(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	srv := New(st, Options{
		DevEchoMail: false,
		Mailer: &ResendMailer{
			APIKey: "k", From: "VideoDelite <noreply@test.local>",
			BaseURL: fakeResend.URL,
			HTTP:    &http.Client{Timeout: 5 * time.Second},
		},
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	})
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// register: code goes to the fake Resend, NOT echoed in response
	status, resp := post(t, ts.URL+"/api/v1/accounts/register", nil, map[string]any{
		"username": "mailuser", "email": "mail@example.com", "password": "password-123",
	})
	if status != 201 {
		t.Fatalf("register: %d %v", status, resp)
	}
	if _, echoed := resp["devVerificationCode"]; echoed {
		t.Fatal("code must not be echoed when a real mailer is configured")
	}
	if lastCode == "" {
		t.Fatal("no verification email delivered")
	}

	// verify with the code delivered by email
	status, resp = post(t, ts.URL+"/api/v1/accounts/verify-email", nil, map[string]any{
		"email": "mail@example.com", "code": lastCode,
	})
	if status != 200 {
		t.Fatalf("verify: %d %v", status, resp)
	}

	// throttle: second resend within 60s is rejected
	lastCode = ""
	_, resp = post(t, ts.URL+"/api/v1/accounts/register", nil, map[string]any{
		"username": "mailuser2", "email": "throttle@example.com", "password": "password-123",
	})
	status, resp = post(t, ts.URL+"/api/v1/accounts/resend-verification", nil, map[string]any{
		"email": "throttle@example.com",
	})
	if status != 200 {
		t.Fatalf("first resend: %d %v", status, resp)
	}
	status, resp = post(t, ts.URL+"/api/v1/accounts/resend-verification", nil, map[string]any{
		"email": "throttle@example.com",
	})
	if status != 429 {
		t.Fatalf("second resend should be throttled 429, got %d %v", status, resp)
	}
}
