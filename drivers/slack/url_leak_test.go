package slack_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/drivers/slack"
)

// The webhook URL carries the provider's token, so no error may repeat it.
const secretURLPart = "tok-SECRET"

func TestSendErrorNeverQuotesAnUnparseableURL(t *testing.T) {
	_, err := (&slack.Driver{}).Send(context.Background(), &driver.OutboundMessage{
		Text: "hi",
		Data: map[string]string{"webhook_url": "http://[::1/" + secretURLPart},
	})
	if err == nil {
		t.Fatal("an unparseable URL produced no error")
	}
	if strings.Contains(err.Error(), secretURLPart) {
		t.Errorf("error leaks the URL: %v", err)
	}
}

func TestSendErrorNeverQuotesTheURLOnTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	closed := srv.URL
	srv.Close()

	_, err := (&slack.Driver{}).Send(context.Background(), &driver.OutboundMessage{
		Text: "hi",
		Data: map[string]string{"webhook_url": closed + "/hooks/" + secretURLPart},
	})
	if err == nil {
		t.Fatal("a closed port produced no error")
	}
	if strings.Contains(err.Error(), secretURLPart) {
		t.Errorf("error leaks the URL: %v", err)
	}
}
