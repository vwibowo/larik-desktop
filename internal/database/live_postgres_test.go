package database

import (
	"context"
	"os"
	"testing"
	"time"
)

// Run with LARIK_TEST_POSTGRES_URL pointing at a disposable read-only database.
func TestLivePostgresReadOnly(t *testing.T) {
	const env = "LARIK_TEST_POSTGRES_URL"
	if os.Getenv(env) == "" {
		t.Skip("set LARIK_TEST_POSTGRES_URL to run the live PostgreSQL check")
	}
	m, err := New(map[string]ConnectionConfig{"live": {Driver: "postgres", URLEnv: env}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tables, err := m.Schema(ctx, "live", false)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, table := range tables {
		if table.Schema == "public" && table.Name == "sample_items" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("sample_items absent from schema: %+v", tables)
	}
	rows, err := m.Rows(ctx, "live", "public", "sample_items", 0, 10, false)
	if err != nil || len(rows.Rows) != 2 {
		t.Fatalf("sample rows = %+v, %v", rows, err)
	}
	if _, err := m.Query(ctx, "live", "UPDATE public.sample_items SET label = 'changed'", 10, false); err == nil {
		t.Fatal("write was not rejected")
	}
	check, err := m.Query(ctx, "live", "SELECT label FROM public.sample_items WHERE id = 1", 10, false)
	if err != nil || len(check.Rows) != 1 || check.Rows[0][0] != "one" {
		t.Fatalf("read-only check = %+v, %v", check, err)
	}
}
