package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the query surface shared by the pool and a transaction.
// Repositories accept a Querier (via Q) so the same code runs inside or
// outside a UnitOfWork transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// UnitOfWork runs functions inside a database transaction that travels in the
// context, so every repository call within the function joins the same
// transaction. Nested Do calls join the outer transaction instead of opening
// a new one.
type UnitOfWork struct {
	pool *pgxpool.Pool
}

// NewUnitOfWork returns a UnitOfWork backed by the given pool.
func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

// Do executes fn inside a transaction. If the context already carries a
// transaction (a nested Do), fn joins it and commit/rollback is left to the
// outermost Do. The transaction is rolled back if fn returns an error or
// panics (the panic is re-raised); otherwise it is committed.
func (u *UnitOfWork) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if txFrom(ctx) != nil {
		// Join the ambient transaction; the outermost Do owns its fate.
		return fn(ctx)
	}

	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin tx: %w", err)
	}

	done := false
	defer func() {
		if !done {
			// fn panicked: roll back and let the panic continue.
			_ = tx.Rollback(ctx)
		}
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		done = true
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("postgres: rollback: %w", rbErr))
		}
		return err
	}

	done = true
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}

// Q returns the ambient transaction if the context carries one, and the pool
// otherwise. Repositories should route every query through Q so they
// transparently join UnitOfWork transactions.
func Q(ctx context.Context, pool *pgxpool.Pool) Querier {
	if tx := txFrom(ctx); tx != nil {
		return tx
	}
	return pool
}

// InTx reports whether the context carries an ambient transaction.
func InTx(ctx context.Context) bool { return txFrom(ctx) != nil }

func txFrom(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(txKey{}).(pgx.Tx)
	return tx
}
