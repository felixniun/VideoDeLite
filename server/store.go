package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
	_ "modernc.org/sqlite"
)

// ---- domain types ----

type Account struct {
	ID            int64
	Username      string
	Email         string
	PasswordHash  string
	Status        string // active | banned | deleted
	EmailVerified bool
	CreatedAt     time.Time
}

type Device struct {
	ID             int64
	AccountID      int64
	InstallationID string
	AppVersion     string
	Status         string // active | revoked
	CreatedAt      time.Time
	LastSeen       time.Time
}

type License struct {
	ID        int64
	AccountID int64
	DeviceID  int64
	Type      string // professional
	Status    string // active | revoked
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// ---- Store contract (implemented for SQL Server and, dev-only, SQLite) ----

type Store interface {
	Migrate() error
	Close() error

	CreateAccount(ctx context.Context, username, email, passwordHash string) (int64, error)
	GetAccountByEmail(ctx context.Context, email string) (*Account, error)
	GetAccountByID(ctx context.Context, id int64) (*Account, error)
	SetAccountStatus(ctx context.Context, id int64, status string) error
	DeleteAccountPersonalData(ctx context.Context, id int64) error

	PutEmailToken(ctx context.Context, accountID int64, code string, expires time.Time) error
	ConsumeEmailToken(ctx context.Context, accountID int64, code string) error
	ConsumeInviteCode(ctx context.Context, code string) error
	InviteRequired(ctx context.Context) (bool, error)

	CreateRefreshToken(ctx context.Context, accountID int64, tokenHash string, expires time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (accountID int64, expires time.Time, revoked bool, err error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeAllRefreshTokens(ctx context.Context, accountID int64) error

	UpsertDevice(ctx context.Context, accountID int64, installationID, appVersion string) (*Device, error)
	GetDeviceByInstallation(ctx context.Context, accountID int64, installationID string) (*Device, error)
	SetDeviceStatus(ctx context.Context, deviceID int64, status string) error
	TouchDevice(ctx context.Context, deviceID int64, appVersion string) error

	CreateLicense(ctx context.Context, accountID, deviceID int64, expires *time.Time) (int64, error)
	GetActiveLicense(ctx context.Context, accountID, deviceID int64) (*License, error)
	GetLicenseStatus(ctx context.Context, accountID, deviceID int64) (string, *time.Time, error)
	RevokeLicense(ctx context.Context, licenseID int64) error
	RevokeAccountLicenses(ctx context.Context, accountID int64) error

	LogSecurityEvent(ctx context.Context, accountID int64, kind, detail string) error
	ListSecurityEvents(ctx context.Context, limit int) ([]SecurityEvent, error)
	ListAccounts(ctx context.Context, limit int) ([]AccountView, error)
	ListDevices(ctx context.Context, accountID int64) ([]Device, error)

	// AdminStats feeds the admin dashboard charts (Phase 10 GUI).
	AdminStats(ctx context.Context) (AdminStats, error)

	Ping() error
}

// AdminStats is the aggregate view for the admin dashboard.
type AdminStats struct {
	TotalAccounts  int64            `json:"totalAccounts"`
	Verified       int64            `json:"verifiedAccounts"`
	Banned         int64            `json:"bannedAccounts"`
	TotalDevices   int64            `json:"totalDevices"`
	ActiveLicense  int64            `json:"activeLicenses"`
	EventsToday    int64            `json:"eventsToday"`
	Registrations  []DayCount       `json:"registrations"` // last 14 days, gaps filled
}

// DayCount is one bucket of a time-series chart.
type DayCount struct {
	Day   string `json:"day"`   // MM-DD
	Count int64  `json:"count"`
}

type SecurityEvent struct {
	ID        int64
	AccountID int64
	Kind      string
	Detail    string
	CreatedAt time.Time
}

type AccountView struct {
	ID            int64
	Username      string
	Email         string
	Status        string
	EmailVerified bool
	CreatedAt     time.Time
}

var ErrNotFound = errors.New("not found")

// ---- shared SQL store ----
//
// One implementation serves both dialects (placeholders `?` work for both
// go-mssqldb and modernc/sqlite). Only identity generation and schema
// creation differ.

type SQLStore struct {
	db      *sql.DB
	dialect string // "mssql" | "sqlite"
}

func OpenSQLServer(conn string) (Store, error) {
	d, err := sql.Open("sqlserver", conn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(20)
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, fmt.Errorf("sql server ping: %w", err)
	}
	return &SQLStore{db: d, dialect: "mssql"}, nil
}

// OpenSQLServerRaw opens a connection without pinging (used by the
// database auto-create helper, which runs before the DB exists).
func OpenSQLServerRaw(conn string) (*SQLStore, error) {
	d, err := sql.Open("sqlserver", conn)
	if err != nil {
		return nil, err
	}
	return &SQLStore{db: d, dialect: "mssql"}, nil
}

// CreateDatabaseIfMissing creates the target database when absent.
func (s *SQLStore) CreateDatabaseIfMissing(name string) error {
	q := `IF DB_ID(@p1) IS NULL CREATE DATABASE ` + quotedIdent(name)
	_, err := s.db.Exec(q, name)
	return err
}

// quotedIdent brackets an identifier to block injection via the DB name.
func quotedIdent(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

func (s *SQLStore) Ping() error  { return s.db.Ping() }
func (s *SQLStore) Close() error { return s.db.Close() }

// reValuesTail matches the ") VALUES (...)" boundary, tolerating newlines
// between the column list and VALUES.
var reValuesTail = regexp.MustCompile(`\)\s*VALUES`)

// insertReturning adapts "INSERT ... VALUES (...)" to return the new id:
//   - MSSQL:  INSERT INTO t (cols) OUTPUT INSERTED.id VALUES (...)
//   - SQLite: INSERT INTO t (cols) VALUES (...) RETURNING id
func (s *SQLStore) insertReturning(q string) string {
	if s.dialect == "mssql" {
		return reValuesTail.ReplaceAllString(q, ") OUTPUT INSERTED.id VALUES")
	}
	return q + " RETURNING id"
}

func (s *SQLStore) Migrate() error {
	if s.dialect == "mssql" {
		return s.migrateMSSQL()
	}
	return s.migrateSQLite()
}

func (s *SQLStore) migrateMSSQL() error {
	stmts := []string{
		`IF OBJECT_ID('dbo.accounts') IS NULL CREATE TABLE dbo.accounts (
			id BIGINT IDENTITY PRIMARY KEY,
			username NVARCHAR(64) NOT NULL UNIQUE,
			email NVARCHAR(254) NOT NULL UNIQUE,
			password_hash NVARCHAR(256) NOT NULL,
			status NVARCHAR(16) NOT NULL DEFAULT 'active',
			email_verified BIT NOT NULL DEFAULT 0,
			created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
		)`,
		`IF OBJECT_ID('dbo.email_tokens') IS NULL CREATE TABLE dbo.email_tokens (
			id BIGINT IDENTITY PRIMARY KEY,
			account_id BIGINT NOT NULL,
			code NVARCHAR(8) NOT NULL,
			expires_at DATETIME2 NOT NULL,
			created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
		)`,
		`IF OBJECT_ID('dbo.invite_codes') IS NULL CREATE TABLE dbo.invite_codes (
			code NVARCHAR(32) PRIMARY KEY,
			used_at DATETIME2 NULL
		)`,
		`IF OBJECT_ID('dbo.refresh_tokens') IS NULL CREATE TABLE dbo.refresh_tokens (
			id BIGINT IDENTITY PRIMARY KEY,
			account_id BIGINT NOT NULL,
			token_hash CHAR(64) NOT NULL UNIQUE,
			expires_at DATETIME2 NOT NULL,
			revoked BIT NOT NULL DEFAULT 0,
			created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
		)`,
		`IF OBJECT_ID('dbo.devices') IS NULL CREATE TABLE dbo.devices (
			id BIGINT IDENTITY PRIMARY KEY,
			account_id BIGINT NOT NULL,
			installation_id NVARCHAR(64) NOT NULL,
			app_version NVARCHAR(32) NOT NULL DEFAULT '',
			status NVARCHAR(16) NOT NULL DEFAULT 'active',
			created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
			last_seen DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
			CONSTRAINT uq_device UNIQUE (account_id, installation_id)
		)`,
		`IF OBJECT_ID('dbo.licenses') IS NULL CREATE TABLE dbo.licenses (
			id BIGINT IDENTITY PRIMARY KEY,
			account_id BIGINT NOT NULL,
			device_id BIGINT NOT NULL,
			type NVARCHAR(32) NOT NULL DEFAULT 'professional',
			status NVARCHAR(16) NOT NULL DEFAULT 'active',
			expires_at DATETIME2 NULL,
			created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
			revoked_at DATETIME2 NULL
		)`,
		`IF OBJECT_ID('dbo.security_events') IS NULL CREATE TABLE dbo.security_events (
			id BIGINT IDENTITY PRIMARY KEY,
			account_id BIGINT NOT NULL DEFAULT 0,
			kind NVARCHAR(32) NOT NULL,
			detail NVARCHAR(512) NOT NULL DEFAULT '',
			created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
		)`,
		`IF OBJECT_ID('dbo.app_versions') IS NULL CREATE TABLE dbo.app_versions (
			version NVARCHAR(32) PRIMARY KEY,
			notes NVARCHAR(1024) NOT NULL DEFAULT '',
			published_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("mssql migrate: %w", err)
		}
	}
	return nil
}

func (s *SQLStore) migrateSQLite() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			email_verified INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS email_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			code TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS invite_codes (
			code TEXT PRIMARY KEY,
			used_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			revoked INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			installation_id TEXT NOT NULL,
			app_version TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			last_seen TEXT NOT NULL,
			UNIQUE (account_id, installation_id)
		)`,
		`CREATE TABLE IF NOT EXISTS licenses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			device_id INTEGER NOT NULL,
			type TEXT NOT NULL DEFAULT 'professional',
			status TEXT NOT NULL DEFAULT 'active',
			expires_at TEXT,
			created_at TEXT NOT NULL,
			revoked_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS security_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL DEFAULT 0,
			kind TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS app_versions (
			version TEXT PRIMARY KEY,
			notes TEXT NOT NULL DEFAULT '',
			published_at TEXT NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("sqlite migrate: %w", err)
		}
	}
	return nil
}

// OpenDevSQLite is for integration tests and local development ONLY.
// Production deployments use SQL Server (plan §67).
func OpenDevSQLite(path string) (Store, error) {
	d, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, err
	}
	return &SQLStore{db: d, dialect: "sqlite"}, nil
}
