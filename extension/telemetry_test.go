package extension

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
)

// okDriver delivers everything without calling out.
type okDriver struct{}

func (okDriver) Name() string                          { return "ok" }
func (okDriver) Channel() string                       { return "email" }
func (okDriver) Validate(_, _ map[string]string) error { return nil }
func (okDriver) Send(context.Context, *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	return &driver.DeliveryResult{Status: message.StatusSent}, nil
}

// scrape reads the app's Prometheus exposition, the body /_/metrics serves.
func scrape(t *testing.T, app forge.App) string {
	t.Helper()
	p, ok := app.Metrics().(interface{ PrometheusHandler() http.Handler })
	if !ok {
		t.Fatalf("app metrics %T have no Prometheus handler", app.Metrics())
	}
	rr := httptest.NewRecorder()
	p.PrometheusHandler().ServeHTTP(rr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/_/metrics", nil))
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read scrape: %v", err)
	}
	return string(body)
}

// sample returns the value on the first scrape line for name that carries
// every one of labels, or "" when there is none.
func sample(body, name string, labels ...string) string {
	for line := range strings.SplitSeq(body, "\n") {
		if !strings.HasPrefix(line, name+"{") && !strings.HasPrefix(line, name+" ") {
			continue
		}
		matched := true
		for _, l := range labels {
			if !strings.Contains(line, l) {
				matched = false
				break
			}
		}
		if matched {
			return line[strings.LastIndex(line, " ")+1:]
		}
	}
	return ""
}

// TestExtensionRecordsOnTheAppMetrics registers the extension on a real
// forge app and checks that a send lands on the app's scrape under the
// forge_herald_ names, with only the documented labels.
func TestExtensionRecordsOnTheAppMetrics(t *testing.T) {
	cfg := forge.DefaultAppConfig()
	cfg.Name = "herald-telemetry-test"
	cfg.Logger = forge.NewNoopLogger()
	cfg.EnableConfigAutoDiscovery = false
	cfg.EnableEnvConfig = false
	app := forge.NewApp(cfg)

	e := New(WithStore(memory.New()), WithDisableRoutes(), WithDriver(okDriver{}))
	if err := e.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	h := e.Herald()
	p := &provider.Provider{ID: id.NewProviderID(), AppID: "app_tel", Name: "ok", Channel: "email", Driver: "ok", Enabled: true}
	if err := h.Store().CreateProvider(t.Context(), p); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Send(t.Context(), &herald.SendRequest{AppID: "app_tel", Channel: "email", Body: "hi", To: []string{"alice@example.com"}}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := h.Send(t.Context(), &herald.SendRequest{AppID: "app_tel", Channel: "fax", Body: "hi", To: []string{"alice@example.com"}}); err == nil {
		t.Fatal("a send on a channel with no provider succeeded")
	}

	body := scrape(t, app)
	if got := sample(body, "forge_herald_deliveries_attempted_total", `channel="email"`, `driver="ok"`, `result="ok"`); got != "1" {
		t.Errorf("forge_herald_deliveries_attempted_total = %q, want 1\n%s", got, grep(body, "herald"))
	}
	if got := sample(body, "forge_herald_delivery_duration_seconds_count", `channel="email"`, `driver="ok"`, `result="ok"`); got != "1" {
		t.Errorf("forge_herald_delivery_duration_seconds_count = %q, want 1", got)
	}
	if got := sample(body, "forge_herald_sends_rejected_total", `channel="other"`, `reason="no_provider"`); got != "1" {
		t.Errorf("forge_herald_sends_rejected_total = %q, want 1", got)
	}
	for _, leak := range []string{"alice@example.com", "app_tel", "fax", p.ID.String()} {
		if strings.Contains(grep(body, "herald"), leak) {
			t.Errorf("%q reached the scrape", leak)
		}
	}
}

func grep(body, substr string) string {
	var out []string
	for line := range strings.SplitSeq(body, "\n") {
		if strings.Contains(line, substr) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
