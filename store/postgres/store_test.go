package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/xraph/grove"
	"github.com/xraph/grove/drivers/pgdriver"
	_ "github.com/xraph/grove/drivers/pgdriver/pgmigrate"

	"github.com/xraph/herald/device"
	"github.com/xraph/herald/device/devicetest"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/storetest"
)

// openTestStore opens a migrated, truncated Store against the DSN in
// HERALD_TEST_POSTGRES_DSN, e.g.
// postgres://user:pass@localhost:55432/herald_conformance?sslmode=disable
// The database must be one the tests may truncate. It skips without a DSN.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("HERALD_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HERALD_TEST_POSTGRES_DSN to run the postgres conformance suites")
	}
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
herald_messages, herald_inbox, herald_preferences, herald_scoped_configs, herald_devices`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

// TestConformance runs the shared store suite against a real Postgres.
func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) store.Store {
		t.Helper()
		return openTestStore(t)
	})
}

// TestDeviceConformance runs the shared device suite against a real Postgres.
func TestDeviceConformance(t *testing.T) {
	devicetest.Run(t, func(t *testing.T) device.Store {
		t.Helper()
		return openTestStore(t)
	})
}
