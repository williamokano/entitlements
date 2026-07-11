//go:build integration

package testkit

import (
	"context"
	"testing"
)

func TestPostgresStartsAndMigrates(t *testing.T) {
	pool := Postgres(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx,
		`SELECT schema_name FROM information_schema.schemata WHERE schema_name = ANY($1)`,
		[]string{"platform", "tenant", "authn", "authz", "catalog", "subscription", "entitlements", "billing"})
	if err != nil {
		t.Fatalf("query schemata: %v", err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[s] = true
	}
	for _, want := range []string{"platform", "tenant", "authn", "authz", "catalog", "subscription", "entitlements", "billing"} {
		if !got[want] {
			t.Errorf("schema %q missing after migrations", want)
		}
	}
}

func TestPostgresIsolationBetweenTests(t *testing.T) {
	ctx := context.Background()
	poolA := Postgres(t)
	poolB := Postgres(t)

	if _, err := poolA.Exec(ctx, `CREATE TABLE platform.isolation_probe (id int)`); err != nil {
		t.Fatalf("create table in A: %v", err)
	}

	var exists bool
	err := poolB.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		 WHERE table_schema = 'platform' AND table_name = 'isolation_probe')`).Scan(&exists)
	if err != nil {
		t.Fatalf("check table in B: %v", err)
	}
	if exists {
		t.Fatal("table created in database A is visible in database B — databases are not isolated")
	}
}
