// VideoDelite account server (Phase 10) — entry point with Viper config.
//
// Usage:
//
//	videoserver -config config.json
//
// Config precedence: VIDEODELITE_* env vars > config.json > defaults.
// Hot reload: editing config.json applies admin key / mail / invite flag
// at runtime (see config.go watch); listen and DB connection need restart.
package main

import (
	"log"
	"net/http"
	"time"

	"videodelite/server"
)

func main() {
	cfg, watch, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.DB.Connection == "" {
		log.Fatal("db.connection is required (config.json or VIDEODELITE_DB_CONN)")
	}
	if cfg.DB.AutoCreateDB {
		if err := ensureDatabase(cfg.DB.Connection); err != nil {
			log.Printf("database auto-create skipped: %v", err)
		}
	}
	st, err := server.OpenSQLServer(cfg.DB.Connection)
	if err != nil {
		log.Fatalf("sql server: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	_, mailer := cfg.runtimeOptions()
	srv := server.New(st, server.Options{
		InviteRequired: cfg.InviteRequired,
		DevEchoMail:    cfg.DevEchoMail,
		AccessTTL:      time.Duration(cfg.AccessTTLMin) * time.Minute,
		RefreshTTL:     time.Duration(cfg.RefreshTTLDays) * 24 * time.Hour,
		JWTSecretFile:  cfg.JWTSecretFile,
		AdminKey:       cfg.AdminKey,
		Mailer:         mailer,
	})
	// Hot reload: config.json edits apply admin key / mail / invite flag live.
	watch(srv)

	switch cfg.Mail.Provider {
	case "resend":
		log.Printf("mail: Resend API (from %s)", cfg.Mail.From)
	case "smtp":
		log.Printf("mail: SMTP %s:%d", cfg.Mail.SMTP.Host, cfg.Mail.SMTP.Port)
	default:
		log.Printf("mail: DEV ECHO mode — verification codes are returned in responses, NOT emailed")
	}

	log.Printf("VideoDelite account server listening on %s (db: sql server)", cfg.Listen)
	hs := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := hs.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
