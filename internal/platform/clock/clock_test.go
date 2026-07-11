package clock

import (
	"testing"
	"time"
)

func TestSystemNowIsUTC(t *testing.T) {
	got := System.Now()
	if got.Location() != time.UTC {
		t.Fatalf("System.Now() location = %v, want UTC", got.Location())
	}
}

func TestFrozenSetAndNormalizesToUTC(t *testing.T) {
	loc := time.FixedZone("EST", -5*3600)
	base := time.Date(2026, 7, 11, 8, 0, 0, 0, loc)

	c := NewFrozen(base)
	if c.Now().Location() != time.UTC {
		t.Fatalf("Frozen.Now() location = %v, want UTC", c.Now().Location())
	}
	if !c.Now().Equal(base) {
		t.Fatalf("Frozen.Now() = %v, want equal to %v", c.Now(), base)
	}
}

func TestFrozenAdvance(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewFrozen(start)

	got := c.Advance(90 * time.Minute)
	want := start.Add(90 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("Advance returned %v, want %v", got, want)
	}
	if !c.Now().Equal(want) {
		t.Fatalf("after Advance, Now() = %v, want %v", c.Now(), want)
	}
}

func TestFrozenSet(t *testing.T) {
	c := NewFrozen(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	target := time.Date(2030, 12, 25, 12, 0, 0, 0, time.UTC)
	c.Set(target)
	if !c.Now().Equal(target) {
		t.Fatalf("after Set, Now() = %v, want %v", c.Now(), target)
	}
}
