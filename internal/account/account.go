// Package account is the client side of the VideoDelite account service
// (Phase 10). It owns tokens, the device activation flow and the sync that
// feeds the authorization state machine.
//
// Boundary rules enforced here:
//   - tokens/secrets live in Windows Credential Manager, never plaintext
//   - the non-sensitive authorization cache lives in local SQLite
//   - nothing video-related ever reaches this package
package account

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"videodelite/internal/auth"
)

const (
	defaultBaseURL    = "https://api.videodelite.app"
	credName          = "VideoDelite/account"
	installationIDLen = 16
	requestTimeout    = 15 * time.Second
)

// Service drives the authorization state machine from server interactions.
type Service struct {
	http  *http.Client
	auth  *auth.Service
	db    *sql.DB
	base  string
	token *tokens // in-memory; persisted via credentials store

	mu     sync.Mutex
	cancel context.CancelFunc
}

type tokens struct {
	Access       string `json:"accessToken"`
	Refresh      string `json:"refreshToken"`
	Email        string `json:"email"`
	FetchedAt    time.Time `json:"fetchedAt"`
}

// Cache is the non-sensitive local authorization state (spec §19).
type Cache struct {
	InstallationID   string `json:"installationId"`
	AccountID        int64  `json:"accountId"`
	Email            string `json:"email"`
	LicenseType      string `json:"licenseType"`
	AuthorizedAt     string `json:"authorizedAt"`
	LastAuthorizedAt string `json:"lastAuthorizedAt"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
	LastSyncAt       string `json:"lastSyncAt,omitempty"`
}

func NewService(authSvc *auth.Service, db *sql.DB, serverURL string) *Service {
	base := serverURL
	if base == "" {
		base = defaultBaseURL
	}
	return &Service{
		http: &http.Client{Timeout: requestTimeout},
		auth: authSvc,
		db:   db,
		base: base,
	}
}

// SetBaseURL switches the API endpoint (used when the user edits the
// account-server address in Settings without restarting).
func (s *Service) SetBaseURL(url string) {
	if url != "" {
		s.base = url
	}
}

// ---- installation id (random, never a hardware fingerprint — spec §18) ----

// EnsureInstallationID returns the stored Installation ID, creating one on
// first use.
func (s *Service) EnsureInstallationID() (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'installation_id'`).Scan(&id)
	if err == nil && id != "" {
		return id, nil
	}
	id = randomHex(installationIDLen)
	_, err = s.db.Exec(`INSERT INTO settings(key, value) VALUES('installation_id', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = randRead(b)
	return hexEncode(b)
}

// ---- public operations ----

func (s *Service) Register(username, email, password, inviteCode string) (devCode string, err error) {
	body := map[string]string{
		"username": username, "email": email, "password": password, "inviteCode": inviteCode,
	}
	var resp struct {
		DevVerificationCode string `json:"devVerificationCode"`
		Error               string `json:"error"`
	}
	if _, err := s.do(ctx_bg(), "POST", "/api/v1/accounts/register", "", body, &resp); err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", errors.New(resp.Error)
	}
	return resp.DevVerificationCode, nil
}

func (s *Service) VerifyEmail(email, code string) error {
	var resp struct{ Error string `json:"error"` }
	_, err := s.do(ctx_bg(), "POST", "/api/v1/accounts/verify-email", "",
		map[string]string{"email": email, "code": code}, &resp)
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

// Login authenticates, stores credentials securely, then activates the
// device (authorization spec §11: activation needs network).
func (s *Service) Login(email, password string) (auth.Snapshot, error) {
	var resp struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		Error        string `json:"error"`
		Account      struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			Email    string `json:"email"`
		} `json:"account"`
	}
	code, err := s.do(ctx_bg(), "POST", "/api/v1/auth/login", "",
		map[string]string{"email": email, "password": password}, &resp)
	if err != nil {
		return s.auth.Snapshot(), fmt.Errorf("无法连接授权服务器")
	}
	if code == 401 || code == 403 {
		return s.auth.Snapshot(), errors.New(resp.Error)
	}
	if resp.Error != "" {
		return s.auth.Snapshot(), errors.New(resp.Error)
	}

	tok := &tokens{
		Access: resp.AccessToken, Refresh: resp.RefreshToken,
		Email: resp.Account.Email, FetchedAt: time.Now(),
	}
	if err := s.saveTokens(tok); err != nil {
		return s.auth.Snapshot(), fmt.Errorf("凭据保存失败: %w", err)
	}
	s.mu.Lock()
	s.token = tok
	s.mu.Unlock()

	s.auth.SetState(auth.StateAuthenticated, resp.Account.Email)

	// device activation (needs network, once)
	snap, aerr := s.activate()
	if aerr != nil {
		// Authenticated but activation failed: stay AUTHENTICATED, Simple unaffected.
		return s.auth.Snapshot(), nil
	}
	return snap, nil
}

func (s *Service) activate() (auth.Snapshot, error) {
	instID, err := s.EnsureInstallationID()
	if err != nil {
		return s.auth.Snapshot(), err
	}
	tok, err := s.currentTokens()
	if err != nil {
		return s.auth.Snapshot(), err
	}
	var resp struct {
		Status        string `json:"status"`
		LicenseType   string `json:"licenseType"`
		AuthorizedAt  string `json:"authorizedAt"`
		ExpiresAt     string `json:"expiresAt"`
		Error         string `json:"error"`
	}
	code, err := s.do(ctx_bg(), "POST", "/api/v1/devices/activate", tok.Access,
		map[string]string{"installationId": instID, "appVersion": appVersion}, &resp)
	if err != nil {
		return s.auth.Snapshot(), err
	}
	if code == 401 {
		// try one refresh round
		if rerr := s.refresh(); rerr == nil {
			tok2, _ := s.currentTokens()
			code, err = s.do(ctx_bg(), "POST", "/api/v1/devices/activate", tok2.Access,
				map[string]string{"installationId": instID, "appVersion": appVersion}, &resp)
			if err != nil {
				return s.auth.Snapshot(), err
			}
		}
	}
	if code != 200 || resp.Error != "" {
		return s.auth.Snapshot(), errors.New(resp.Error)
	}
	if resp.Status == "authorized" {
		_ = s.saveCache(&Cache{
			InstallationID:   instID,
			Email:            tok.Email,
			LicenseType:      resp.LicenseType,
			AuthorizedAt:     resp.AuthorizedAt,
			LastAuthorizedAt: nowStr(),
			ExpiresAt:        resp.ExpiresAt,
			LastSyncAt:       nowStr(),
		})
		s.auth.SetState(auth.StateAuthorized, tok.Email)
	}
	return s.auth.Snapshot(), nil
}

// Logout clears tokens and the local cache; Professional locks (spec §22).
// Local videos/history/settings are never touched.
func (s *Service) Logout() auth.Snapshot {
	if tok, err := s.currentTokens(); err == nil && tok.Refresh != "" {
		_, _ = s.do(ctx_bg(), "POST", "/api/v1/auth/logout", tok.Access,
			map[string]string{"refreshToken": tok.Refresh}, nil)
	}
	_ = deleteCredentials()
	_ = s.saveCache(nil)
	s.mu.Lock()
	s.token = nil
	s.mu.Unlock()
	s.auth.SetState(auth.StateUnauthenticated, "")
	return s.auth.Snapshot()
}

// DeleteAccount removes server-side personal data. Local videos, outputs,
// history and settings are untouched (boundary spec §24).
func (s *Service) DeleteAccount(password string) error {
	tok, err := s.currentTokens()
	if err != nil {
		return err
	}
	var resp struct{ Error string `json:"error"` }
	code, err := s.do(ctx_bg(), "POST", "/api/v1/accounts/me/delete", tok.Access,
		map[string]string{"password": password}, &resp)
	if err != nil {
		return fmt.Errorf("无法连接授权服务器")
	}
	if code != 200 || resp.Error != "" {
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		return fmt.Errorf("删除失败 (HTTP %d)", code)
	}
	s.Logout()
	return nil
}

// Startup restores offline state from the local cache (authorization spec
// §21): valid cache → AUTHORIZED/OFFLINE_AUTHORIZED, then a background sync
// corrects to REVOKED/EXPIRED if the server says so. Network problems never
// block the app (spec §9/§10).
func (s *Service) Startup() {
	cache := s.loadCache()
	if cache == nil || cache.InstallationID == "" {
		return
	}
	s.auth.SetState(auth.StateOfflineAuthorized, cache.Email)

	ctx, cancel := context.WithCancel(ctx_bg())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		select {
		case <-time.After(2 * time.Second): // let the UI settle first
		case <-ctx.Done():
			return
		}
		s.Sync()
	}()
}

// Sync checks license status with the server and corrects local state.
// Failures keep OFFLINE_AUTHORIZED (spec: one failed HTTP request must not
// lock Professional).
func (s *Service) Sync() {
	cache := s.loadCache()
	if cache == nil {
		return
	}
	tok, err := s.currentTokens()
	if err != nil {
		return // offline: stay in OFFLINE_AUTHORIZED
	}
	instID := cache.InstallationID
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	code, err := s.do(ctx_bg(), "GET", "/api/v1/license/status?installationId="+instID, tok.Access, nil, &resp)
	if err != nil {
		return // offline
	}
	if code == 401 {
		if rerr := s.refresh(); rerr == nil {
			tok2, _ := s.currentTokens()
			code, err = s.do(ctx_bg(), "GET", "/api/v1/license/status?installationId="+instID, tok2.Access, nil, &resp)
			if err != nil {
				return
			}
		} else {
			return
		}
	}
	if code != 200 {
		return
	}
	switch resp.Status {
	case "active":
		cache.LastSyncAt = nowStr()
		_ = s.saveCache(cache)
		s.auth.SetState(auth.StateAuthorized, cache.Email)
	case "revoked":
		s.auth.SetState(auth.StateRevoked, cache.Email)
	case "expired":
		s.auth.SetState(auth.StateAuthorizationExpired, cache.Email)
	case "none":
		s.auth.SetState(auth.StateRevoked, cache.Email)
	}
}

// ---- internals ----

func (s *Service) currentTokens() (*tokens, error) {
	s.mu.Lock()
	t := s.token
	s.mu.Unlock()
	if t != nil {
		return t, nil
	}
	t, err := loadCredentials()
	if err != nil {
		return nil, errors.New("not logged in")
	}
	s.mu.Lock()
	s.token = t
	s.mu.Unlock()
	return t, nil
}

func (s *Service) saveTokens(t *tokens) error {
	if err := storeCredentials(t); err != nil {
		return err
	}
	return nil
}

func (s *Service) refresh() error {
	tok, err := s.currentTokens()
	if err != nil {
		return err
	}
	var resp struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		Error        string `json:"error"`
	}
	code, err := s.do(ctx_bg(), "POST", "/api/v1/auth/refresh", "",
		map[string]string{"refreshToken": tok.Refresh}, &resp)
	if err != nil || code != 200 || resp.Error != "" {
		// refresh failed for good: log out silently
		s.Logout()
		return errors.New("refresh failed")
	}
	nt := &tokens{Access: resp.AccessToken, Refresh: resp.RefreshToken, Email: tok.Email, FetchedAt: time.Now()}
	_ = s.saveTokens(nt)
	s.mu.Lock()
	s.token = nt
	s.mu.Unlock()
	return nil
}

func (s *Service) do(ctx context.Context, method, path, bearer string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VideoDelite/"+appVersion)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode, nil
}

// ---- cache persistence (settings table, non-sensitive) ----

func (s *Service) saveCache(c *Cache) error {
	if c == nil {
		_, err := s.db.Exec(`DELETE FROM settings WHERE key = 'auth_cache'`)
		return err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO settings(key, value) VALUES('auth_cache', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(raw))
	return err
}

func (s *Service) loadCache() *Cache {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'auth_cache'`).Scan(&raw)
	if err != nil {
		return nil
	}
	var c Cache
	if json.Unmarshal([]byte(raw), &c) != nil {
		return nil
	}
	return &c
}
