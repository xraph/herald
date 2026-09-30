// Package storetest is the conformance suite every herald store backend runs.
//
// Each backend's own test file calls Run with a function that opens a fresh,
// empty, migrated store. The suite populates every map and JSON field on
// purpose: a suite that only builds empty structs tests the absence of those
// fields, which is how serialization bugs survive on one backend and not
// another.
package storetest

import (
	"testing"

	"github.com/xraph/herald/store"
)

// Opener returns a fresh, empty, migrated store. It is called once per
// subtest, so subtests never see each other's rows.
type Opener func(t *testing.T) store.Store

// Run executes every conformance check against the backend open returns.
func Run(t *testing.T, open Opener) {
	t.Helper()
	t.Run("NotFound", func(t *testing.T) { testNotFound(t, open(t)) })
	t.Run("Duplicates", func(t *testing.T) { testDuplicates(t, open(t)) })
	t.Run("RoundTrips", func(t *testing.T) { testRoundTrips(t, open(t)) })
	t.Run("ReadsAreCopies", func(t *testing.T) { testReadsAreCopies(t, open(t)) })
	t.Run("EmptyAppIsExact", func(t *testing.T) { testEmptyAppIsExact(t, open(t)) })
	t.Run("Ordering", func(t *testing.T) { testOrdering(t, open(t)) })
	t.Run("MessagePaging", func(t *testing.T) { testMessagePaging(t, open(t)) })
	t.Run("RecordDelivery", func(t *testing.T) { testRecordDelivery(t, open(t)) })
	t.Run("CountMessages", func(t *testing.T) { testCountMessages(t, open(t)) })
}
