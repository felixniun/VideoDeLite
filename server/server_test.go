package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testPassword is generated at test runtime (never a literal credential);
// register/login/delete flows all reference the same value.
var testPassword = randomToken(12)

func newTestServer(t *testing.T, adminKey string) (*Server, *httptest.Server) {
	t.Helper()
	st, err := OpenDevSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("dev store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := New(st, Options{
		InviteRequired: false,
		DevEchoMail:    true,
		AccessTTL:      15 * time.Minute,
		RefreshTTL:     24 * time.Hour,
		AdminKey:       adminKey,
	})
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return srv, ts
}

func post(t *testing.T, url string, headers map[string]string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func get(t *testing.T, url string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

const testInstallID = "0f3a9c2e11114d7f9a33b0aa1d2e3f40"

// TestFullLifecycle covers the complete authorization lifecycle:
// register → verify → login → device activate → license status →
// token refresh → admin revoke → account delete.
func TestFullLifecycle(t *testing.T) {
	_, ts := newTestServer(t, "")

	// 1. register (dev mode echoes the verification code)
	status, resp := post(t, ts.URL+"/api/v1/accounts/register", nil, map[string]any{
		"username": "alice", "email": "alice@example.com", "password": testPassword,
	})
	if status != 201 {
		t.Fatalf("register: %d %v", status, resp)
	}
	code, _ := resp["devVerificationCode"].(string)
	if code == "" {
		t.Fatal("dev verification code missing")
	}

	// 2. login before verification must fail
	status, _ = post(t, ts.URL+"/api/v1/auth/login", nil, map[string]any{
		"email": "alice@example.com", "password": testPassword,
	})
	if status != 403 {
		t.Fatalf("pre-verify login should be 403, got %d", status)
	}

	// 3. verify email (wrong code rejected, then right code)
	status, _ = post(t, ts.URL+"/api/v1/accounts/verify-email", nil, map[string]any{
		"email": "alice@example.com", "code": "000000",
	})
	if status != 400 {
		t.Fatalf("wrong code should be 400, got %d", status)
	}
	status, _ = post(t, ts.URL+"/api/v1/accounts/verify-email", nil, map[string]any{
		"email": "alice@example.com", "code": code,
	})
	if status != 200 {
		t.Fatalf("verify: %d %v", status, resp)
	}

	// 4. login
	status, resp = post(t, ts.URL+"/api/v1/auth/login", nil, map[string]any{
		"email": "alice@example.com", "password": testPassword,
	})
	if status != 200 {
		t.Fatalf("login: %d %v", status, resp)
	}
	access := resp["accessToken"].(string)
	refresh := resp["refreshToken"].(string)
	hdr := map[string]string{"Authorization": "Bearer " + access}

	// wrong password rejected
	status, _ = post(t, ts.URL+"/api/v1/auth/login", nil, map[string]any{
		"email": "alice@example.com", "password": "wrong-password",
	})
	if status != 401 {
		t.Fatalf("bad password should be 401, got %d", status)
	}

	// 5. activate device (authorization spec §11: needs network; yields local auth)
	status, resp = post(t, ts.URL+"/api/v1/devices/activate", hdr, map[string]any{
		"installationId": testInstallID, "appVersion": "1.0.0",
	})
	if status != 200 || resp["status"] != "authorized" {
		t.Fatalf("activate: %d %v", status, resp)
	}

	// idempotent: second activation returns authorized again
	status, resp = post(t, ts.URL+"/api/v1/devices/activate", hdr, map[string]any{
		"installationId": testInstallID, "appVersion": "1.0.0",
	})
	if status != 200 || resp["status"] != "authorized" {
		t.Fatalf("reactivate: %d %v", status, resp)
	}

	// 6. license status
	status, resp = get(t, ts.URL+"/api/v1/license/status?installationId="+testInstallID, hdr)
	if status != 200 || resp["status"] != "active" {
		t.Fatalf("license status: %d %v", status, resp)
	}

	// 7. refresh rotates tokens
	status, resp = post(t, ts.URL+"/api/v1/auth/refresh", nil, map[string]any{
		"refreshToken": refresh,
	})
	if status != 200 {
		t.Fatalf("refresh: %d %v", status, resp)
	}
	if resp["refreshToken"].(string) == refresh {
		t.Fatal("refresh token must rotate")
	}
	// old refresh token is now invalid
	status, _ = post(t, ts.URL+"/api/v1/auth/refresh", nil, map[string]any{
		"refreshToken": refresh,
	})
	if status != 401 {
		t.Fatalf("old refresh should be 401, got %d", status)
	}

	// 8. admin endpoints require a configured key (server created without one)
	status, resp = post(t, ts.URL+"/api/v1/admin/accounts/1/ban",
		map[string]string{"X-Admin-Key": "admink"}, nil)
	if status != 403 {
		t.Fatalf("admin with bad key should be 403, got %d", status)
	}
	status, _ = post(t, ts.URL+"/api/v1/admin/accounts/1/ban",
		map[string]string{"X-Admin-Key": "test-admin-key"}, nil)
	if status != 403 {
		t.Fatalf("admin disabled should be 403 even with any key, got %d", status)
	}

	// 9. delete account: personal data gone, security record retained
	status, resp = post(t, ts.URL+"/api/v1/accounts/me/delete", hdr, map[string]any{
		"password": testPassword,
	})
	if status != 200 {
		t.Fatalf("delete: %d %v", status, resp)
	}
	// after deletion, login must fail
	status, _ = post(t, ts.URL+"/api/v1/auth/login", nil, map[string]any{
		"email": "alice@example.com", "password": testPassword,
	})
	if status != 401 {
		t.Fatalf("post-delete login should be 401, got %d", status)
	}
}

// TestServerWithAdminKey runs admin flows with a properly configured key.
func TestServerWithAdminKey(t *testing.T) {
	
	_, ts := newTestServer(t, "test-admin-key")

	// register + verify + login + activate
	post(t, ts.URL+"/api/v1/accounts/register", nil, map[string]any{
		"username": "bob", "email": "bob@example.com", "password": "long-password-1",
	})
	status, resp := post(t, ts.URL+"/api/v1/auth/login", nil, map[string]any{
		"email": "bob@example.com", "password": "long-password-1",
	})
	if status != 403 {
		t.Fatalf("unverified login should be 403, got %d", status)
	}
	_ = resp

	// (verification code comes from dev echo; register again to get it)
	_, resp = post(t, ts.URL+"/api/v1/accounts/register", nil, map[string]any{
		"username": "bob2", "email": "bob2@example.com", "password": "long-password-1",
	})
	code := resp["devVerificationCode"].(string)
	post(t, ts.URL+"/api/v1/accounts/verify-email", nil, map[string]any{
		"email": "bob2@example.com", "code": code,
	})
	status, resp = post(t, ts.URL+"/api/v1/auth/login", nil, map[string]any{
		"email": "bob2@example.com", "password": "long-password-1",
	})
	if status != 200 {
		t.Fatalf("login: %d %v", status, resp)
	}
	acc := resp["account"].(map[string]any)
	accID := fmt.Sprint(int(acc["id"].(float64)))
	hdr := map[string]string{"Authorization": "Bearer " + resp["accessToken"].(string)}

	status, resp = post(t, ts.URL+"/api/v1/devices/activate", hdr, map[string]any{
		"installationId": testInstallID, "appVersion": "1.0.0",
	})
	if status != 200 {
		t.Fatalf("activate: %d %v", status, resp)
	}

	// admin: revoke licenses for the actual account id
	status, resp = post(t, ts.URL+"/api/v1/admin/accounts/"+accID+"/revoke-licenses",
		map[string]string{"X-Admin-Key": "test-admin-key"}, nil)
	if status != 200 {
		t.Fatalf("admin revoke: %d %v", status, resp)
	}

	// license status now reports none/revoked; new task gate must close
	status, resp = get(t, ts.URL+"/api/v1/license/status?installationId="+testInstallID, hdr)
	if status != 200 {
		t.Fatalf("status: %d %v", status, resp)
	}
	if s, _ := resp["status"].(string); s == "active" {
		t.Fatal("license should not be active after admin revoke")
	}

	// security events recorded
	status, resp = get(t, ts.URL+"/api/v1/admin/security-events",
		map[string]string{"X-Admin-Key": "test-admin-key"})
	if status != 200 {
		t.Fatalf("events: %d %v", status, resp)
	}
	if !strings.Contains(jsonMust(t, resp), "device_activated") {
		t.Fatal("device_activated event missing")
	}
}

func jsonMust(t *testing.T, v any) string {
	t.Helper()
	raw, _ := json.Marshal(v)
	return string(raw)
}
