// Package telemetrytest records what herald's telemetry emits, for tests: a
// span recorder installed as the global tracer provider, and a go-utils
// metrics sink that remembers every series registered on it.
package telemetrytest

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	gometrics "github.com/xraph/go-utils/metrics"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Spans installs a span recorder as the global tracer provider for the rest
// of the test and puts the previous provider back when the test ends. Tests
// that call it must not run in parallel.
func Spans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	prev := otel.GetTracerProvider()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Errorf("shut down the test tracer provider: %v", err)
		}
	})
	return rec
}

// Named returns the ended spans called name, in the order they ended.
func Named(rec *tracetest.SpanRecorder, name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

// One returns the only ended span called name, failing the test otherwise.
func One(t *testing.T, rec *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	got := Named(rec, name)
	if len(got) != 1 {
		names := make([]string, 0, len(rec.Ended()))
		for _, s := range rec.Ended() {
			names = append(names, s.Name())
		}
		t.Fatalf("want one %q span, got %d (ended: %q)", name, len(got), names)
	}
	return got[0]
}

// Attrs returns s's attributes as strings keyed by attribute name.
func Attrs(s sdktrace.ReadOnlySpan) map[string]string {
	out := make(map[string]string, len(s.Attributes()))
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.String()
	}
	return out
}

// ExceptionTypes returns the exception.type of every exception event on s.
func ExceptionTypes(s sdktrace.ReadOnlySpan) []string {
	var out []string
	for _, ev := range s.Events() {
		if ev.Name != "exception" {
			continue
		}
		for _, kv := range ev.Attributes {
			if kv.Key == "exception.type" {
				out = append(out, kv.Value.String())
			}
		}
	}
	return out
}

// Leaks returns every place value appears in what the recorder saw: a span
// name, an attribute, an event name or attribute, or a status description.
// A test that feeds personal data or a secret through the code asserts this
// is empty.
func Leaks(rec *tracetest.SpanRecorder, value string) []string {
	var out []string
	for _, s := range rec.Ended() {
		if strings.Contains(s.Name(), value) {
			out = append(out, s.Name()+": name")
		}
		for _, kv := range s.Attributes() {
			if strings.Contains(kv.Value.String(), value) {
				out = append(out, fmt.Sprintf("%s: attribute %s", s.Name(), kv.Key))
			}
		}
		for _, ev := range s.Events() {
			if strings.Contains(ev.Name, value) {
				out = append(out, fmt.Sprintf("%s: event %s", s.Name(), ev.Name))
			}
			for _, kv := range ev.Attributes {
				if strings.Contains(kv.Value.String(), value) {
					out = append(out, fmt.Sprintf("%s: event %s attribute %s", s.Name(), ev.Name, kv.Key))
				}
			}
		}
		if strings.Contains(s.Status().Description, value) {
			out = append(out, s.Name()+": status")
		}
	}
	return out
}

// Series is one metric series registered on a [Sink].
type Series struct {
	Name    string
	Kind    string // counter or histogram
	Labels  map[string]string
	Unit    string
	Buckets []float64

	counter   *gometrics.MockCounter
	histogram *gometrics.MockHistogram
}

// Value is a counter's value, or a histogram's observation count.
func (s *Series) Value() float64 {
	if s == nil {
		return 0
	}
	if s.counter != nil {
		return s.counter.Value()
	}
	return float64(s.histogram.Count())
}

// Sink is a go-utils metrics.Metrics that remembers every counter and
// histogram registered on it, keyed by name and labels.
type Sink struct {
	*gometrics.MockMetrics

	mu     sync.Mutex
	series map[string]*Series
}

// NewSink returns an empty Sink.
func NewSink() *Sink {
	s := &Sink{MockMetrics: gometrics.NewMockMetrics(), series: make(map[string]*Series)}
	s.CounterFunc = func(name string, opts ...gometrics.MetricOption) gometrics.Counter {
		return s.register(name, "counter", opts).counter
	}
	s.HistogramFunc = func(name string, opts ...gometrics.MetricOption) gometrics.Histogram {
		return s.register(name, "histogram", opts).histogram
	}
	return s
}

func (s *Sink) register(name, kind string, opts []gometrics.MetricOption) *Series {
	var o gometrics.MetricOptions
	for _, opt := range opts {
		opt(&o)
	}
	key := seriesKey(name, o.Labels)

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.series[key]; ok {
		return existing
	}
	sr := &Series{Name: name, Kind: kind, Labels: maps.Clone(o.Labels), Unit: o.Unit, Buckets: o.Buckets}
	if kind == "counter" {
		sr.counter = gometrics.NewMockCounter()
	} else {
		sr.histogram = gometrics.NewMockHistogram()
	}
	s.series[key] = sr
	return sr
}

// Get returns the series name{labels}, where labels alternate key and value,
// or nil if nothing registered it.
func (s *Sink) Get(name string, labels ...string) *Series {
	l := make(map[string]string, len(labels)/2)
	for i := 0; i+1 < len(labels); i += 2 {
		l[labels[i]] = labels[i+1]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.series[seriesKey(name, l)]
}

// All returns every series registered under name.
func (s *Sink) All(name string) []*Series {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Series
	for _, sr := range s.series {
		if sr.Name == name {
			out = append(out, sr)
		}
	}
	return out
}

// Names returns every metric name registered, sorted.
func (s *Sink) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool)
	for _, sr := range s.series {
		seen[sr.Name] = true
	}
	return slices.Sorted(maps.Keys(seen))
}

func seriesKey(name string, labels map[string]string) string {
	var b strings.Builder
	b.WriteString(name)
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		b.WriteString("|" + k + "=" + labels[k])
	}
	return b.String()
}
