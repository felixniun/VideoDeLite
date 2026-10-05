package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// appVersion is injected by the app layer to avoid an import cycle.
var appVersion = "1.0.0"

// SetAppVersion lets the host provide the version string.
func SetAppVersion(v string) { appVersion = v }

func ctx_bg() context.Context { return context.Background() }

func nowStr() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }

func randRead(b []byte) (int, error) { return rand.Read(b) }

func hexEncode(b []byte) string { return hex.EncodeToString(b) }

var _ = fmt.Sprintf
