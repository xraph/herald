package sendgrid_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/drivers/sendgrid"
	"github.com/xraph/herald/message"
)

func TestSendgridSend(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = readAll(r)
		w.Header().Set("X-Message-Id", "sg-1")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := &sendgrid.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", FromName: "Acme", Subject: "Hi", HTML: "<b>x</b>",
		Data: map[string]string{"api_key": "K", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "sg-1" {
		t.Errorf("res = %+v", res)
	}
	if gotPath != "/v3/mail/send" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer K" {
		t.Errorf("auth = %q", gotAuth)
	}
	var payload struct {
		Personalizations []struct {
			To []struct {
				Email string `json:"email"`
			} `json:"to"`
		} `json:"personalizations"`
		From struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"from"`
		Subject string `json:"subject"`
		Content []struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"content"`
	}
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(payload.Personalizations) == 0 || len(payload.Personalizations[0].To) == 0 {
		t.Fatal("personalizations[0].to is empty")
	}
	if got := payload.Personalizations[0].To[0].Email; got != "to@x.com" {
		t.Errorf("personalizations[0].to[0].email = %q, want %q", got, "to@x.com")
	}
	if got := payload.From.Email; got != "from@x.com" {
		t.Errorf("from.email = %q, want %q", got, "from@x.com")
	}
	if got := payload.From.Name; got != "Acme" {
		t.Errorf("from.name = %q, want %q", got, "Acme")
	}
	if got := payload.Subject; got != "Hi" {
		t.Errorf("subject = %q, want %q", got, "Hi")
	}
}

func readAll(r *http.Request) ([]byte, error) { return io.ReadAll(r.Body) }
