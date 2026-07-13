package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewFeatureValidation(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	tests := []struct {
		name    string
		key     string
		typ     FeatureType
		def     any
		behav   string
		reset   string
		meta    map[string]any
		wantErr bool
	}{
		{name: "valid boolean", key: "sso", typ: FeatureBoolean, def: false},
		{name: "valid limit with hard behavior", key: "seats", typ: FeatureLimit, def: int64(10), behav: LimitHard, reset: ResetBillingCycle},
		{name: "empty key rejected", key: "", typ: FeatureBoolean, def: false, wantErr: true},
		{name: "unknown type rejected", key: "x", typ: FeatureType("weird"), wantErr: true},
		{name: "limit_behavior on non-limit rejected", key: "sso", typ: FeatureBoolean, def: false, behav: LimitHard, wantErr: true},
		{name: "bad limit_behavior rejected", key: "seats", typ: FeatureLimit, def: int64(1), behav: "medium", wantErr: true},
		{name: "bad reset_period rejected", key: "seats", typ: FeatureLimit, def: int64(1), reset: "hourly", wantErr: true},
		{name: "non-bool default on boolean rejected", key: "sso", typ: FeatureBoolean, def: "nope", wantErr: true},
		{name: "non-int default on limit rejected", key: "seats", typ: FeatureLimit, def: "lots", wantErr: true},
		{name: "enum default outside allowed rejected", key: "tier", typ: FeatureEnum, def: "diamond", meta: map[string]any{"allowed_values": []any{"silver", "gold"}}, wantErr: true},
		{name: "enum default inside allowed ok", key: "tier", typ: FeatureEnum, def: "gold", meta: map[string]any{"allowed_values": []any{"silver", "gold"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewFeature(uuid.New(), tc.key, tc.typ, tc.def, "", tc.behav, tc.reset, tc.meta, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NewFeature err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestArchiveMarksInactive(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	f, err := NewFeature(uuid.New(), "sso", FeatureBoolean, false, "", "", "", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Active {
		t.Fatal("new feature should be active")
	}
	f.Archive(now.Add(time.Hour))
	if f.Active {
		t.Fatal("archived feature should be inactive")
	}
	if !f.UpdatedAt.After(now) {
		t.Fatal("archive should bump updated_at")
	}
}
