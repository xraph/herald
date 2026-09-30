package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xraph/grove"
	"github.com/xraph/grove/drivers/sqlitedriver"
	_ "github.com/xraph/grove/drivers/sqlitedriver/sqlitemigrate"

	"github.com/xraph/herald/store"
	sqlitestore "github.com/xraph/herald/store/sqlite"
	"github.com/xraph/herald/store/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, openSQLite)
}

func openSQLite(t *testing.T) store.Store {
	t.Helper()
	ctx := context.Background()
	sdb := sqlitedriver.New()
	if err := sdb.Open(ctx, filepath.Join(t.TempDir(), "herald.db")); err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	db, err := grove.Open(sdb)
	if err != nil {
		t.Fatalf("grove open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := sqlitestore.New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}
