// Package jobs provides a minimal recurring-job scheduler whose runner uses
// Postgres advisory locks for leader election, so exactly one instance
// executes a job across replicas.
//
// Implemented in T-006. Used for subscription renewals, trial expiry, and
// billing dunning.
package jobs
