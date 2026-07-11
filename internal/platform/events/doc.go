// Package events is the asynchronous backbone: an in-process event bus backed
// by a transactional outbox.
//
// Append writes an event to the outbox inside the ambient UnitOfWork
// transaction so events are never lost or phantom. A relay worker polls with
// FOR UPDATE SKIP LOCKED and dispatches to subscribers at least once; a
// processed-events table makes every consumer idempotent (exactly-once
// effect). Implemented in T-005.
package events
