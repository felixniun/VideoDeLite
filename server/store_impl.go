package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const timeLayout = "2006-01-02 15:04:05"

func ts(t time.Time) string    { return t.UTC().Format(timeLayout) }
func parseTS(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
}

func optTS(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

// toTime normalizes timestamp values across dialects: MSSQL DATETIME2
// columns scan as time.Time, SQLite TEXT columns as string.
func toTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case string:
		return parseTS(t)
	case []byte:
		return parseTS(string(t))
	}
	return time.Time{}
}

// limitClause returns the dialect-appropriate row-limit tail. The `?`
// placeholder is rewritten to @pN for MSSQL by the shared wrapper.
func (s *SQLStore) limitClause() string {
	if s.dialect == "mssql" {
		return "OFFSET 0 ROWS FETCH NEXT ? ROWS ONLY"
	}
	return "LIMIT ?"
}

// ---- accounts ----

func (s *SQLStore) CreateAccount(ctx context.Context, username, email, passwordHash string) (int64, error) {
	q := s.insertReturning(`INSERT INTO accounts (username, email, password_hash, status, email_verified, created_at)
		VALUES (?, ?, ?, 'active', 0, ?)`)
	var id int64
	err := s.qRowCtx(ctx, q, username, email, passwordHash, ts(time.Now())).Scan(&id)
	if err != nil && isUniqueViolation(err) {
		return 0, fmt.Errorf("username or email already registered")
	}
	return id, err
}

func (s *SQLStore) GetAccountByEmail(ctx context.Context, email string) (*Account, error) {
	return s.scanAccount(s.qRowCtx(ctx,
		`SELECT id, username, email, password_hash, status, email_verified, created_at
		 FROM accounts WHERE email = ?`, email))
}

func (s *SQLStore) GetAccountByID(ctx context.Context, id int64) (*Account, error) {
	return s.scanAccount(s.qRowCtx(ctx,
		`SELECT id, username, email, password_hash, status, email_verified, created_at
		 FROM accounts WHERE id = ?`, id))
}

func (s *SQLStore) scanAccount(row *sql.Row) (*Account, error) {
	var a Account
	// BIT columns arrive as bool from go-mssqldb and int64 from SQLite.
	var verified any
	var created any
	if err := row.Scan(&a.ID, &a.Username, &a.Email, &a.PasswordHash, &a.Status, &verified, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	switch v := verified.(type) {
	case bool:
		a.EmailVerified = v
	case int64:
		a.EmailVerified = v != 0
	case int:
		a.EmailVerified = v != 0
	}
	a.CreatedAt = toTime(created)
	return &a, nil
}

func (s *SQLStore) SetAccountStatus(ctx context.Context, id int64, status string) error {
	_, err := s.execCtx(ctx, `UPDATE accounts SET status = ? WHERE id = ?`, status, id)
	return err
}

// DeleteAccountPersonalData removes personal data while keeping the minimal
// security record (authorization spec §23: anonymized retained records).
func (s *SQLStore) DeleteAccountPersonalData(ctx context.Context, id int64) error {
	anon := fmt.Sprintf("deleted-account-%d", id)
	if _, err := s.execCtx(ctx,
		`UPDATE accounts SET email = ?, username = ?, status = 'deleted' WHERE id = ?`,
		anon+"@deleted.local", anon, id); err != nil {
		return err
	}
	if _, err := s.execCtx(ctx, `UPDATE devices SET status = 'revoked' WHERE account_id = ?`, id); err != nil {
		return err
	}
	return s.RevokeAccountLicenses(ctx, id)
}

// ---- email verification ----

func (s *SQLStore) PutEmailToken(ctx context.Context, accountID int64, code string, expires time.Time) error {
	_, err := s.execCtx(ctx,
		`INSERT INTO email_tokens (account_id, code, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		accountID, code, ts(expires), ts(time.Now()))
	return err
}

func (s *SQLStore) ConsumeEmailToken(ctx context.Context, accountID int64, code string) error {
	var id int64
	err := s.qRowCtx(ctx,
		`SELECT id FROM email_tokens
		 WHERE account_id = ? AND code = ? AND expires_at > ?`,
		accountID, code, ts(time.Now())).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("invalid or expired verification code")
	}
	if err != nil {
		return err
	}
	if _, err := s.execCtx(ctx,
		`UPDATE accounts SET email_verified = 1 WHERE id = ?`, accountID); err != nil {
		return err
	}
	_, err = s.execCtx(ctx, `DELETE FROM email_tokens WHERE account_id = ?`, accountID)
	return err
}

func (s *SQLStore) ConsumeInviteCode(ctx context.Context, code string) error {
	res, err := s.execCtx(ctx,
		`UPDATE invite_codes SET used_at = ? WHERE code = ? AND used_at IS NULL`,
		ts(time.Now()), code)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("invite code invalid or already used")
	}
	return nil
}

func (s *SQLStore) InviteRequired(ctx context.Context) (bool, error) {
	var n int
	// Required only if at least one invite code has ever been issued.
	err := s.qRowCtx(ctx, `SELECT COUNT(*) FROM invite_codes`).Scan(&n)
	return n > 0, err
}

// ---- refresh tokens ----

func (s *SQLStore) CreateRefreshToken(ctx context.Context, accountID int64, tokenHash string, expires time.Time) error {
	_, err := s.execCtx(ctx,
		`INSERT INTO refresh_tokens (account_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		accountID, tokenHash, ts(expires), ts(time.Now()))
	return err
}

func (s *SQLStore) GetRefreshToken(ctx context.Context, tokenHash string) (int64, time.Time, bool, error) {
	var accountID int64
	var expires any
	var revoked bool
	err := s.qRowCtx(ctx,
		`SELECT account_id, expires_at, revoked FROM refresh_tokens WHERE token_hash = ?`,
		tokenHash).Scan(&accountID, &expires, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, time.Time{}, false, ErrNotFound
	}
	if err != nil {
		return 0, time.Time{}, false, err
	}
	return accountID, toTime(expires), revoked, nil
}

func (s *SQLStore) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.execCtx(ctx, `UPDATE refresh_tokens SET revoked = 1 WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *SQLStore) RevokeAllRefreshTokens(ctx context.Context, accountID int64) error {
	_, err := s.execCtx(ctx, `UPDATE refresh_tokens SET revoked = 1 WHERE account_id = ?`, accountID)
	return err
}

// ---- devices ----

func (s *SQLStore) UpsertDevice(ctx context.Context, accountID int64, installationID, appVersion string) (*Device, error) {
	_, err := s.execCtx(ctx, `
		MERGE INTO devices AS t
		USING (SELECT ? AS account_id, ? AS installation_id) AS src
		ON t.account_id = src.account_id AND t.installation_id = src.installation_id
		WHEN MATCHED THEN UPDATE SET last_seen = ?, app_version = ?
		WHEN NOT MATCHED THEN
			INSERT (account_id, installation_id, app_version, status, created_at, last_seen)
			VALUES (?, ?, ?, 'active', ?, ?);`,
		accountID, installationID, ts(time.Now()), appVersion,
		accountID, installationID, appVersion, ts(time.Now()), ts(time.Now()))
	if err != nil {
		// MERGE is MSSQL-only; SQLite fallback.
		if s.dialect == "sqlite" {
			_, err = s.execCtx(ctx, `
				INSERT INTO devices (account_id, installation_id, app_version, status, created_at, last_seen)
				VALUES (?, ?, ?, 'active', ?, ?)
				ON CONFLICT(account_id, installation_id) DO UPDATE SET
					last_seen = excluded.last_seen, app_version = excluded.app_version`,
				accountID, installationID, appVersion, ts(time.Now()), ts(time.Now()))
		} else {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	return s.GetDeviceByInstallation(ctx, accountID, installationID)
}

func (s *SQLStore) GetDeviceByInstallation(ctx context.Context, accountID int64, installationID string) (*Device, error) {
	var d Device
	var created, seen any
	err := s.qRowCtx(ctx,
		`SELECT id, account_id, installation_id, app_version, status, created_at, last_seen
		 FROM devices WHERE account_id = ? AND installation_id = ?`,
		accountID, installationID).Scan(&d.ID, &d.AccountID, &d.InstallationID, &d.AppVersion, &d.Status, &created, &seen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.CreatedAt, d.LastSeen = toTime(created), toTime(seen)
	return &d, nil
}

func (s *SQLStore) SetDeviceStatus(ctx context.Context, deviceID int64, status string) error {
	_, err := s.execCtx(ctx, `UPDATE devices SET status = ? WHERE id = ?`, status, deviceID)
	return err
}

func (s *SQLStore) TouchDevice(ctx context.Context, deviceID int64, appVersion string) error {
	_, err := s.execCtx(ctx,
		`UPDATE devices SET last_seen = ?, app_version = ? WHERE id = ?`,
		ts(time.Now()), appVersion, deviceID)
	return err
}

func (s *SQLStore) ListDevices(ctx context.Context, accountID int64) ([]Device, error) {
	rows, err := s.qCtx(ctx,
		`SELECT id, account_id, installation_id, app_version, status, created_at, last_seen
		 FROM devices WHERE account_id = ? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		var created, seen any
		if err := rows.Scan(&d.ID, &d.AccountID, &d.InstallationID, &d.AppVersion, &d.Status, &created, &seen); err != nil {
			return nil, err
		}
		d.CreatedAt, d.LastSeen = toTime(created), toTime(seen)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---- licenses ----

func (s *SQLStore) CreateLicense(ctx context.Context, accountID, deviceID int64, expires *time.Time) (int64, error) {
	q := s.insertReturning(`INSERT INTO licenses (account_id, device_id, type, status, expires_at, created_at)
		VALUES (?, ?, 'professional', 'active', ?, ?)`)
	var id int64
	err := s.qRowCtx(ctx, q, accountID, deviceID, optTS(expires), ts(time.Now())).Scan(&id)
	return id, err
}

func (s *SQLStore) GetActiveLicense(ctx context.Context, accountID, deviceID int64) (*License, error) {
	var l License
	var expires any
	var created any
	err := s.qRowCtx(ctx,
		`SELECT id, account_id, device_id, type, status, expires_at, created_at
		 FROM licenses WHERE account_id = ? AND device_id = ? AND status = 'active'
		 ORDER BY id DESC`, accountID, deviceID).
		Scan(&l.ID, &l.AccountID, &l.DeviceID, &l.Type, &l.Status, &expires, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if expires != nil {
		if t := toTime(expires); !t.IsZero() {
			l.ExpiresAt = &t
		}
	}
	l.CreatedAt = toTime(created)
	return &l, nil
}

func (s *SQLStore) GetLicenseStatus(ctx context.Context, accountID, deviceID int64) (string, *time.Time, error) {
	l, err := s.GetActiveLicense(ctx, accountID, deviceID)
	if errors.Is(err, ErrNotFound) {
		return "none", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	if l.ExpiresAt != nil && l.ExpiresAt.Before(time.Now()) {
		return "expired", l.ExpiresAt, nil
	}
	return l.Status, l.ExpiresAt, nil
}

func (s *SQLStore) RevokeLicense(ctx context.Context, licenseID int64) error {
	_, err := s.execCtx(ctx,
		`UPDATE licenses SET status = 'revoked', revoked_at = ? WHERE id = ?`,
		ts(time.Now()), licenseID)
	return err
}

func (s *SQLStore) RevokeAccountLicenses(ctx context.Context, accountID int64) error {
	_, err := s.execCtx(ctx,
		`UPDATE licenses SET status = 'revoked', revoked_at = ? WHERE account_id = ? AND status = 'active'`,
		ts(time.Now()), accountID)
	return err
}

// ---- security / admin ----

func (s *SQLStore) LogSecurityEvent(ctx context.Context, accountID int64, kind, detail string) error {
	_, err := s.execCtx(ctx,
		`INSERT INTO security_events (account_id, kind, detail, created_at) VALUES (?, ?, ?, ?)`,
		accountID, kind, detail, ts(time.Now()))
	return err
}

func (s *SQLStore) ListSecurityEvents(ctx context.Context, limit int) ([]SecurityEvent, error) {
	rows, err := s.qCtx(ctx,
		`SELECT id, account_id, kind, detail, created_at FROM security_events ORDER BY id DESC `+s.limitClause(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SecurityEvent
	for rows.Next() {
		var e SecurityEvent
		var created any
		if err := rows.Scan(&e.ID, &e.AccountID, &e.Kind, &e.Detail, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = toTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// AdminStats aggregates dashboard numbers. The 14-day registration series
// is built in Go from a grouped query so gaps become zero buckets.
// Day buckets: MSSQL CONVERT(…,23) and SQLite substr(…,1,10) both yield
// "YYYY-MM-DD" from the stored timestamps.
func (s *SQLStore) AdminStats(ctx context.Context) (AdminStats, error) {
	var st AdminStats
	one := func(q string) (int64, error) {
		var n int64
		err := s.qRowCtx(ctx, q).Scan(&n)
		return n, err
	}
	var err error
	if st.TotalAccounts, err = one(`SELECT COUNT(*) FROM accounts`); err != nil {
		return st, err
	}
	if st.Verified, err = one(`SELECT COUNT(*) FROM accounts WHERE email_verified = 1`); err != nil {
		return st, err
	}
	if st.Banned, err = one(`SELECT COUNT(*) FROM accounts WHERE status = 'banned'`); err != nil {
		return st, err
	}
	if st.TotalDevices, err = one(`SELECT COUNT(*) FROM devices`); err != nil {
		return st, err
	}
	if st.ActiveLicense, err = one(`SELECT COUNT(*) FROM licenses WHERE status = 'active'`); err != nil {
		return st, err
	}
	if err := s.qRowCtx(ctx,
		`SELECT COUNT(*) FROM security_events WHERE created_at >= ?`,
		ts(time.Now().UTC().Add(-24*time.Hour))).Scan(&st.EventsToday); err != nil {
		return st, err
	}

	// 14-day series
	dayExpr := `substr(created_at, 1, 10)`
	since := ts(time.Now().UTC().Add(-13 * 24 * time.Hour))
	if s.dialect == "mssql" {
		dayExpr = `CONVERT(varchar(10), created_at, 23)`
	}
	rows, err := s.qCtx(ctx,
		`SELECT `+dayExpr+` AS d, COUNT(*) AS c FROM accounts
		 WHERE created_at >= ? GROUP BY `+dayExpr+` ORDER BY d`, since)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	byDay := map[string]int64{}
	for rows.Next() {
		var d string
		var c int64
		if err := rows.Scan(&d, &c); err != nil {
			return st, err
		}
		byDay[d] = c
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	for i := 13; i >= 0; i-- {
		day := time.Now().UTC().Add(-time.Duration(i) * 24 * time.Hour)
		key := day.Format("2006-01-02")
		st.Registrations = append(st.Registrations, DayCount{
			Day:   day.Format("01-02"),
			Count: byDay[key],
		})
	}
	return st, nil
}

func (s *SQLStore) ListAccounts(ctx context.Context, limit int) ([]AccountView, error) {
	rows, err := s.qCtx(ctx,
		`SELECT id, username, email, status, email_verified, created_at
		 FROM accounts ORDER BY id DESC `+s.limitClause(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountView
	for rows.Next() {
		var a AccountView
		var verified any
		var created any
		if err := rows.Scan(&a.ID, &a.Username, &a.Email, &a.Status, &verified, &created); err != nil {
			return nil, err
		}
		switch v := verified.(type) {
		case bool:
			a.EmailVerified = v
		case int64:
			a.EmailVerified = v != 0
		case int:
			a.EmailVerified = v != 0
		}
		a.CreatedAt = toTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// MSSQL: violation of UNIQUE constraint (2627/2601); SQLite: SQLITE_CONSTRAINT
	return contains(msg, "UNIQUE") || contains(msg, "unique") || contains(msg, "2627") || contains(msg, "2601")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
