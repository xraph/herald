package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/xraph/grove"
	"github.com/xraph/grove/drivers/pgdriver"
	_ "github.com/xraph/grove/drivers/pgdriver/pgmigrate"

	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/storetest"
)

// TestConformance runs the shared suite against a real Postgres. It needs a
// DSN for a database it may truncate, e.g.
// HERALD_TEST_POSTGRES_DSN=postgres://user:pass@localhost:55432/herald_conformance?sslmode=disable
func TestConformance(t *testing.T) {
	dsn := os.Getenv("HERALD_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HERALD_TEST_POSTGRES_DSN to run the postgres conformance suite")
	}
	storetest.Run(t, func(t *testing.T) store.Store {
		t.Helper()
		ctx := context.Background()
		pdb := pgdriver.New()
		if err := pdb.Open(ctx, dsn); err != nil {
			t.Fatalf("postgres open: %v", err)
		}
		db, err := grove.Open(pdb)
		if err != nil {
			t.Fatalf("grove open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		s := New(db)
		if err := s.Migrate(ctx); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		if _, err := s.pg.Exec(ctx, `TRUNCATE herald_providers, herald_templates, herald_template_versions,
herald_messages, herald_inbox, herald_preferences, herald_scoped_configs`); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		return s
	})
}
