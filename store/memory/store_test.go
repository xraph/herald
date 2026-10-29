package memory_test

import (
	"testing"

	"github.com/xraph/herald/device"
	"github.com/xraph/herald/device/devicetest"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/memory"
	"github.com/xraph/herald/store/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func(*testing.T) store.Store { return memory.New() })
}

func TestDeviceConformance(t *testing.T) {
	devicetest.Run(t, func(*testing.T) device.Store { return memory.New() })
}
