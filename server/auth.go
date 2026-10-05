// Package server implements the VideoDelite account service.
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

const (
	tokenVersion    = 1
	emailTokenTTL   = 24 * time.Hour
	installationIDLen = 16
)

// ---- passwords: Argon2id (plan §69) ----

func hashPassword(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	// time=3, memory=64MB, threads=2 — reasonable desktop-server defaults
	h := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("argon2id$%d$%d$%d$%s$%s", 3, 64*1024, 32,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(h))
}

func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "argon2id" {
		return false
	}
	var timeCost, memKB, keyLen int
	if _, err := fmt.Sscanf(parts[1]+" "+parts[2]+" "+parts[3], "%d %d %d",
		&timeCost, &memKB, &keyLen); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(timeCost), uint32(memKB), 2, uint32(keyLen))
	return equalBytes(got, want)
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// ---- random helpers ----

func randomToken(bytesN int) string {
	b := make([]byte, bytesN)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomCode6() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	return fmt.Sprintf("%06d", n%1000000)
}

func hashOpaque(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ---- JWT access tokens ----

type accessClaims struct {
	AccountID int64  `json:"aid"`
	Username  string `json:"usr"`
	jwt.RegisteredClaims
}

func (s *Server) signAccessToken(accountID int64, username string, ttl time.Duration) (string, error) {
	claims := accessClaims{
		AccountID: accountID,
		Username:  username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "videodelite",
			Subject:   fmt.Sprint(accountID),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *Server) parseAccessToken(tokenStr string) (*accessClaims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &accessClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*accessClaims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// bearerAuth validates the Authorization header and returns the account id.
func (s *Server) bearerAuth(r *http.Request) (int64, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return 0, errors.New("missing bearer token")
	}
	claims, err := s.parseAccessToken(strings.TrimPrefix(h, "Bearer "))
	if err != nil {
		return 0, err
	}
	return claims.AccountID, nil
}
