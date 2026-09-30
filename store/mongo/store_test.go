package mongo

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/xraph/grove"
	"github.com/xraph/grove/drivers/mongodriver"
	_ "github.com/xraph/grove/drivers/mongodriver/mongomigrate"

	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/storetest"
)

// TestConformance runs the shared suite against a real MongoDB, each subtest
// in a fresh database that is dropped afterwards, e.g.
// HERALD_TEST_MONGO_URI=mongodb://localhost:57017
func TestConformance(t *testing.T) {
	uri := os.Getenv("HERALD_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set HERALD_TEST_MONGO_URI to run the mongo conformance suite")
	}
	storetest.Run(t, func(t *testing.T) store.Store {
		t.Helper()
		ctx := context.Background()
		name := "herald_conformance_" + strconv.FormatInt(time.Now().UnixNano(), 36)
		mdb := mongodriver.New()
		if err := mdb.Open(ctx, uri, mongodriver.WithDatabase(name)); err != nil {
			t.Fatalf("mongo open: %v", err)
		}
		db, err := grove.Open(mdb)
		if err != nil {
			t.Fatalf("grove open: %v", err)
		}
		t.Cleanup(func() {
			_ = mdb.Database().Drop(context.Background())
			_ = db.Close()
		})
		s := New(db)
		if err := s.Migrate(ctx); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		return s
	})
}
