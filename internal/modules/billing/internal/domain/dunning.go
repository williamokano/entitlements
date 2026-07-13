package domain

import (
	"time"

	"github.com/google/uuid"
)

// DunningStatus is the state of a dunning schedule for a failed renewal charge.
type DunningStatus string

// Dunning statuses. active means retries are still scheduled; recovered and
// exhausted are terminal.
const (
	DunningActive    DunningStatus = "active"
	DunningRecovered DunningStatus = "recovered"
	DunningExhausted DunningStatus = "exhausted"
)

// Dunning tracks the retry schedule for an invoice whose renewal charge was
// declined. Attempt is the highest charge-attempt number already made against
// the invoice (1 = the initial renewal charge); the dunning retries use 2, 3, …
// so each retry gets a distinct, stable provider idempotency key
// (charge:<invoiceID>:<attempt>). FailedAt anchors the schedule: the retry after
// attempt N is due at FailedAt + offsets[N-1], and once that index runs past the
// configured offsets the schedule is exhausted.
type Dunning struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	InvoiceID      uuid.UUID
	SubscriptionID uuid.UUID
	Status         DunningStatus
	Attempt        int
	FailedAt       time.Time
	NextRetryAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewDunning opens a dunning schedule right after the initial renewal charge
// (attempt 1) was declined at now. It schedules the first retry at the first
// configured offset (or exhausts immediately if no offsets are configured).
func NewDunning(id, tenantID, invoiceID, subscriptionID uuid.UUID, offsets []time.Duration, now time.Time) *Dunning {
	d := &Dunning{
		ID:             id,
		TenantID:       tenantID,
		InvoiceID:      invoiceID,
		SubscriptionID: subscriptionID,
		Status:         DunningActive,
		Attempt:        1,
		FailedAt:       now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	d.schedule(offsets, now)
	return d
}

// NextAttempt is the charge-attempt number the next retry will use.
func (d *Dunning) NextAttempt() int { return d.Attempt + 1 }

// RecordFailure advances past a retry that was declined: it bumps the attempt
// count and reschedules the next retry, or exhausts the schedule when the
// configured offsets are used up.
func (d *Dunning) RecordFailure(offsets []time.Duration, now time.Time) {
	d.Attempt++
	d.schedule(offsets, now)
}

// RecordRecovery marks the schedule recovered after a retry succeeded.
func (d *Dunning) RecordRecovery(now time.Time) {
	d.Attempt++
	d.Status = DunningRecovered
	d.NextRetryAt = nil
	d.UpdatedAt = now
}

// schedule sets NextRetryAt from the offset for the current attempt, or exhausts
// the schedule when no further offset exists. The retry that follows attempt N
// is due at FailedAt + offsets[N-1].
func (d *Dunning) schedule(offsets []time.Duration, now time.Time) {
	idx := d.Attempt - 1
	if idx >= 0 && idx < len(offsets) {
		next := d.FailedAt.Add(offsets[idx])
		d.NextRetryAt = &next
	} else {
		d.NextRetryAt = nil
		d.Status = DunningExhausted
	}
	d.UpdatedAt = now
}

// Due reports whether an active schedule's next retry is due at now.
func (d *Dunning) Due(now time.Time) bool {
	return d.Status == DunningActive && d.NextRetryAt != nil && !now.Before(*d.NextRetryAt)
}

// Exhausted reports whether all configured retries have been used up.
func (d *Dunning) Exhausted() bool { return d.Status == DunningExhausted }
