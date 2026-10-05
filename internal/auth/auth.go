// Package auth implements the V1 authorization state machine skeleton.
//
// Frozen rules (authorization spec):
//   - Rule 01: Simple Mode needs no login
//   - Rule 02: Professional Mode needs a valid authorization
//   - Rule 03: Professional compression always runs locally
//   - Rule 14: authorization checks are never a FFmpeg runtime dependency
//
// V1 MVP ships without the account server (plan §120), so the service starts
// and stays in UNAUTHENTICATED: Simple fully usable, Professional locked.
// The state machine and local cache layout are already in place so Phase 10
// (Account) can plug in without touching the encoding domain.
package auth

import (
	"sync"
)

type State string

const (
	StateUnauthenticated      State = "UNAUTHENTICATED"
	StateAuthenticated        State = "AUTHENTICATED"
	StateActivating           State = "ACTIVATING"
	StateAuthorized           State = "AUTHORIZED"
	StateOfflineAuthorized    State = "OFFLINE_AUTHORIZED"
	StateAuthorizationExpired State = "AUTHORIZATION_EXPIRED"
	StateRevoked              State = "REVOKED"
	StateAccountDisabled      State = "ACCOUNT_DISABLED"
)

// LocalCache is the non-sensitive local authorization state (spec §19).
// Stored locally, no tokens: tokens belong in Windows Credential Manager.
type LocalCache struct {
	InstallationID   string `json:"installationId"`
	AccountID        string `json:"accountId"`
	LicenseType      string `json:"licenseType"`
	AuthorizedAt     string `json:"authorizedAt"`
	LastAuthorizedAt string `json:"lastAuthorizedAt"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
	LastSyncAt       string `json:"lastSyncAt"`
}

// Snapshot is what the UI renders.
type Snapshot struct {
	State              State  `json:"state"`
	AccountEmail       string `json:"accountEmail"`
	CanUseSimple       bool   `json:"canUseSimple"`
	CanUseProfessional bool   `json:"canUseProfessional"`
	Reason             string `json:"reason"`
}

type Service struct {
	mu     sync.Mutex
	state  State
	email  string
	cache  *LocalCache
}

func New() *Service {
	return &Service{state: StateUnauthenticated}
}

func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{
		State:        s.state,
		CanUseSimple: true, // Rule 01: always
	}
	switch s.state {
	case StateAuthorized, StateOfflineAuthorized:
		snap.CanUseProfessional = true
	case StateUnauthenticated:
		snap.Reason = "未登录：Professional 需要登录并获得授权（V1 暂未开放账户系统）"
	case StateAuthorizationExpired:
		snap.Reason = "授权已过期"
	case StateRevoked:
		snap.Reason = "授权已被撤销"
	case StateAccountDisabled:
		snap.Reason = "账户已被禁用"
	}
	return snap
}

// CanUseProfessional gates new Professional tasks only (Rule 10: changes
// affect future tasks, never a running FFmpeg).
func (s *Service) CanUseProfessional() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == StateAuthorized || s.state == StateOfflineAuthorized
}

// SetState is reserved for the Phase 10 account subsystem.
func (s *Service) SetState(st State, email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = st
	s.email = email
}
