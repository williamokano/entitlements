// Package postgres provides the pgx connection pool, the UnitOfWork
// transaction manager (transactions travel in context so repositories join
// the ambient transaction), and the goose-based migration runner.
//
// Implemented in T-003. Each module owns its own Postgres schema; no
// cross-module joins.
package postgres
