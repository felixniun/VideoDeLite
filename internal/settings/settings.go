// Package settings persists local application settings in SQLite.
// Settings are strictly local (boundary spec §20): never synced to a server.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"

	"videodelite/internal/paths"
)

type Settings struct {
	Language         string `json:"language"`         // "" follow system | zh-CN | en
	Theme            string `json:"theme"`            // "" follow system | light | dark
	OutputDir        string `json:"outputDir"`
	ConflictStrategy string `json:"conflictStrategy"` // overwrite | skip | auto | cancel
	RememberConflict bool   `json:"rememberConflict"`
	DefaultCodec     string `json:"defaultCodec"` // h264 | h265
	HardwareEncode   bool   `json:"hardwareEncode"`
	FFmpegPath       string `json:"ffmpegPath"`  // optional override
	FFprobePath      string `json:"ffprobePath"` // optional override
	AccountServer    string `json:"accountServer"` // optional override of the account service URL
	ParallelTasks    int    `json:"parallelTasks"`  // 1 = sequential (default, plan §35); up to 3
}

func defaults() Settings {
	return Settings{
		Language:         "",
		Theme:            "",
		OutputDir:        paths.DefaultOutputDir(),
		ConflictStrategy: "auto",
		RememberConflict: false,
		DefaultCodec:     "h264",
		HardwareEncode:   true,
		// Production default: domain endpoint via Cloudflare (orange cloud).
		// Port 8080 is a Cloudflare-supported HTTP edge port while origin
		// TLS is not yet configured; switches to https://… (no port) once
		// acme.sh SSL lands. Legacy values migrate in Get().
		AccountServer: "https://videodelite1.898280.xyz:28443",
		ParallelTasks:    1,
	}
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const settingsKey = "app"

func (s *Store) Get() Settings {
	cur := defaults()
	row := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, settingsKey)
	var raw string
	if err := row.Scan(&raw); err != nil {
		return cur
	}
	var stored Settings
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return cur
	}
	// Merge over defaults so new fields get sane values across upgrades.
	if stored.Language != "" {
		cur.Language = stored.Language
	}
	if stored.Theme != "" {
		cur.Theme = stored.Theme
	}
	if stored.OutputDir != "" {
		cur.OutputDir = stored.OutputDir
	}
	if stored.ConflictStrategy != "" {
		cur.ConflictStrategy = stored.ConflictStrategy
	}
	cur.RememberConflict = stored.RememberConflict
	if stored.DefaultCodec != "" {
		cur.DefaultCodec = stored.DefaultCodec
	}
	cur.HardwareEncode = stored.HardwareEncode
	cur.FFmpegPath = stored.FFmpegPath
	cur.FFprobePath = stored.FFprobePath
	if stored.AccountServer != "" {
		cur.AccountServer = stored.AccountServer
	}
	// One-time migration: legacy hardcoded IP/local defaults move to the
	// HTTPS domain endpoint (the API address is no longer exposed in the UI).
	switch cur.AccountServer {
	case "http://192.168.100.101:8800", "http://127.0.0.1:8800", "http://videodelite.898280.xyz:8800",
		"http://videodelite1.898280.xyz:8080", "https://videodelite1.898280.xyz:8443", "https://videodelite1.898280.xyz":
		cur.AccountServer = "https://videodelite1.898280.xyz:28443"
	}
	if stored.ParallelTasks > 0 {
		cur.ParallelTasks = stored.ParallelTasks
	}
	return cur
}

func (s *Store) Save(next Settings) error {
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		settingsKey, string(raw))
	return err
}
