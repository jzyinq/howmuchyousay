package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBExec is the subset of pgx methods used by store methods that may run
// inside a transaction. Both *pgxpool.Pool and pgx.Tx satisfy it.
//
// Store methods that do not participate in the answer-handler transaction
// keep their existing pool-based signatures.
type DBExec interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
