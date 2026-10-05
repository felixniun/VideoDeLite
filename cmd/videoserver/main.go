// VideoDelite account server (Phase 10) — entry point with config file.
//
// Usage:
//
//	videoserver -config config.json
//
// See config.json / config.example.json for all options. Environment
// overrides: VIDEODELITE_DB_CONN, VIDEODELITE_ADMIN_KEY, VIDEODELITE_LISTEN.
package main

import (
	"log"
	"net/http"
	"time"

	"videodelite/server"
)

func main() {
	cfg, err := loadConfig()
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

	srv := server.New(st, server.Options{
		InviteRequired: cfg.InviteRequired,
		DevEchoMail:    cfg.DevEchoMail,
		AccessTTL:      time.Duration(cfg.AccessTTLMin) * time.Minute,
		RefreshTTL:     time.Duration(cfg.RefreshTTLDays) * 24 * time.Hour,
		JWTSecretFile:  cfg.JWTSecretFile,
		AdminKey:       cfg.AdminKey,
		Mailer:         buildMailer(cfg.Mail),
	})

	switch {
	case cfg.Mail.Provider == "resend":
		log.Printf("mail: Resend API (from %s)", cfg.Mail.From)
	case cfg.Mail.Provider == "smtp":
		log.Printf("mail: SMTP %s:%d", cfg.Mail.SMTP.Host, cfg.Mail.SMTP.Port)
	default:
		log.Printf("mail: DEV ECHO mode — verification codes are returned in responses, NOT emailed")
	}

	log.Printf("VideoDelite account server listening on %s (db: sql server)", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}

// buildMailer selects the verification-email sender. nil = dev-echo.
func buildMailer(m MailConfig) server.Mailer {
	switch m.Provider {
	case "resend":
		return server.NewResendMailer(m.APIKey, m.From)
	case "smtp":
		return server.NewSMTPMailer(m.SMTP)
	}
	return nil
}
