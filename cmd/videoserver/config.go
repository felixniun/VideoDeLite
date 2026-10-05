package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"videodelite/server"
)

// Config is the server configuration file (config.json).
//
// Deployment notes: keep production secrets out of source control. For a
// future public/internet deployment, only the fields below change — the
// binary is identical:
//
//	listen           公网监听地址，如 ":443"（配合反向代理终止 TLS）
//	db.connection    外网 SQL Server 连接串（建议启用 encrypt=true）
//	admin.key        管理后台密钥（必须设置强随机值）
//	auth.secret_file JWT 签名密钥文件路径（多实例部署需共享存储）
// MailConfig selects the verification-email provider.
//   - provider ""      → dev-echo（验证码由注册响应返回，仅测试）
//   - provider "resend" → Resend HTTP API（推荐；需在 Cloudflare 配置 DNS 记录）
//   - provider "smtp"   → 直连 SMTP（587 STARTTLS / 465 TLS）
type MailConfig struct {
	Provider string     `json:"provider"`
	From     string     `json:"from"`  // "VideoDelite <noreply@mail.yourdomain.com>"
	APIKey   string     `json:"apiKey"` // Resend API key
	SMTP     SMTPConfig `json:"smtp"`
}

type SMTPConfig = server.SMTPConfig

type Config struct {
	Listen string `json:"listen"`
	DB     struct {
		// SQL Server (production, plan §67):
		//   sqlserver://sa:password@192.168.100.101:1433?database=videodelite&encrypt=disable
		// encrypt: disable = 内网测试；外网必须 true 并配置证书
		Connection   string `json:"connection"`
		AutoCreateDB bool   `json:"autoCreateDatabase"`
	} `json:"db"`
	Mail           MailConfig `json:"mail"`
	InviteRequired bool       `json:"inviteRequired"`
	DevEchoMail    bool       `json:"devEchoMail"` // DEV ONLY: verification code returned in response
	AdminKey       string     `json:"adminKey"`
	JWTSecretFile  string     `json:"jwtSecretFile"`
	AccessTTLMin   int        `json:"accessTtlMinutes"`
	RefreshTTLDays int        `json:"refreshTtlDays"`
}

// defaultConfig reflects the current TEST deployment (internal network).
func defaultConfig() *Config {
	c := &Config{
		Listen:         ":8800",
		InviteRequired: false,
		DevEchoMail:    false,
		JWTSecretFile:  "jwt.secret",
		AccessTTLMin:   15,
		RefreshTTLDays: 30,
	}
	c.DB.AutoCreateDB = true
	return c
}

func loadConfig() (*Config, error) {
	cfg := defaultConfig()

	configPath := flag.String("config", "config.json", "configuration file path")
	flag.Parse()

	if raw, err := os.ReadFile(*configPath); err == nil {
		if err := json.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", *configPath, err)
		}
	} else if *configPath != "" && !os.IsNotExist(err) {
		return nil, err
	}

	// Environment overrides (12-factor friendly; used by the Docker image).
	if v := os.Getenv("VIDEODELITE_DB_CONN"); v != "" {
		cfg.DB.Connection = v
	}
	if v := os.Getenv("VIDEODELITE_ADMIN_KEY"); v != "" {
		cfg.AdminKey = v
	}
	if v := os.Getenv("VIDEODELITE_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("VIDEODELITE_MAIL_PROVIDER"); v != "" {
		cfg.Mail.Provider = v
	}
	if v := os.Getenv("VIDEODELITE_MAIL_FROM"); v != "" {
		cfg.Mail.From = v
	}
	if v := os.Getenv("VIDEODELITE_MAIL_APIKEY"); v != "" {
		cfg.Mail.APIKey = v
	}
	if v := os.Getenv("VIDEODELITE_INVITE_REQUIRED"); v == "1" || strings.EqualFold(v, "true") {
		cfg.InviteRequired = true
	}
	return cfg, nil
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
	time.Sleep(500 * time.Millisecond) // let the new DB become visible
	return nil
}
