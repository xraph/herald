package discord_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/drivers/discord"
)

// The user supplies the whole URL, so the client span carries neither its
// token nor its host: only the method, the driver and the status.
func TestSendNeverPutsTheURLOnASpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	if _, err := (&discord.Driver{}).Send(context.Background(), &driver.OutboundMessage{
		Text: "hi",
		Data: map[string]string{"webhook_url": srv.URL + "/hooks/" + secretURLPart + "?token=" + secretURLPart},
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Name() != "POST" {
		t.Fatalf("want one POST span, got %d", len(spans))
	}
	attrs := map[string]string{}
	for _, kv := range spans[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.String()
		for _, leak := range []string{secretURLPart, u.Host, u.Hostname()} {
			if strings.Contains(kv.Value.String(), leak) {
				t.Errorf("attribute %s = %q carries %q", kv.Key, kv.Value.String(), leak)
			}
		}
	}
	if attrs["herald.driver"] != "discord" || attrs["http.response.status_code"] != "200" {
		t.Errorf("attrs = %v", attrs)
	}
}
