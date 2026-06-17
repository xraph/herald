package discord_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/discord"
	"github.com/xraph/herald/message"
)

func TestDiscordSendContent(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"id":"d-1"}`)
	d := &discord.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		Text: "plain message",
		Data: map[string]string{"webhook_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "d-1" {
		t.Errorf("res = %+v", res)
	}
	if !strings.Contains(srv.Captured.Query, "wait=true") {
		t.Errorf("query = %q, want wait=true", srv.Captured.Query)
	}
	var body struct{ Content string }
	srv.Captured.DecodeJSON(t, &body)
	if body.Content != "plain message" {
		t.Errorf("content = %q", body.Content)
	}
}

func TestDiscordSendEmbed(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"id":"d-2"}`)
	d := &discord.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		Title: "Alert", Text: "details",
		Data: map[string]string{"webhook_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(srv.Captured.Query, "wait=true") {
		t.Errorf("query = %q, want wait=true", srv.Captured.Query)
	}
	var body struct {
		Embeds []struct{ Title, Description string }
	}
	srv.Captured.DecodeJSON(t, &body)
	if len(body.Embeds) != 1 || body.Embeds[0].Title != "Alert" || body.Embeds[0].Description != "details" {
		t.Errorf("embeds = %+v", body.Embeds)
	}
}

func TestDiscordValidate(t *testing.T) {
	d := &discord.Driver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing webhook_url")
	}
}
