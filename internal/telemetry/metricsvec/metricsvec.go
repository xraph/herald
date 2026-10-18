// Package metricsvec declares herald's labelled counters and histograms over
// a go-utils metrics sink. It lives apart from telemetry so the driver
// modules, which import telemetry through driver, don't pull in go-utils.
package metricsvec

import (
	"strings"
	"sync"

	gometrics "github.com/xraph/go-utils/metrics"
)

// vec caches one series per label tuple. A series is registered on the sink
// the first time its tuple is used, so a hot path never registers twice.
type vec[T any] struct {
	keys   []string
	build  func(labels map[string]string) T
	mu     sync.RWMutex
	series map[string]T
}

func newVec[T any](keys []string, build func(map[string]string) T) *vec[T] {
	return &vec[T]{keys: keys, build: build, series: make(map[string]T)}
}

// get returns the series for values, given in the order the keys were
// declared.
func (v *vec[T]) get(values []string) T {
	key := strings.Join(values, "\xff")
	v.mu.RLock()
	s, ok := v.series[key]
	v.mu.RUnlock()
	if ok {
		return s
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if s, ok = v.series[key]; ok {
		return s
	}
	l := make(map[string]string, len(v.keys))
	for i, k := range v.keys {
		if i < len(values) {
			l[k] = values[i]
		} else {
			l[k] = ""
		}
	}
	s = v.build(l)
	v.series[key] = s
	return s
}

// CounterVec is a counter declared with label keys over a go-utils sink. A
// nil *CounterVec records nothing.
type CounterVec struct{ v *vec[gometrics.Counter] }

// NewCounterVec declares a counter called name with the given label keys. A
// nil m returns nil.
func NewCounterVec(m gometrics.Metrics, name, help string, keys ...string) *CounterVec {
	if m == nil {
		return nil
	}
	return &CounterVec{v: newVec(keys, func(l map[string]string) gometrics.Counter {
		return m.Counter(name, gometrics.WithDescription(help), gometrics.WithLabels(l))
	})}
}

// Inc adds one to the series for values, in the order the keys were declared.
func (c *CounterVec) Inc(values ...string) {
	if c == nil {
		return
	}
	c.v.get(values).Inc()
}

// HistogramVec is a histogram in seconds declared with label keys over a
// go-utils sink. A nil *HistogramVec records nothing.
type HistogramVec struct{ v *vec[gometrics.Histogram] }

// NewHistogramVec declares a histogram in seconds called name with the given
// buckets and label keys. A nil m returns nil.
func NewHistogramVec(m gometrics.Metrics, name, help string, buckets []float64, keys ...string) *HistogramVec {
	if m == nil {
		return nil
	}
	return &HistogramVec{v: newVec(keys, func(l map[string]string) gometrics.Histogram {
		return m.Histogram(name,
			gometrics.WithDescription(help),
			gometrics.WithUnit("s"),
			gometrics.WithBuckets(buckets...),
			gometrics.WithLabels(l),
		)
	})}
}

// Observe records seconds on the series for values, in the order the keys
// were declared.
func (h *HistogramVec) Observe(seconds float64, values ...string) {
	if h == nil {
		return
	}
	h.v.get(values).Observe(seconds)
}
