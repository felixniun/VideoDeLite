package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"

	"videodelite/server"
)

// Config is the server configuration (config.json), loaded through Viper.
//
// Precedence (highest first):
//  1. environment variables (VIDEODELITE_*, bound explicitly below)
//  2. config.json
//  3. built-in defaults
//
// Hot reload: viper.WatchConfig() in main() applies compatible changes at
// runtime (admin key, mail provider/keys, invite flag) without restart.
// Restart-only fields: listen, db.connection (a live SQL pool cannot be
// swapped safely), jwtSecretFile.
//
// Deployment notes: keep production secrets out of source control. For a
// future public/internet deployment, only the values change — the binary
// is identical.
type MailConfig struct {
	Provider string     `mapstructure:"provider"`
	From     string     `mapstructure:"from"` // "VideoDelite <noreply@mail.yourdomain.com>"
	APIKey   string     `mapstructure:"apiKey"`
	SMTP     SMTPConfig `mapstructure:"smtp"`
}

type SMTPConfig = server.SMTPConfig

type DBConfig struct {
	Connection   string `mapstructure:"connection"`
	AutoCreateDB bool   `mapstructure:"autoCreateDatabase"`
}

type Config struct {
	Listen         string     `mapstructure:"listen"`
	DB             DBConfig   `mapstructure:"db"`
	Mail           MailConfig `mapstructure:"mail"`
	InviteRequired bool       `mapstructure:"inviteRequired"`
	DevEchoMail    bool       `mapstructure:"devEchoMail"`
	AdminKey       string     `mapstructure:"adminKey"`
	JWTSecretFile  string     `mapstructure:"jwtSecretFile"`
	AccessTTLMin   int        `mapstructure:"accessTtlMinutes"`
	RefreshTTLDays int        `mapstructure:"refreshTtlDays"`
}

// RuntimeFields are the config parts that can be applied to a live server
// without a restart (hot reload).
func (c *Config) runtimeOptions() (server.Options, server.Mailer) {
	return server.Options{
		InviteRequired: c.InviteRequired,
		DevEchoMail:    c.DevEchoMail,
		AdminKey:       c.AdminKey,
	}, buildMailer(c.Mail)
}

// loadConfig initializes Viper from the config file plus environment
// overrides, applies defaults, and returns the effective configuration.
// The returned watch function re-applies runtime fields on file changes.
func loadConfig() (*Config, func(*server.Server), error) {
	// Config file path: -config flag > VIDEODELITE_CONFIG > ./config.json.
	// The file is OPTIONAL: Docker deployments may configure purely via env.
	configPath := flag.String("config", "", "configuration file path (optional)")
	flag.Parse()
	if *configPath == "" {
		*configPath = os.Getenv("VIDEODELITE_CONFIG")
	}
	if *configPath == "" {
		*configPath = "config.json"
	}

	v := viper.New()
	v.SetConfigFile(*configPath)
	v.SetConfigType("json")

	// Defaults
	v.SetDefault("listen", ":8800")
	v.SetDefault("db.autoCreateDatabase", true)
	v.SetDefault("jwtSecretFile", "jwt.secret")
	v.SetDefault("accessTtlMinutes", 15)
	v.SetDefault("refreshTtlDays", 30)
	v.SetDefault("devEchoMail", false)

	// Environment overrides. Legacy names keep working:
	v.SetEnvPrefix("VIDEODELITE")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = v.BindEnv("db.connection", "VIDEODELITE_DB_CONN")
	_ = v.BindEnv("adminKey", "VIDEODELITE_ADMIN_KEY")
	_ = v.BindEnv("listen", "VIDEODELITE_LISTEN")
	_ = v.BindEnv("mail.provider", "VIDEODELITE_MAIL_PROVIDER")
	_ = v.BindEnv("mail.from", "VIDEODELITE_MAIL_FROM")
	_ = v.BindEnv("mail.apiKey", "VIDEODELITE_MAIL_APIKEY")
	_ = v.BindEnv("inviteRequired", "VIDEODELITE_INVITE_REQUIRED")
	_ = v.BindEnv("jwtSecretFile", "VIDEODELITE_JWT_SECRET_FILE")

	if err := v.ReadInConfig(); err != nil {
		// Missing file is fine (env/defaults still apply); syntax errors are
		// fatal so a broken edit never rolls out silently.
		var pe *os.PathError
		if errors.As(err, &pe) && os.IsNotExist(pe.Err) {
			fmt.Println("[config] no config file, using env/defaults:", *configPath)
		} else {
			return nil, nil, fmt.Errorf("read config %s: %w", *configPath, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}

	watch := func(srv *server.Server) {
		// Hot reload via polling (every 2s, direct file read). fsnotify's
		// watch on a single-file docker bind-mount misses sed -i style
		// inode replacements; polling reads the path fresh every tick and
		// works with any edit method. Only runtime fields are compared.
		go func() {
			path := *configPath
			signature := ""
			first := true
			for {
				time.Sleep(2 * time.Second)
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				var next Config
				if err := json.Unmarshal(data, &next); err != nil {
					continue // broken edit: keep last good config
				}
				sig := fmt.Sprintf("%s|%s|%s|%v|%v",
					next.AdminKey, next.Mail.Provider, next.Mail.APIKey,
					next.InviteRequired, next.DevEchoMail)
				if first {
					signature, first = sig, false
					continue
				}
				if sig != signature {
					signature = sig
					opts, mailer := next.runtimeOptions()
					srv.UpdateRuntime(opts, mailer)
					fmt.Println("[config] hot reload applied (admin key / mail / invite flag)")
				}
			}
		}()
	}
	return &cfg, watch, nil
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

// ensureDatabase creates the target database when missing (test convenience;
// production DBAs usually pre-create it).
func ensureDatabase(conn string) error {
	if !strings.Contains(conn, "database=") {
		return nil
	}
	dbName := conn
	if i := strings.Index(dbName, "database="); i >= 0 {
		dbName = dbName[i+len("database="):]
		if j := strings.IndexAny(dbName, "&; "); j >= 0 {
			dbName = dbName[:j]
		}
	}
	master := strings.Replace(conn, "database="+dbName, "database=master", 1)
	st, err := server.OpenSQLServerRaw(master)
	if err != nil {
		return fmt.Errorf("connect master: %w", err)
	}
	defer st.Close()
	if err := st.CreateDatabaseIfMissing(dbName); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	return nil
}
