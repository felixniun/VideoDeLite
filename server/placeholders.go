package server

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

// Placeholder conversion: the shared SQL uses `?` (works with modernc/sqlite),
// but go-mssqldb requires named parameters @p1..@pN. These wrappers rewrite
// the query at execution time so the store implementations stay dialect-free.

func mssqlPlaceholders(q string) string {
	if !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteString("@p" + strconv.Itoa(n))
		} else {
			b.WriteByte(q[i])
		}
	}
	return b.String()
}

func (s *SQLStore) execCtx(ctx context.Context, q string, args ...any) (sql.Result, error) {
	if s.dialect == "mssql" {
		q = mssqlPlaceholders(q)
	}
	return s.db.ExecContext(ctx, q, args...)
}

func (s *SQLStore) qRowCtx(ctx context.Context, q string, args ...any) *sql.Row {
	if s.dialect == "mssql" {
		q = mssqlPlaceholders(q)
	}
	return s.db.QueryRowContext(ctx, q, args...)
}

func (s *SQLStore) qCtx(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	if s.dialect == "mssql" {
		q = mssqlPlaceholders(q)
	}
	return s.db.QueryContext(ctx, q, args...)
}
