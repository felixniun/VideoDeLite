package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Options struct {
	InviteRequired bool
	DevEchoMail    bool
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	JWTSecretFile  string // persistent HMAC secret location
	AdminKey       string // plaintext admin key (hashed in memory)
	Mailer         Mailer // nil = dev-echo mode (code returned in response)
}

type Server struct {
	store  Store
	opts   Options
	secret []byte

	mu          sync.Mutex
	mailCodes   map[int64]string // dev-echo only: accountID -> code
	adminKeySHA string
}

func New(st Store, opts Options) *Server {
	s := &Server{store: st, opts: opts, mailCodes: map[int64]string{}}
	// JWT secret: persistent file so restarts don't invalidate tokens.
	secretPath := opts.JWTSecretFile
	if secretPath == "" {
		secretPath = os.Getenv("VIDEODELITE_JWT_SECRET_FILE")
	}
	if secretPath == "" {
		secretPath = "jwt.secret"
	}
	if data, err := os.ReadFile(secretPath); err == nil && len(data) >= 32 {
		s.secret = data
	} else {
		s.secret = []byte(randomToken(32))
		_ = os.WriteFile(secretPath, s.secret, 0o600)
	}
	if opts.AdminKey != "" {
		h := sha256.Sum256([]byte(opts.AdminKey))
		s.adminKeySHA = hexEncode(h[:])
	}
	return s
}

func hexEncode(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0xF]
	}
	return string(out)
}

// Routes returns the HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/accounts/register", s.handleRegister)
	mux.HandleFunc("POST /api/v1/accounts/verify-email", s.handleVerifyEmail)
	mux.HandleFunc("POST /api/v1/accounts/resend-verification", s.handleResendVerification)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/refresh", s.handleRefresh)
	mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	mux.HandleFunc("POST /api/v1/devices/activate", s.authed(s.handleActivate))
	mux.HandleFunc("GET /api/v1/devices", s.authed(s.handleListDevices))
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.authed(s.handleDeactivateDevice))
	mux.HandleFunc("GET /api/v1/license/status", s.authed(s.handleLicenseStatus))
	mux.HandleFunc("POST /api/v1/accounts/me/delete", s.authed(s.handleDeleteAccount))
	mux.HandleFunc("GET /api/v1/version", s.handleVersion)

	// admin
	mux.HandleFunc("GET /admin", s.handleAdminUI)
	mux.HandleFunc("GET /api/v1/admin/stats", s.admin(s.handleAdminStats))
	mux.HandleFunc("GET /api/v1/admin/accounts", s.admin(s.handleAdminAccounts))
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/ban", s.admin(s.handleAdminBan(true)))
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/unban", s.admin(s.handleAdminBan(false)))
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/revoke-licenses", s.admin(s.handleAdminRevoke))
	mux.HandleFunc("GET /api/v1/admin/security-events", s.admin(s.handleAdminEvents))

	return mux
}

// ---- helpers ----

type jsonBody map[string]any

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, jsonBody{"error": msg})
}

func readBody(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

type authedHandler func(w http.ResponseWriter, r *http.Request, accountID int64)

func (s *Server) authed(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.bearerAuth(r)
		if err != nil {
			fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		acc, err := s.store.GetAccountByID(r.Context(), id)
		if err != nil || acc.Status == "banned" || acc.Status == "deleted" {
			fail(w, http.StatusForbidden, "account unavailable")
			return
		}
		h(w, r, id)
	}
}

func (s *Server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminKeySHA == "" {
			fail(w, http.StatusForbidden, "admin disabled")
			return
		}
		key := r.Header.Get("X-Admin-Key")
		hk := sha256.Sum256([]byte(key))
		if hexEncode(hk[:]) != s.adminKeySHA {
			fail(w, http.StatusForbidden, "invalid admin key")
			return
		}
		h(w, r)
	}
}

// ---- registration & email verification ----

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username   string `json:"username"`
		Email      string `json:"email"`
		Password   string `json:"password"`
		InviteCode string `json:"inviteCode"`
	}
	if err := readBody(r, &req); err != nil {
		fail(w, 400, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if len(req.Username) < 3 || len(req.Username) > 64 {
		fail(w, 400, "username must be 3-64 characters")
		return
	}
	if !strings.Contains(req.Email, "@") || len(req.Email) > 254 {
		fail(w, 400, "invalid email")
		return
	}
	if len(req.Password) < 8 {
		fail(w, 400, "password must be at least 8 characters")
		return
	}

	required, err := s.store.InviteRequired(r.Context())
	if err == nil && (required || s.opts.InviteRequired) {
		if req.InviteCode == "" {
			fail(w, 400, "invite code required")
			return
		}
		if err := s.store.ConsumeInviteCode(r.Context(), req.InviteCode); err != nil {
			fail(w, 400, err.Error())
			return
		}
	}

	id, err := s.store.CreateAccount(r.Context(), req.Username, req.Email, hashPassword(req.Password))
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	code := s.issueEmailCode(r.Context(), id)
	_ = s.store.LogSecurityEvent(r.Context(), id, "register", req.Email)

	resp := jsonBody{"accountId": id, "message": "verification code sent"}
	if err := s.deliverCode(r.Context(), req.Email, code); err != nil {
		// Account exists; the user can retry via resend-verification.
		resp["warning"] = "验证码邮件发送失败，请稍后使用 resend-verification 重发"
		resp["sendError"] = err.Error()
		writeJSON(w, 201, resp)
		return
	}
	if s.opts.DevEchoMail && s.opts.Mailer == nil {
		resp["devVerificationCode"] = code
	}
	writeJSON(w, 201, resp)
}

// deliverCode sends the code through the configured Mailer (Resend/SMTP).
// In dev-echo mode without a Mailer it's a no-op (code returned in response).
func (s *Server) deliverCode(ctx context.Context, email, code string) error {
	if s.opts.Mailer == nil {
		return nil
	}
	return s.opts.Mailer.SendVerificationCode(email, code)
}

var resendThrottle = newEmailThrottle(time.Minute)

// handleResendVerification re-issues the code (60s throttle per email).
func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := readBody(r, &req); err != nil {
		fail(w, 400, "invalid request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") {
		fail(w, 400, "invalid email")
		return
	}
	if !resendThrottle.allow(email) {
		fail(w, 429, "发送过于频繁，请一分钟后再试")
		return
	}
	acc, err := s.store.GetAccountByEmail(r.Context(), email)
	if err != nil {
		// Do not reveal whether the address is registered.
		writeJSON(w, 200, jsonBody{"message": "如果该邮箱已注册，验证码已发送"})
		return
	}
	if acc.EmailVerified {
		writeJSON(w, 200, jsonBody{"message": "该邮箱已验证，可直接登录"})
		return
	}
	code := s.issueEmailCode(r.Context(), acc.ID)
	_ = s.store.LogSecurityEvent(r.Context(), acc.ID, "resend_verification", "")
	if err := s.deliverCode(r.Context(), email, code); err != nil {
		fail(w, 502, "验证码邮件发送失败，请稍后重试")
		return
	}
	writeJSON(w, 200, jsonBody{"message": "如果该邮箱已注册，验证码已发送"})
}

// emailThrottle is a minimal per-key sliding window limiter.
type emailThrottle struct {
	mu   sync.Mutex
	ttl  time.Duration
	last map[string]time.Time
}

func newEmailThrottle(ttl time.Duration) *emailThrottle {
	return &emailThrottle{ttl: ttl, last: map[string]time.Time{}}
}

func (t *emailThrottle) allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if when, ok := t.last[key]; ok && time.Since(when) < t.ttl {
		return false
	}
	t.last[key] = time.Now()
	return true
}

// issueEmailCode creates a 6-digit code token. Delivery goes through the
// configured Mailer (Resend/SMTP); dev-echo mode returns it in the response.
func (s *Server) issueEmailCode(ctx context.Context, accountID int64) string {
	code := randomCode6()
	_ = s.store.PutEmailToken(ctx, accountID, code, time.Now().Add(emailTokenTTL))
	if s.opts.DevEchoMail {
		s.mu.Lock()
		s.mailCodes[accountID] = code
		s.mu.Unlock()
	}
	return code
}

func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := readBody(r, &req); err != nil {
		fail(w, 400, "invalid request body")
		return
	}
	acc, err := s.store.GetAccountByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		fail(w, 404, "account not found")
		return
	}
	if err := s.store.ConsumeEmailToken(r.Context(), acc.ID, strings.TrimSpace(req.Code)); err != nil {
		fail(w, 400, err.Error())
		return
	}
	_ = s.store.LogSecurityEvent(r.Context(), acc.ID, "email_verified", acc.Email)
	writeJSON(w, 200, jsonBody{"verified": true})
}

// ---- login / refresh / logout ----

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readBody(r, &req); err != nil {
		fail(w, 400, "invalid request body")
		return
	}
	acc, err := s.store.GetAccountByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if errors.Is(err, ErrNotFound) {
		fail(w, 401, "invalid email or password")
		return
	}
	if err != nil {
		fail(w, 500, "store error")
		return
	}
	if !verifyPassword(req.Password, acc.PasswordHash) {
		_ = s.store.LogSecurityEvent(r.Context(), acc.ID, "login_failed", "")
		fail(w, 401, "invalid email or password")
		return
	}
	if acc.Status == "banned" {
		_ = s.store.LogSecurityEvent(r.Context(), acc.ID, "login_banned", "")
		fail(w, 403, "account banned")
		return
	}
	if acc.Status == "deleted" {
		fail(w, 401, "invalid email or password")
		return
	}
	if !acc.EmailVerified {
		fail(w, 403, "email not verified")
		return
	}

	access, err := s.signAccessToken(acc.ID, acc.Username, s.opts.AccessTTL)
	if err != nil {
		fail(w, 500, "token signing failed")
		return
	}
	refresh := randomToken(32)
	if err := s.store.CreateRefreshToken(r.Context(), acc.ID, hashOpaque(refresh), time.Now().Add(s.opts.RefreshTTL)); err != nil {
		fail(w, 500, "refresh token storage failed")
		return
	}
	_ = s.store.LogSecurityEvent(r.Context(), acc.ID, "login", "")

	writeJSON(w, 200, jsonBody{
		"accessToken":  access,
		"refreshToken": refresh,
		"expiresIn":    int(s.opts.AccessTTL.Seconds()),
		"account": jsonBody{
			"id": acc.ID, "username": acc.Username, "email": acc.Email,
		},
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := readBody(r, &req); err != nil || req.RefreshToken == "" {
		fail(w, 400, "invalid request body")
		return
	}
	hash := hashOpaque(req.RefreshToken)
	accID, expires, revoked, err := s.store.GetRefreshToken(r.Context(), hash)
	if err != nil || revoked || time.Now().After(expires) {
		fail(w, 401, "invalid refresh token")
		return
	}
	acc, err := s.store.GetAccountByID(r.Context(), accID)
	if err != nil || acc.Status != "active" {
		fail(w, 403, "account unavailable")
		return
	}
	// rotation: old token is revoked, a new one is issued
	if err := s.store.RevokeRefreshToken(r.Context(), hash); err != nil {
		fail(w, 500, "rotation failed")
		return
	}
	access, err := s.signAccessToken(acc.ID, acc.Username, s.opts.AccessTTL)
	if err != nil {
		fail(w, 500, "token signing failed")
		return
	}
	refresh := randomToken(32)
	if err := s.store.CreateRefreshToken(r.Context(), acc.ID, hashOpaque(refresh), time.Now().Add(s.opts.RefreshTTL)); err != nil {
		fail(w, 500, "refresh token storage failed")
		return
	}
	writeJSON(w, 200, jsonBody{
		"accessToken": access, "refreshToken": refresh, "expiresIn": int(s.opts.AccessTTL.Seconds()),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = readBody(r, &req)
	if req.RefreshToken != "" {
		_ = s.store.RevokeRefreshToken(r.Context(), hashOpaque(req.RefreshToken))
	}
	writeJSON(w, 200, jsonBody{"ok": true})
}

// ---- devices & license ----

type activationResponse struct {
	Status       string     `json:"status"`
	InstallationID string   `json:"installationId"`
	LicenseType  string     `json:"licenseType"`
	AuthorizedAt string     `json:"authorizedAt"`
	ExpiresAt    string     `json:"expiresAt,omitempty"`
}

func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request, accountID int64) {
	var req struct {
		InstallationID string `json:"installationId"`
		AppVersion     string `json:"appVersion"`
	}
	if err := readBody(r, &req); err != nil || req.InstallationID == "" {
		fail(w, 400, "installationId required")
		return
	}
	if len(req.InstallationID) > 64 {
		fail(w, 400, "invalid installationId")
		return
	}
	acc, err := s.store.GetAccountByID(r.Context(), accountID)
	if err != nil {
		fail(w, 403, "account unavailable")
		return
	}
	if acc.Status == "banned" {
		_ = s.store.LogSecurityEvent(r.Context(), accountID, "activation_banned", req.InstallationID)
		fail(w, 403, "account banned")
		return
	}

	dev, err := s.store.UpsertDevice(r.Context(), accountID, req.InstallationID, req.AppVersion)
	if err != nil {
		fail(w, 500, "device upsert failed")
		return
	}
	if dev.Status == "revoked" {
		fail(w, 403, "device revoked")
		return
	}

	// Existing active license wins (idempotent activation).
	if _, err := s.store.GetActiveLicense(r.Context(), accountID, dev.ID); err == nil {
		_ = s.store.TouchDevice(r.Context(), dev.ID, req.AppVersion)
		writeJSON(w, 200, activationResponse{
			Status: "authorized", InstallationID: req.InstallationID,
			LicenseType: "professional", AuthorizedAt: ts(time.Now()),
		})
		return
	}

	// Issue a new professional license (V1 policy: every verified account
	// gets one; replace with purchase/entitlement flow later).
	var expires *time.Time
	licID, err := s.store.CreateLicense(r.Context(), accountID, dev.ID, expires)
	if err != nil {
		fail(w, 500, "license creation failed")
		return
	}
	_ = s.store.LogSecurityEvent(r.Context(), accountID, "device_activated",
		strconv.FormatInt(licID, 10))

	writeJSON(w, 200, activationResponse{
		Status: "authorized", InstallationID: req.InstallationID,
		LicenseType: "professional", AuthorizedAt: ts(time.Now()),
	})
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request, accountID int64) {
	devs, err := s.store.ListDevices(r.Context(), accountID)
	if err != nil {
		fail(w, 500, "list failed")
		return
	}
	out := make([]jsonBody, 0, len(devs))
	for _, d := range devs {
		out = append(out, jsonBody{
			"id": d.ID, "installationId": d.InstallationID, "appVersion": d.AppVersion,
			"status": d.Status, "createdAt": ts(d.CreatedAt), "lastSeen": ts(d.LastSeen),
		})
	}
	writeJSON(w, 200, jsonBody{"devices": out})
}

func (s *Server) handleDeactivateDevice(w http.ResponseWriter, r *http.Request, accountID int64) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, 400, "invalid device id")
		return
	}
	devs, err := s.store.ListDevices(r.Context(), accountID)
	if err != nil {
		fail(w, 500, "list failed")
		return
	}
	mine := false
	for _, d := range devs {
		if d.ID == id {
			mine = true
			break
		}
	}
	if !mine {
		fail(w, 404, "device not found")
		return
	}
	_ = s.store.SetDeviceStatus(r.Context(), id, "revoked")
	writeJSON(w, 200, jsonBody{"ok": true})
}

func (s *Server) handleLicenseStatus(w http.ResponseWriter, r *http.Request, accountID int64) {
	installationID := r.URL.Query().Get("installationId")
	if installationID == "" {
		fail(w, 400, "installationId query param required")
		return
	}
	dev, err := s.store.GetDeviceByInstallation(r.Context(), accountID, installationID)
	if errors.Is(err, ErrNotFound) {
		writeJSON(w, 200, jsonBody{"status": "none"})
		return
	}
	if err != nil {
		fail(w, 500, "store error")
		return
	}
	status, expires, err := s.store.GetLicenseStatus(r.Context(), accountID, dev.ID)
	if err != nil {
		fail(w, 500, "store error")
		return
	}
	resp := jsonBody{"status": status}
	if expires != nil {
		resp["expiresAt"] = ts(*expires)
	}
	writeJSON(w, 200, resp)
}

// ---- account deletion (authorization spec §23) ----

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request, accountID int64) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readBody(r, &req); err != nil {
		fail(w, 400, "invalid request body")
		return
	}
	acc, err := s.store.GetAccountByID(r.Context(), accountID)
	if err != nil {
		fail(w, 404, "account not found")
		return
	}
	if !verifyPassword(req.Password, acc.PasswordHash) {
		fail(w, 401, "invalid password")
		return
	}
	if err := s.store.DeleteAccountPersonalData(r.Context(), accountID); err != nil {
		fail(w, 500, "deletion failed")
		return
	}
	_ = s.store.RevokeAllRefreshTokens(r.Context(), accountID)
	_ = s.store.LogSecurityEvent(r.Context(), 0, "account_deleted", acc.Email)
	writeJSON(w, 200, jsonBody{"ok": true})
}

// ---- misc / admin ----

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, jsonBody{"latest": "1.0.0", "channel": "stable"})
}

// handleAdminStats serves the dashboard aggregates (charts + cards).
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.AdminStats(r.Context())
	if err != nil {
		fail(w, 500, "stats failed: "+err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	accs, err := s.store.ListAccounts(r.Context(), 200)
	if err != nil {
		fail(w, 500, "list failed")
		return
	}
	writeJSON(w, 200, jsonBody{"accounts": accs})
}

func (s *Server) handleAdminBan(ban bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			fail(w, 400, "invalid id")
			return
		}
		status := "active"
		if ban {
			status = "banned"
		}
		if err := s.store.SetAccountStatus(r.Context(), id, status); err != nil {
			fail(w, 500, "update failed")
			return
		}
		if ban {
			_ = s.store.RevokeAllRefreshTokens(r.Context(), id)
			_ = s.store.LogSecurityEvent(r.Context(), id, "account_banned", "")
		}
		writeJSON(w, 200, jsonBody{"ok": true, "status": status})
	}
}

func (s *Server) handleAdminRevoke(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, 400, "invalid id")
		return
	}
	if err := s.store.RevokeAccountLicenses(r.Context(), id); err != nil {
		fail(w, 500, "revoke failed")
		return
	}
	_ = s.store.LogSecurityEvent(r.Context(), id, "license_revoked_by_admin", "")
	writeJSON(w, 200, jsonBody{"ok": true})
}

func (s *Server) handleAdminEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.ListSecurityEvents(r.Context(), 200)
	if err != nil {
		fail(w, 500, "list failed")
		return
	}
	writeJSON(w, 200, jsonBody{"events": events})
}
