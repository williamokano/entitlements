// Package audit provides an append-only audit-log writer shared by every
// module (who / what / when / why), persisting within the ambient
// UnitOfWork transaction.
//
// Implemented in T-007.
package audit
