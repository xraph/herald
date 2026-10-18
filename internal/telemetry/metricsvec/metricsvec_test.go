package metricsvec_test

import (
	"sync"
	"testing"

	gometrics "github.com/xraph/go-utils/metrics"

	"github.com/xraph/herald/internal/telemetry/metricsvec"
	"github.com/xraph/herald/internal/telemetry/telemetrytest"
)

// countingSink counts how often each metric name is registered.
type countingSink struct {
	*telemetrytest.Sink
	mu    sync.Mutex
	calls map[string]int
}

func newCountingSink() *countingSink {
	c := &countingSink{Sink: telemetrytest.NewSink(), calls: map[string]int{}}
	counter, histogram := c.CounterFunc, c.HistogramFunc
	c.CounterFunc = func(name string, opts ...gometrics.MetricOption) gometrics.Counter {
		c.mu.Lock()
		c.calls[name]++
		c.mu.Unlock()
		return counter(name, opts...)
	}
	c.HistogramFunc = func(name string, opts ...gometrics.MetricOption) gometrics.Histogram {
		c.mu.Lock()
		c.calls[name]++
		c.mu.Unlock()
		return histogram(name, opts...)
	}
	return c
}

func TestVecRegistersEachTupleOnce(t *testing.T) {
	sink := newCountingSink()
	c := metricsvec.NewCounterVec(sink, "x_total", "help", "a", "b")
	h := metricsvec.NewHistogramVec(sink, "x_seconds", "help", []float64{1, 2}, "a")

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			c.Inc("1", "2")
			c.Inc("1", "3")
			h.Observe(0.5, "1")
		})
	}
	wg.Wait()

	if sink.calls["x_total"] != 2 || sink.calls["x_seconds"] != 1 {
		t.Errorf("registrations = %v, want one per label tuple", sink.calls)
	}
	if got := sink.Get("x_total", "a", "1", "b", "2").Value(); got != 50 {
		t.Errorf("x_total{1,2} = %v", got)
	}
	hs := sink.Get("x_seconds", "a", "1")
	if hs.Value() != 50 || hs.Unit != "s" || len(hs.Buckets) != 2 {
		t.Errorf("x_seconds = %+v", hs)
	}
}

func TestVecIsANoOpWithoutASink(t *testing.T) {
	c := metricsvec.NewCounterVec(nil, "x_total", "help", "a")
	h := metricsvec.NewHistogramVec(nil, "x_seconds", "help", nil, "a")
	if c != nil || h != nil {
		t.Fatal("a nil sink should give nil vectors")
	}
	c.Inc("1")
	h.Observe(1, "1")
}
